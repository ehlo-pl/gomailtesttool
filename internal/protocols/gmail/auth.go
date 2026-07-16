package gmail

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	calendarapi "google.golang.org/api/calendar/v3"
	gmailapi "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/ehlo-pl/gomailtesttool/internal/common/security"
)

// Per-action default OAuth scopes. Each must be a subset of the domain-wide
// delegation authorization granted in the Admin Console, or Google rejects the
// token exchange with unauthorized_client.
const (
	scopeGmailSend        = "https://www.googleapis.com/auth/gmail.send"
	scopeGmailReadonly    = "https://www.googleapis.com/auth/gmail.readonly"
	scopeCalendarReadonly = "https://www.googleapis.com/auth/calendar.readonly"
	scopeCalendarEvents   = "https://www.googleapis.com/auth/calendar.events"
)

// effectiveScopes returns the explicit --scope override if provided, otherwise
// the least-privilege scope set for the current action.
func effectiveScopes(config *Config) []string {
	if len(config.Scopes) > 0 {
		return config.Scopes
	}
	switch config.Action {
	case ActionSendMail:
		return []string{scopeGmailSend}
	case ActionGetInbox, ActionExportMessages:
		return []string{scopeGmailReadonly}
	case ActionGetEvents, ActionGetSchedule:
		return []string{scopeCalendarReadonly}
	case ActionSendInvite:
		return []string{scopeCalendarEvents}
	default: // testauth, exportbearertoken
		return []string{scopeGmailReadonly}
	}
}

// newTokenSource builds an OAuth2 token source using exactly one of the three
// supported authentication methods. Validation guarantees exactly one is set.
func newTokenSource(ctx context.Context, config *Config, scopes []string, logger *slog.Logger) (oauth2.TokenSource, error) {
	switch {
	case config.BearerToken != "":
		logDebug(logger, "Authentication method: pre-obtained bearer token")
		return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: config.BearerToken}), nil

	case config.OAuth:
		logDebug(logger, "Authentication method: interactive OAuth loopback flow")
		return oauthTokenSource(ctx, config, scopes, logger)

	case config.CredentialsFile != "":
		logDebug(logger, "Authentication method: service account + domain-wide delegation",
			"subject", security.MaskEmail(config.Mailbox))
		key, err := os.ReadFile(config.CredentialsFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read credentials file: %w", err)
		}
		conf, err := google.JWTConfigFromJSON(key, scopes...)
		if err != nil {
			return nil, fmt.Errorf("failed to parse service-account JSON: %w", err)
		}
		// Subject is MANDATORY for domain-wide delegation: without it the
		// service account authenticates as itself (no mailbox) and every call
		// fails. It is tied to --mailbox.
		conf.Subject = config.Mailbox
		return conf.TokenSource(ctx), nil

	default:
		return nil, fmt.Errorf("no authentication method configured")
	}
}

// newGmailService builds an authenticated Gmail API service for the action.
func newGmailService(ctx context.Context, config *Config, logger *slog.Logger) (*gmailapi.Service, error) {
	ts, err := newTokenSource(ctx, config, effectiveScopes(config), logger)
	if err != nil {
		return nil, err
	}
	svc, err := gmailapi.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("failed to create Gmail service: %w", err)
	}
	return svc, nil
}

// newCalendarService builds an authenticated Calendar API service for the action.
func newCalendarService(ctx context.Context, config *Config, logger *slog.Logger) (*calendarapi.Service, error) {
	ts, err := newTokenSource(ctx, config, effectiveScopes(config), logger)
	if err != nil {
		return nil, err
	}
	svc, err := calendarapi.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("failed to create Calendar service: %w", err)
	}
	return svc, nil
}

// oauthTokenSource runs (or reuses a cached) interactive OAuth loopback flow.
func oauthTokenSource(ctx context.Context, config *Config, scopes []string, logger *slog.Logger) (oauth2.TokenSource, error) {
	b, err := os.ReadFile(config.OAuthCredentials)
	if err != nil {
		return nil, fmt.Errorf("failed to read OAuth client-secret file: %w", err)
	}
	oauthConf, err := google.ConfigFromJSON(b, scopes...)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OAuth client-secret JSON: %w", err)
	}

	cachePath := config.TokenCache
	if cachePath == "" {
		cachePath = defaultTokenCachePath()
	}

	if tok, err := loadCachedToken(cachePath); err == nil {
		logDebug(logger, "Reusing cached OAuth token", "path", cachePath)
		return oauthConf.TokenSource(ctx, tok), nil
	}

	tok, err := tokenFromWeb(ctx, oauthConf, logger)
	if err != nil {
		return nil, err
	}
	if err := saveToken(cachePath, tok); err != nil {
		logWarn(logger, "Could not cache OAuth token", "path", cachePath, "error", err)
	}
	return oauthConf.TokenSource(ctx, tok), nil
}

// tokenFromWeb performs the loopback authorization-code flow: it starts a local
// listener, opens the consent URL in a browser, captures the redirected code,
// and exchanges it for a token.
func tokenFromWeb(ctx context.Context, conf *oauth2.Config, logger *slog.Logger) (*oauth2.Token, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to start local OAuth listener: %w", err)
	}
	defer func() { _ = listener.Close() }()

	conf.RedirectURL = fmt.Sprintf("http://%s/", listener.Addr().String())
	state, err := randomState()
	if err != nil {
		return nil, err
	}

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			errCh <- fmt.Errorf("OAuth state mismatch (possible CSRF)")
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "missing authorization code", http.StatusBadRequest)
			errCh <- fmt.Errorf("no authorization code returned")
			return
		}
		_, _ = fmt.Fprintln(w, "Authorization complete. You can close this window and return to the terminal.")
		codeCh <- code
	})

	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(listener) }()
	defer func() { _ = srv.Shutdown(context.Background()) }()

	authURL := conf.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	fmt.Printf("Open this URL in your browser to authorize gomailtest:\n\n%s\n\n", authURL)
	if err := openBrowser(authURL); err != nil {
		logDebug(logger, "Could not open browser automatically", "error", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-errCh:
		return nil, err
	case code := <-codeCh:
		tok, err := conf.Exchange(ctx, code)
		if err != nil {
			return nil, fmt.Errorf("token exchange failed: %w", err)
		}
		return tok, nil
	}
}

// randomState returns a URL-safe random string for CSRF protection.
func randomState() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate OAuth state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// defaultTokenCachePath returns the default on-disk OAuth token cache location
// under the OS user-config directory.
func defaultTokenCachePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "gomailtest", "gmail-token.json")
}

// loadCachedToken reads a cached OAuth token from path.
func loadCachedToken(path string) (*oauth2.Token, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	tok := &oauth2.Token{}
	if err := json.NewDecoder(f).Decode(tok); err != nil {
		return nil, err
	}
	return tok, nil
}

// saveToken writes an OAuth token to path with restrictive permissions.
func saveToken(path string, tok *oauth2.Token) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return json.NewEncoder(f).Encode(tok)
}

// openBrowser opens url in the platform's default browser.
func openBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}
