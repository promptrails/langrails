// Package awssig implements AWS Signature Version 4 request signing using only
// the standard library. It is intentionally minimal: just enough to sign the
// HTTP requests that the Bedrock provider makes, keeping langrails dependency-free.
//
// See https://docs.aws.amazon.com/general/latest/gr/sigv4_signing.html
package awssig

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	algorithm    = "AWS4-HMAC-SHA256"
	terminator   = "aws4_request"
	amzDateFmt   = "20060102T150405Z"
	shortDateFmt = "20060102"
)

// Credentials holds the AWS access credentials used for signing.
// SessionToken is optional and only set for temporary (STS) credentials.
type Credentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
}

// Signer signs HTTP requests for a specific AWS service and region.
type Signer struct {
	Credentials Credentials
	Region      string
	Service     string
}

// Sign adds the SigV4 authentication headers to req in place. payload is the
// exact request body bytes (use nil/empty for bodyless requests). now is the
// signing time; callers normally pass time.Now().UTC().
//
// The request must already have its URL and method set. Sign sets the Host,
// X-Amz-Date, X-Amz-Content-Sha256, optional X-Amz-Security-Token and
// Authorization headers.
func (s *Signer) Sign(req *http.Request, payload []byte, now time.Time) {
	now = now.UTC()
	amzDate := now.Format(amzDateFmt)
	shortDate := now.Format(shortDateFmt)

	payloadHash := hexSHA256(payload)

	if req.Host == "" {
		req.Host = req.URL.Host
	}
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	if s.Credentials.SessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", s.Credentials.SessionToken)
	}

	canonicalHeaders, signedHeaders := s.canonicalHeaders(req)

	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI(req),
		canonicalQuery(req),
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	scope := strings.Join([]string{shortDate, s.Region, s.Service, terminator}, "/")
	stringToSign := strings.Join([]string{
		algorithm,
		amzDate,
		scope,
		hexSHA256([]byte(canonicalRequest)),
	}, "\n")

	signingKey := s.signingKey(shortDate)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	auth := fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		algorithm, s.Credentials.AccessKeyID, scope, signedHeaders, signature)
	req.Header.Set("Authorization", auth)
}

// canonicalHeaders builds the canonical headers block and the signed-headers
// list. It signs host plus the x-amz-* headers that are always present.
func (s *Signer) canonicalHeaders(req *http.Request) (canonical, signed string) {
	headers := map[string]string{
		"host":                 req.Host,
		"x-amz-date":           req.Header.Get("X-Amz-Date"),
		"x-amz-content-sha256": req.Header.Get("X-Amz-Content-Sha256"),
	}
	if tok := req.Header.Get("X-Amz-Security-Token"); tok != "" {
		headers["x-amz-security-token"] = tok
	}

	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	for _, name := range names {
		b.WriteString(name)
		b.WriteByte(':')
		b.WriteString(strings.TrimSpace(headers[name]))
		b.WriteByte('\n')
	}
	return b.String(), strings.Join(names, ";")
}

func canonicalURI(req *http.Request) string {
	uri := req.URL.EscapedPath()
	if uri == "" {
		return "/"
	}
	return uri
}

func canonicalQuery(req *http.Request) string {
	// Query values are already percent-encoded by url.Values.Encode, which
	// also sorts by key. Bedrock requests are bodyless on the query string.
	return req.URL.Query().Encode()
}

func (s *Signer) signingKey(shortDate string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+s.Credentials.SecretAccessKey), []byte(shortDate))
	kRegion := hmacSHA256(kDate, []byte(s.Region))
	kService := hmacSHA256(kRegion, []byte(s.Service))
	return hmacSHA256(kService, []byte(terminator))
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func hexSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
