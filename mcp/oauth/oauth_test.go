package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// fakeAS is an authorization server that also serves the protected
// resource metadata for "/mcp".
type fakeAS struct {
	t   *testing.T
	srv *httptest.Server

	mu         sync.Mutex
	challenges map[string]string // code -> PKCE challenge
	grants     []string
	registered int
	tokens     int
	noMetadata bool
	refreshErr bool
}

func newFakeAS(t *testing.T) *fakeAS {
	f := &fakeAS{t: t, challenges: map[string]string{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/oauth-protected-resource/mcp", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"resource": f.srv.URL + "/mcp", "authorization_servers": []string{f.srv.URL + "/tenant"}})
	})
	mux.HandleFunc("/.well-known/oauth-authorization-server/tenant", func(w http.ResponseWriter, _ *http.Request) {
		if f.noMetadata {
			http.NotFound(w, nil)
			return
		}
		writeJSON(w, map[string]any{
			"authorization_endpoint": f.srv.URL + "/tenant/auth",
			"token_endpoint":         f.srv.URL + "/tenant/token",
			"registration_endpoint":  f.srv.URL + "/tenant/register",
		})
	})
	mux.HandleFunc("/tenant/register", f.register)
	mux.HandleFunc("/register", f.register)
	mux.HandleFunc("/tenant/token", f.token)
	mux.HandleFunc("/token", f.token)
	f.srv = httptest.NewServer(mux)
	return f
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (f *fakeAS) register(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body["client_name"] != "langrails" || body["token_endpoint_auth_method"] != "none" {
		f.t.Errorf("registration = %v", body)
	}
	f.mu.Lock()
	f.registered++
	f.mu.Unlock()
	writeJSON(w, map[string]any{"client_id": "dyn-client"})
}

func (f *fakeAS) token(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	f.mu.Lock()
	defer f.mu.Unlock()
	grant := r.Form.Get("grant_type")
	f.grants = append(f.grants, grant)
	if r.Form.Get("resource") != f.srv.URL+"/mcp" {
		f.t.Errorf("resource = %q", r.Form.Get("resource"))
	}
	switch grant {
	case "authorization_code":
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if f.challenges[r.Form.Get("code")] != base64.RawURLEncoding.EncodeToString(sum[:]) {
			w.WriteHeader(http.StatusBadRequest)
			writeJSON(w, map[string]any{"error": "invalid_grant", "error_description": "PKCE mismatch"})
			return
		}
		if r.Form.Get("client_id") != "dyn-client" {
			f.t.Errorf("client_id = %q", r.Form.Get("client_id"))
		}
	case "client_credentials":
		id, secret, ok := r.BasicAuth()
		if !ok || id != "svc" || secret != "s3cret" {
			w.WriteHeader(http.StatusUnauthorized)
			writeJSON(w, map[string]any{"error": "invalid_client"})
			return
		}
	case "refresh_token":
		if f.refreshErr || r.Form.Get("refresh_token") != "refresh-1" {
			w.WriteHeader(http.StatusBadRequest)
			writeJSON(w, map[string]any{"error": "invalid_grant"})
			return
		}
	}
	f.tokens++
	resp := map[string]any{"access_token": fmt.Sprintf("access-%d", f.tokens), "token_type": "Bearer", "expires_in": 3600}
	if grant == "authorization_code" {
		resp["refresh_token"] = "refresh-1"
	}
	writeJSON(w, resp)
}

// authorize plays the user: it checks the authorization URL and returns a
// code bound to its PKCE challenge.
func (f *fakeAS) authorize(_ context.Context, authURL string) (string, error) {
	u, err := url.Parse(authURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	if !strings.HasPrefix(authURL, f.srv.URL+"/tenant/auth?") || q.Get("code_challenge_method") != "S256" ||
		q.Get("client_id") != "dyn-client" || q.Get("state") == "" || q.Get("scope") != "read write" {
		f.t.Errorf("authorization URL = %s", authURL)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.challenges["code-1"] = q.Get("code_challenge")
	return "code-1", nil
}

func TestAuthorizationCodeFlow(t *testing.T) {
	f := newFakeAS(t)
	defer f.srv.Close()
	store := &MemoryStore{}
	ts := New(f.srv.URL+"/mcp", Config{
		RedirectURL: "http://127.0.0.1:1/cb",
		Scopes:      []string{"read", "write"},
		Authorize:   f.authorize,
		Store:       store,
	})

	tok, err := ts.Token(context.Background())
	if err != nil || tok != "access-1" {
		t.Fatalf("token = %q, err = %v", tok, err)
	}
	// Cached while valid.
	if tok, _ := ts.Token(context.Background()); tok != "access-1" {
		t.Errorf("second token = %q", tok)
	}

	// After the server rejects it, the refresh token is used.
	ts.InvalidateToken()
	if tok, err := ts.Token(context.Background()); err != nil || tok != "access-2" {
		t.Fatalf("refreshed token = %q, err = %v", tok, err)
	}
	if got := strings.Join(f.grants, ","); got != "authorization_code,refresh_token" {
		t.Errorf("grants = %s", got)
	}

	// The session survives in the store: a new source neither registers
	// nor authorizes again.
	s, _ := store.Load(context.Background())
	if s.ClientID != "dyn-client" || s.Token.RefreshToken != "refresh-1" {
		t.Errorf("stored session = %+v", s)
	}
	ts2 := New(f.srv.URL+"/mcp", Config{Store: store})
	if tok, err := ts2.Token(context.Background()); err != nil || tok != "access-2" {
		t.Fatalf("restored token = %q, err = %v", tok, err)
	}
	if f.registered != 1 {
		t.Errorf("registered %d times", f.registered)
	}
}

func TestRefreshFailureReauthorizes(t *testing.T) {
	f := newFakeAS(t)
	defer f.srv.Close()
	ts := New(f.srv.URL+"/mcp", Config{RedirectURL: "http://127.0.0.1:1/cb", Scopes: []string{"read", "write"}, Authorize: f.authorize})
	if _, err := ts.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.refreshErr = true
	ts.InvalidateToken()
	if tok, err := ts.Token(context.Background()); err != nil || tok != "access-2" {
		t.Fatalf("token = %q, err = %v", tok, err)
	}
	if got := strings.Join(f.grants, ","); got != "authorization_code,refresh_token,authorization_code" {
		t.Errorf("grants = %s", got)
	}
}

func TestClientCredentials(t *testing.T) {
	f := newFakeAS(t)
	defer f.srv.Close()
	ts := New(f.srv.URL+"/mcp", Config{ClientID: "svc", ClientSecret: "s3cret"})
	if tok, err := ts.Token(context.Background()); err != nil || tok != "access-1" {
		t.Fatalf("token = %q, err = %v", tok, err)
	}
	if f.registered != 0 {
		t.Error("a pre-registered client should not register")
	}

	bad := New(f.srv.URL+"/mcp", Config{ClientID: "svc", ClientSecret: "wrong"})
	if _, err := bad.Token(context.Background()); err == nil || !strings.Contains(err.Error(), "invalid_client") {
		t.Fatalf("err = %v", err)
	}
}

func TestFallbackEndpoints(t *testing.T) {
	f := newFakeAS(t)
	f.noMetadata = true
	defer f.srv.Close()
	ts := New(f.srv.URL+"/mcp", Config{ClientID: "svc", ClientSecret: "s3cret"})
	if _, err := ts.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ts.meta.TokenEndpoint != f.srv.URL+"/token" {
		t.Errorf("token endpoint = %q", ts.meta.TokenEndpoint)
	}
}

func TestAuthorizationRequired(t *testing.T) {
	f := newFakeAS(t)
	defer f.srv.Close()
	if _, err := New(f.srv.URL+"/mcp", Config{}).Token(context.Background()); !errors.Is(err, ErrAuthorizationRequired) {
		t.Fatalf("err = %v", err)
	}
	if _, err := New(f.srv.URL+"/mcp", Config{Authorize: f.authorize}).Token(context.Background()); err == nil {
		t.Fatal("expected an error without RedirectURL")
	}
}

func TestLoopback(t *testing.T) {
	redirect := "http://127.0.0.1:18765/callback"
	follow := func(q string) func(string) error {
		return func(string) error {
			go func() {
				resp, err := http.Get(redirect + q) // #nosec G107 -- test loopback URL
				if err == nil {
					resp.Body.Close()
				}
			}()
			return nil
		}
	}
	authURL := "https://as.example/auth?state=xyz"

	code, err := Loopback(redirect, follow("?state=xyz&code=abc"))(context.Background(), authURL)
	if err != nil || code != "abc" {
		t.Fatalf("code = %q, err = %v", code, err)
	}
	if _, err := Loopback(redirect, follow("?state=evil&code=abc"))(context.Background(), authURL); err == nil {
		t.Fatal("expected a state mismatch")
	}
	if _, err := Loopback(redirect, follow("?state=xyz&error=access_denied"))(context.Background(), authURL); err == nil || !strings.Contains(err.Error(), "access_denied") {
		t.Fatalf("err = %v", err)
	}
	if _, err := Loopback("https://example.com/cb", follow(""))(context.Background(), authURL); err == nil {
		t.Fatal("expected a non-loopback redirect to be refused")
	}
}
