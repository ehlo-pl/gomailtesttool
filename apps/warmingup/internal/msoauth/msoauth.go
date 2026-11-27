// Package msoauth signs in to Outlook.com once with the OAuth2 device code
// flow and afterwards supplies IMAP access tokens unattended from the stored
// refresh token.
package msoauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/endpoints"
)

// Scopes needed for IMAP plus a refresh token.
var Scopes = []string{"https://outlook.office.com/IMAP.AccessAsUser.All", "offline_access"}

// DefaultTenant admits personal Microsoft accounts.
const DefaultTenant = "consumers"

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)

// Account is the sign-in of one mailbox endpoint.
type Account struct {
	ClientID  string
	TokenFile string
	// Endpoint defaults to the Microsoft identity platform; tests override it.
	Endpoint oauth2.Endpoint
}

// New returns the account of the endpoint called name, keeping its token in
// <datadir>/tokens/<name>.json.
func New(datadir, name, clientID, tenant string) (*Account, error) {
	if clientID == "" {
		return nil, errors.New("'clientid' of an app registration is required")
	}
	if !namePattern.MatchString(name) {
		return nil, fmt.Errorf("endpoint name %q cannot be used as a file name", name)
	}
	if tenant == "" {
		tenant = DefaultTenant
	}
	ep := endpoints.AzureAD(tenant)
	ep.AuthStyle = oauth2.AuthStyleInParams // public client: no secret
	return &Account{ClientID: clientID, TokenFile: filepath.Join(datadir, "tokens", name+".json"), Endpoint: ep}, nil
}

func (a *Account) config() *oauth2.Config {
	return &oauth2.Config{ClientID: a.ClientID, Endpoint: a.Endpoint, Scopes: Scopes}
}

// Login runs the device code flow, printing the instructions to out.
func (a *Account) Login(ctx context.Context, out io.Writer) error {
	cfg := a.config()
	da, err := cfg.DeviceAuth(ctx)
	if err != nil {
		return fmt.Errorf("start device sign-in: %w", err)
	}
	if _, err := fmt.Fprintf(out, "Open %s and enter the code %s (sign in with the mailbox's account).\n", da.VerificationURI, da.UserCode); err != nil {
		return err
	}
	tok, err := cfg.DeviceAccessToken(ctx, da)
	if err != nil {
		return fmt.Errorf("device sign-in: %w", err)
	}
	if tok.RefreshToken == "" {
		return errors.New("no refresh token was returned; the offline_access scope is required")
	}
	return a.save(tok)
}

// AccessToken returns a valid access token, refreshing it when needed and
// storing rotated refresh tokens. It never prompts.
func (a *Account) AccessToken(ctx context.Context) (string, error) {
	data, err := os.ReadFile(a.TokenFile)
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("not signed in: run \"warmingup auth\" for this endpoint (no %s)", a.TokenFile)
	}
	if err != nil {
		return "", err
	}
	var stored oauth2.Token
	if err := json.Unmarshal(data, &stored); err != nil {
		return "", fmt.Errorf("token file %s: %w", a.TokenFile, err)
	}
	tok, err := a.config().TokenSource(ctx, &stored).Token()
	if err != nil {
		return "", fmt.Errorf("refresh sign-in (run \"warmingup auth\" again if it expired): %w", err)
	}
	if tok.AccessToken != stored.AccessToken || tok.RefreshToken != stored.RefreshToken {
		if err := a.save(tok); err != nil {
			return "", fmt.Errorf("store refreshed token: %w", err)
		}
	}
	return tok.AccessToken, nil
}

// save replaces the token file atomically with owner-only permissions.
func (a *Account) save(tok *oauth2.Token) error {
	data, err := json.Marshal(tok)
	if err != nil {
		return err
	}
	dir := filepath.Dir(a.TokenFile)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "token-*") // created 0600
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err = errors.Join(err, f.Close()); err == nil {
		err = os.Rename(f.Name(), a.TokenFile)
	}
	if err != nil {
		_ = os.Remove(f.Name())
	}
	return err
}
