package gemini

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/promptrails/langrails"
)

type fakeTokenSource string

func (f fakeTokenSource) Token(context.Context) (string, error) { return string(f), nil }

// Vertex mode must route to the aiplatform publisher path and authenticate with
// a Bearer token — the whole point of the option (no API-key geo restriction).
func TestVertexRoutingAndAuth(t *testing.T) {
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"hi"}]},"finishReason":"STOP"}]}`)
	}))
	defer srv.Close()

	p := New("", WithVertex("my-proj", "europe-west4", fakeTokenSource("tok-123")), WithBaseURL(srv.URL))
	resp, err := p.Complete(context.Background(), &langrails.CompletionRequest{
		Model:    "gemini-2.5-flash",
		Messages: []langrails.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Content != "hi" {
		t.Fatalf("content = %q, want %q", resp.Content, "hi")
	}
	if gotAuth != "Bearer tok-123" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer tok-123")
	}
	wantPath := "/projects/my-proj/locations/europe-west4/publishers/google/models/gemini-2.5-flash:generateContent"
	if gotPath != wantPath {
		t.Fatalf("path = %q, want %q", gotPath, wantPath)
	}
}

// The default (API-key) path must be byte-for-byte unchanged by the Vertex work.
func TestDefaultURLUnchanged(t *testing.T) {
	p := New("KEY")
	if got, want := p.buildURL("gemini-2.0-flash", "generateContent", false),
		"https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key=KEY"; got != want {
		t.Fatalf("complete url = %q, want %q", got, want)
	}
	if got, want := p.buildURL("gemini-2.0-flash", "streamGenerateContent", true),
		"https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:streamGenerateContent?alt=sse&key=KEY"; got != want {
		t.Fatalf("stream url = %q, want %q", got, want)
	}
}

// NewServiceAccountTokenSource must mint a token from a service-account key via
// the JWT-bearer grant, and cache it (a second call within TTL must not refetch).
func TestServiceAccountTokenSource(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_ = r.ParseForm()
		if r.FormValue("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" || r.FormValue("assertion") == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, `{"access_token":"ya29.test","expires_in":3600}`)
	}))
	defer srv.Close()

	saJSON, _ := json.Marshal(map[string]string{
		"type":         "service_account",
		"client_email": "svc@proj.iam.gserviceaccount.com",
		"private_key":  string(keyPEM),
		"token_uri":    srv.URL,
	})

	ts, err := NewServiceAccountTokenSource(saJSON)
	if err != nil {
		t.Fatalf("NewServiceAccountTokenSource: %v", err)
	}
	tok, err := ts.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok != "ya29.test" {
		t.Fatalf("token = %q, want %q", tok, "ya29.test")
	}
	if _, err := ts.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Fatalf("token endpoint hit %d times, want 1 (cached)", hits)
	}
}

func TestServiceAccountTokenSourceRejectsBadJSON(t *testing.T) {
	if _, err := NewServiceAccountTokenSource([]byte(`{"client_email":""}`)); err == nil {
		t.Fatal("expected error for missing fields")
	}
}
