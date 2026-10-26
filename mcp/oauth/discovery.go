package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// serverMetadata is the part of RFC 8414 authorization server metadata
// the flows use.
type serverMetadata struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	RegistrationEndpoint  string `json:"registration_endpoint"`
}

// discover finds the authorization server for the resource and loads its
// metadata, once. The resource's protected resource metadata (RFC 9728)
// names the authorization server; without it, the resource's origin is
// assumed to be one. Without server metadata, the endpoints default to
// /authorize, /token and /register on the server's origin, as the
// 2025-03-26 MCP spec allows.
func (ts *TokenSource) discover(ctx context.Context) error {
	if ts.meta != nil {
		return nil
	}
	res, err := url.Parse(ts.resource)
	if err != nil || res.Scheme == "" || res.Host == "" {
		return fmt.Errorf("oauth: invalid server URL %q", ts.resource)
	}

	issuer := res.Scheme + "://" + res.Host
	var prm struct {
		AuthorizationServers []string `json:"authorization_servers"`
	}
	for _, u := range wellKnown(res, "oauth-protected-resource") {
		if ts.getJSON(ctx, u, &prm) == nil && len(prm.AuthorizationServers) > 0 {
			issuer = prm.AuthorizationServers[0]
			break
		}
	}

	iss, err := url.Parse(issuer)
	if err != nil || iss.Host == "" {
		return fmt.Errorf("oauth: invalid authorization server %q", issuer)
	}
	var meta serverMetadata
	candidates := append(wellKnown(iss, "oauth-authorization-server"), wellKnown(iss, "openid-configuration")...)
	candidates = append(candidates, strings.TrimSuffix(issuer, "/")+"/.well-known/openid-configuration")
	for _, u := range candidates {
		if ts.getJSON(ctx, u, &meta) == nil && meta.TokenEndpoint != "" {
			ts.meta = &meta
			return nil
		}
	}

	origin := iss.Scheme + "://" + iss.Host
	ts.meta = &serverMetadata{
		AuthorizationEndpoint: origin + "/authorize",
		TokenEndpoint:         origin + "/token",
		RegistrationEndpoint:  origin + "/register",
	}
	return nil
}

// wellKnown returns the well-known URLs for u: with u's path appended
// after the well-known suffix (RFC 8414 / RFC 9728 path insertion) and,
// when u has a path, at the origin root as a fallback.
func wellKnown(u *url.URL, suffix string) []string {
	origin := u.Scheme + "://" + u.Host
	path := strings.TrimSuffix(u.EscapedPath(), "/")
	urls := []string{origin + "/.well-known/" + suffix + path}
	if path != "" {
		urls = append(urls, origin+"/.well-known/"+suffix)
	}
	return urls
}

func (ts *TokenSource) getJSON(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := ts.cfg.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
}
