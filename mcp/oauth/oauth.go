// Package oauth authorizes an MCP client against a protected MCP server
// the way the MCP authorization spec describes: OAuth 2.1 with discovery
// of the authorization server (RFC 9728 protected resource metadata and
// RFC 8414 server metadata), dynamic client registration (RFC 7591), the
// authorization code flow with PKCE, resource indicators (RFC 8707) and
// token refresh. Machine-to-machine clients can use the client
// credentials grant instead.
//
// A TokenSource plugs into the client with mcp.WithTokenSource:
//
//	ts := oauth.New("https://mcp.example.com/mcp", oauth.Config{
//	    RedirectURL: "http://127.0.0.1:8765/callback",
//	    Authorize:   oauth.Loopback("http://127.0.0.1:8765/callback", openBrowser),
//	})
//	client, err := mcp.NewClient("https://mcp.example.com/mcp", mcp.WithTokenSource(ts))
package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ErrAuthorizationRequired is returned when no valid token is available
// and the Config has neither an Authorize function nor a client secret to
// get one.
var ErrAuthorizationRequired = errors.New("oauth: authorization required")

// expiryMargin renews a token this long before it expires.
const expiryMargin = 30 * time.Second

// Token is an OAuth access token.
type Token struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Expiry       time.Time `json:"expiry,omitempty"`
}

// Valid reports whether the token is set and not about to expire.
func (t *Token) Valid() bool {
	if t == nil || t.AccessToken == "" {
		return false
	}
	return t.Expiry.IsZero() || time.Now().Add(expiryMargin).Before(t.Expiry)
}

// Session is what a Store keeps between runs: the registered client and
// its latest token.
type Session struct {
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
	Token        *Token `json:"token,omitempty"`
}

// Store persists the session, so a restart neither registers a new client
// nor asks the user to authorize again. Keep it somewhere private: it
// holds credentials.
type Store interface {
	Load(ctx context.Context) (*Session, error)
	Save(ctx context.Context, s *Session) error
}

// MemoryStore keeps the session in memory. It is the default.
type MemoryStore struct {
	mu sync.Mutex
	s  *Session
}

// Load returns the stored session, or nil.
func (m *MemoryStore) Load(context.Context) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.s == nil {
		return nil, nil
	}
	c := *m.s
	return &c, nil
}

// Save stores a copy of s.
func (m *MemoryStore) Save(_ context.Context, s *Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := *s
	m.s = &c
	return nil
}

// Authorizer sends the user to authURL and returns the authorization code
// the server redirects back with. It must check that the redirect's state
// parameter matches the one in authURL; Loopback does.
type Authorizer func(ctx context.Context, authURL string) (code string, err error)

// Config configures a TokenSource.
type Config struct {
	// ClientID and ClientSecret identify a pre-registered client. When
	// ClientID is empty the client registers itself dynamically, if the
	// server supports it.
	ClientID     string
	ClientSecret string

	// ClientName is the name used for dynamic registration. Default
	// "langrails".
	ClientName string

	// RedirectURL is where the authorization server sends the user back
	// to. Required for the authorization code flow.
	RedirectURL string

	// Scopes are requested with the token. Optional.
	Scopes []string

	// Authorize runs the authorization code flow's user step. When nil and
	// ClientSecret is set, the client credentials grant is used instead.
	Authorize Authorizer

	// Store persists the session. Default is a MemoryStore.
	Store Store

	// HTTPClient makes the discovery, registration and token requests.
	HTTPClient *http.Client
}

// TokenSource supplies access tokens for one MCP server, getting and
// renewing them as needed. It implements the token source the mcp package
// accepts. It is safe for concurrent use.
type TokenSource struct {
	resource string
	cfg      Config

	mu      sync.Mutex
	session *Session
	meta    *serverMetadata
}

