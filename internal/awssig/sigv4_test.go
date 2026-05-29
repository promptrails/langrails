package awssig

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"
)

func testSigner() *Signer {
	return &Signer{
		Credentials: Credentials{
			AccessKeyID:     "AKIDEXAMPLE",
			SecretAccessKey: "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY",
		},
		Region:  "us-east-1",
		Service: "bedrock",
	}
}

func newReq(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost,
		"https://bedrock-runtime.us-east-1.amazonaws.com/model/anthropic.claude/converse", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	return req
}

func TestSign_SetsRequiredHeaders(t *testing.T) {
	s := testSigner()
	req := newReq(t)
	s.Sign(req, []byte(`{"hello":"world"}`), time.Unix(1700000000, 0))

	if got := req.Header.Get("X-Amz-Date"); got == "" {
		t.Error("missing X-Amz-Date")
	}
	if got := req.Header.Get("X-Amz-Content-Sha256"); len(got) != 64 {
		t.Errorf("X-Amz-Content-Sha256 should be 64 hex chars, got %q", got)
	}

	auth := req.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 ") {
		t.Errorf("unexpected algorithm prefix: %q", auth)
	}
	if !strings.Contains(auth, "Credential=AKIDEXAMPLE/") {
		t.Errorf("missing credential: %q", auth)
	}
	if !strings.Contains(auth, "/us-east-1/bedrock/aws4_request") {
		t.Errorf("missing/incorrect scope: %q", auth)
	}
	if !strings.Contains(auth, "SignedHeaders=host;x-amz-content-sha256;x-amz-date") {
		t.Errorf("unexpected signed headers: %q", auth)
	}

	sig := regexp.MustCompile(`Signature=([0-9a-f]{64})$`)
	if !sig.MatchString(auth) {
		t.Errorf("signature should be 64 lowercase hex chars: %q", auth)
	}
}

func TestSign_Deterministic(t *testing.T) {
	body := []byte(`{"a":1}`)
	at := time.Unix(1700000000, 0)

	s := testSigner()
	r1 := newReq(t)
	s.Sign(r1, body, at)

	r2 := newReq(t)
	s.Sign(r2, body, at)

	if r1.Header.Get("Authorization") != r2.Header.Get("Authorization") {
		t.Error("signing is not deterministic for identical inputs")
	}
}

func TestSign_VariesWithInputs(t *testing.T) {
	body := []byte(`{"a":1}`)
	at := time.Unix(1700000000, 0)
	base := testSigner()
	baseReq := newReq(t)
	base.Sign(baseReq, body, at)
	baseAuth := baseReq.Header.Get("Authorization")

	cases := map[string]func(*testing.T){
		"different body": func(t *testing.T) {
			r := newReq(t)
			base.Sign(r, []byte(`{"a":2}`), at)
			if r.Header.Get("Authorization") == baseAuth {
				t.Error("signature unchanged for different body")
			}
		},
		"different time": func(t *testing.T) {
			r := newReq(t)
			base.Sign(r, body, at.Add(time.Hour))
			if r.Header.Get("Authorization") == baseAuth {
				t.Error("signature unchanged for different time")
			}
		},
		"different secret": func(t *testing.T) {
			s := testSigner()
			s.Credentials.SecretAccessKey = "another-secret"
			r := newReq(t)
			s.Sign(r, body, at)
			if r.Header.Get("Authorization") == baseAuth {
				t.Error("signature unchanged for different secret")
			}
		},
	}
	for name, fn := range cases {
		t.Run(name, fn)
	}
}

func TestSign_IncludesSessionToken(t *testing.T) {
	s := testSigner()
	s.Credentials.SessionToken = "session-token-value"
	req := newReq(t)
	s.Sign(req, nil, time.Unix(1700000000, 0))

	if req.Header.Get("X-Amz-Security-Token") != "session-token-value" {
		t.Error("session token not set on request")
	}
	if !strings.Contains(req.Header.Get("Authorization"), "x-amz-security-token") {
		t.Error("session token not included in signed headers")
	}
}
