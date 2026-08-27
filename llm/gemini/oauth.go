package gemini

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	vertexScope     = "https://www.googleapis.com/auth/cloud-platform"
	defaultTokenURI = "https://oauth2.googleapis.com/token" //nosec G101 -- public OAuth2 token endpoint URL, not a credential
)

type serviceAccountKey struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

// saTokenSource mints Google OAuth2 access tokens from a service account via the
// JWT-bearer grant, caching until shortly before expiry. Hand-rolled with the
// standard library (like bedrock's SigV4 signer) so langrails takes on no Google
// cloud-SDK dependency.
type saTokenSource struct {
	email    string
	key      *rsa.PrivateKey
	tokenURI string
	client   *http.Client

	mu      sync.Mutex
	token   string
	expires time.Time
	now     func() time.Time
}

// TokenSourceOption configures a service-account token source.
type TokenSourceOption func(*saTokenSource)

// WithTokenHTTPClient sets the HTTP client used to reach the OAuth2 token
// endpoint (e.g. to route it through a proxy).
func WithTokenHTTPClient(c *http.Client) TokenSourceOption {
	return func(t *saTokenSource) { t.client = c }
}

// NewServiceAccountTokenSource builds a TokenSource from a Google service-account
// JSON key (the file downloaded from the GCP console). Pass the result to
// WithVertex. The token is fetched lazily on first use and refreshed
// automatically before it expires.
func NewServiceAccountTokenSource(saJSON []byte, opts ...TokenSourceOption) (TokenSource, error) {
	var sa serviceAccountKey
	if err := json.Unmarshal(saJSON, &sa); err != nil {
		return nil, fmt.Errorf("vertex: parse service account JSON: %w", err)
	}
	if sa.ClientEmail == "" || sa.PrivateKey == "" {
		return nil, fmt.Errorf("vertex: service account JSON missing client_email or private_key")
	}
	key, err := parseRSAPrivateKey(sa.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("vertex: %w", err)
	}
	tokenURI := sa.TokenURI
	if tokenURI == "" {
		tokenURI = defaultTokenURI
	}
	ts := &saTokenSource{
		email:    sa.ClientEmail,
		key:      key,
		tokenURI: tokenURI,
		client:   &http.Client{Timeout: 30 * time.Second},
		now:      time.Now,
	}
	for _, o := range opts {
		o(ts)
	}
	return ts, nil
}

// Token returns a cached access token, minting a new one when the cache is empty
// or near expiry.
func (t *saTokenSource) Token(ctx context.Context) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.token != "" && t.now().Before(t.expires) {
		return t.token, nil
	}

	tok, ttl, err := t.mint(ctx)
	if err != nil {
		return "", err
	}
	t.token = tok
	// Refresh a minute early so an in-flight request never rides a token that
	// expires between mint and use.
	t.expires = t.now().Add(ttl - time.Minute)
	return tok, nil
}

func (t *saTokenSource) mint(ctx context.Context) (string, time.Duration, error) {
	now := t.now()
	claims := map[string]any{
		"iss":   t.email,
		"scope": vertexScope,
		"aud":   t.tokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	}
	assertion, err := signRS256JWT(claims, t.key)
	if err != nil {
		return "", 0, err
	}

	form := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {assertion},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.tokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := t.client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("vertex: token request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("vertex: token endpoint status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(raw, &tr); err != nil {
		return "", 0, fmt.Errorf("vertex: parse token response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", 0, fmt.Errorf("vertex: token response carried no access_token")
	}
	ttl := time.Duration(tr.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = time.Hour
	}
	return tr.AccessToken, ttl, nil
}

func signRS256JWT(claims map[string]any, key *rsa.PrivateKey) (string, error) {
	cb, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signingInput := b64url([]byte(`{"alg":"RS256","typ":"JWT"}`)) + "." + b64url(cb)
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign jwt: %w", err)
	}
	return signingInput + "." + b64url(sig), nil
}

func b64url(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func parseRSAPrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in private key")
	}
	// GCP service-account keys are PKCS#8 ("PRIVATE KEY"); accept PKCS#1 too.
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := k.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("private key is not RSA")
		}
		return rsaKey, nil
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	return nil, fmt.Errorf("unsupported private key format (want PKCS#8 or PKCS#1 RSA)")
}