// New creates a TokenSource for the MCP server at serverURL.
func New(serverURL string, cfg Config) *TokenSource {
	if cfg.Store == nil {
		cfg.Store = &MemoryStore{}
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if cfg.ClientName == "" {
		cfg.ClientName = "langrails"
	}
	return &TokenSource{resource: serverURL, cfg: cfg}
}

// Token returns a valid access token: the stored one, a refreshed one, or
// a new one from the configured grant.
func (ts *TokenSource) Token(ctx context.Context) (string, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.session == nil {
		s, err := ts.cfg.Store.Load(ctx)
		if err != nil {
			return "", fmt.Errorf("oauth: load session: %w", err)
		}
		if s == nil {
			s = &Session{}
		}
		ts.session = s
	}
	if ts.session.Token.Valid() {
		return ts.session.Token.AccessToken, nil
	}

	if err := ts.discover(ctx); err != nil {
		return "", err
	}
	if t := ts.session.Token; t != nil && t.RefreshToken != "" {
		if tok, err := ts.refresh(ctx, t.RefreshToken); err == nil {
			return ts.store(ctx, tok)
		}
		// A refresh token can expire or be revoked; get a new grant.
	}

	var tok *Token
	var err error
	switch {
	case ts.cfg.Authorize != nil:
		tok, err = ts.authorizationCode(ctx)
	case ts.clientSecret() != "":
		tok, err = ts.clientCredentials(ctx)
	default:
		return "", ErrAuthorizationRequired
	}
	if err != nil {
		return "", err
	}
	return ts.store(ctx, tok)
}

// InvalidateToken drops the current access token, for example after the
// server rejected it, so the next Token call refreshes or re-authorizes.
func (ts *TokenSource) InvalidateToken() {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.session != nil && ts.session.Token != nil {
		t := *ts.session.Token
		t.AccessToken = ""
		ts.session.Token = &t
	}
}

func (ts *TokenSource) store(ctx context.Context, tok *Token) (string, error) {
	if tok.RefreshToken == "" && ts.session.Token != nil {
		// Servers may keep the refresh token unchanged and omit it.
		tok.RefreshToken = ts.session.Token.RefreshToken
	}
	ts.session.Token = tok
	if err := ts.cfg.Store.Save(ctx, ts.session); err != nil {
		return "", fmt.Errorf("oauth: save session: %w", err)
	}
	return tok.AccessToken, nil
}

func (ts *TokenSource) clientID() string {
	if ts.cfg.ClientID != "" {
		return ts.cfg.ClientID
	}
	return ts.session.ClientID
}

func (ts *TokenSource) clientSecret() string {
	if ts.cfg.ClientID != "" {
		return ts.cfg.ClientSecret
	}
	return ts.session.ClientSecret
}

// authorizationCode runs the authorization code flow with PKCE.
func (ts *TokenSource) authorizationCode(ctx context.Context) (*Token, error) {
	if ts.cfg.RedirectURL == "" {
		return nil, errors.New("oauth: RedirectURL is required for the authorization code flow")
	}
	if err := ts.register(ctx, []string{"authorization_code", "refresh_token"}); err != nil {
		return nil, err
	}
	verifier, err := randomString(32)
	if err != nil {
		return nil, err
	}
	state, err := randomString(16)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(verifier))

	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {ts.clientID()},
		"redirect_uri":          {ts.cfg.RedirectURL},
		"state":                 {state},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(sum[:])},
		"code_challenge_method": {"S256"},
		"resource":              {ts.resource},
	}
	if len(ts.cfg.Scopes) > 0 {
		q.Set("scope", strings.Join(ts.cfg.Scopes, " "))
	}
	authURL := ts.meta.AuthorizationEndpoint
	if strings.Contains(authURL, "?") {
		authURL += "&" + q.Encode()
	} else {
		authURL += "?" + q.Encode()
	}

	code, err := ts.cfg.Authorize(ctx, authURL)
	if err != nil {
		return nil, fmt.Errorf("oauth: authorize: %w", err)
	}
	return ts.tokenRequest(ctx, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {ts.cfg.RedirectURL},
		"code_verifier": {verifier},
	})
}

func (ts *TokenSource) clientCredentials(ctx context.Context) (*Token, error) {
	form := url.Values{"grant_type": {"client_credentials"}}
	if len(ts.cfg.Scopes) > 0 {
		form.Set("scope", strings.Join(ts.cfg.Scopes, " "))
	}
	return ts.tokenRequest(ctx, form)
}

func (ts *TokenSource) refresh(ctx context.Context, refreshToken string) (*Token, error) {
	return ts.tokenRequest(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	})
}

// tokenRequest posts form to the token endpoint, authenticating the client
// with HTTP basic auth when it has a secret and by client_id otherwise.
func (ts *TokenSource) tokenRequest(ctx context.Context, form url.Values) (*Token, error) {
	form.Set("resource", ts.resource)
	secret := ts.clientSecret()
	if secret == "" {
		form.Set("client_id", ts.clientID())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ts.meta.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if secret != "" {
		req.SetBasicAuth(url.QueryEscape(ts.clientID()), url.QueryEscape(secret))
	}

	var out struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := ts.do(req, &out); err != nil {
		return nil, fmt.Errorf("oauth: %s grant: %w", form.Get("grant_type"), err)
	}
	if out.AccessToken == "" {
		return nil, fmt.Errorf("oauth: %s grant: no access token in response", form.Get("grant_type"))
	}
	tok := &Token{AccessToken: out.AccessToken, TokenType: out.TokenType, RefreshToken: out.RefreshToken}
	if out.ExpiresIn > 0 {
		tok.Expiry = time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	}
	return tok, nil
}

// register registers the client dynamically unless it already has an ID.
func (ts *TokenSource) register(ctx context.Context, grants []string) error {
	if ts.clientID() != "" {
		return nil
	}
	if ts.meta.RegistrationEndpoint == "" {
		return errors.New("oauth: the server does not support dynamic client registration; set Config.ClientID")
	}
	body := map[string]any{
		"client_name":                ts.cfg.ClientName,
		"grant_types":                grants,
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
	}
	if ts.cfg.RedirectURL != "" {
		body["redirect_uris"] = []string{ts.cfg.RedirectURL}
	}
	if len(ts.cfg.Scopes) > 0 {
		body["scope"] = strings.Join(ts.cfg.Scopes, " ")
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ts.meta.RegistrationEndpoint, strings.NewReader(string(raw)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	var out struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := ts.do(req, &out); err != nil {
		return fmt.Errorf("oauth: client registration: %w", err)
	}
	if out.ClientID == "" {
		return errors.New("oauth: client registration: no client_id in response")
	}
	ts.session.ClientID, ts.session.ClientSecret = out.ClientID, out.ClientSecret
	if err := ts.cfg.Store.Save(ctx, ts.session); err != nil {
		return fmt.Errorf("oauth: save session: %w", err)
	}
	return nil
}

// do sends req and decodes a 2xx JSON response into out, or returns the
// OAuth error the server reported.
func (ts *TokenSource) do(req *http.Request, out any) error {
	resp, err := ts.cfg.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var e struct {
			Error       string `json:"error"`
			Description string `json:"error_description"`
		}
		if json.Unmarshal(raw, &e) == nil && e.Error != "" {
			if e.Description != "" {
				return fmt.Errorf("%s: %s", e.Error, e.Description)
			}
			return errors.New(e.Error)
		}
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return json.Unmarshal(raw, out)
}

func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
