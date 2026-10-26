package oauth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Loopback returns an Authorizer for desktop and CLI apps: it listens on
// redirectURL (which must be an http://127.0.0.1 or http://localhost
// address), calls open with the authorization URL (typically to launch a
// browser, or to print it), and waits for the redirect. The redirect's
// state must match the authorization URL's.
func Loopback(redirectURL string, open func(authURL string) error) Authorizer {
	return func(ctx context.Context, authURL string) (string, error) {
		redirect, err := url.Parse(redirectURL)
		if err != nil {
			return "", fmt.Errorf("redirect URL: %w", err)
		}
		if redirect.Scheme != "http" || (redirect.Hostname() != "127.0.0.1" && redirect.Hostname() != "localhost") {
			return "", fmt.Errorf("redirect URL %q is not a loopback address", redirectURL)
		}
		auth, err := url.Parse(authURL)
		if err != nil {
			return "", err
		}
		state := auth.Query().Get("state")

		var lc net.ListenConfig
		ln, err := lc.Listen(ctx, "tcp", redirect.Host)
		if err != nil {
			return "", err
		}

		type result struct {
			code string
			err  error
		}
		done := make(chan result, 1)
		path := redirect.Path
		if path == "" {
			path = "/"
		}
		mux := http.NewServeMux()
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			var res result
			switch {
			case q.Get("state") != state:
				res.err = errors.New("state mismatch in redirect")
			case q.Get("error") != "":
				res.err = fmt.Errorf("%s: %s", q.Get("error"), q.Get("error_description"))
			case q.Get("code") == "":
				res.err = errors.New("no code in redirect")
			default:
				res.code = q.Get("code")
			}
			if res.err != nil {
				http.Error(w, "Authorization failed. You can close this window.", http.StatusBadRequest)
			} else {
				_, _ = w.Write([]byte("Authorization complete. You can close this window."))
			}
			select {
			case done <- res:
			default:
			}
		})
		srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
		go func() { _ = srv.Serve(ln) }()
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_ = srv.Shutdown(shutdownCtx)
		}()

		if err := open(authURL); err != nil {
			return "", err
		}
		select {
		case res := <-done:
			return res.code, res.err
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}
