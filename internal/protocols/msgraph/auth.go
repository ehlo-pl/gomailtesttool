package msgraph

import (
	"context"
	"crypto"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/ehlo-pl/gomailtesttool/internal/common/security"
	"github.com/golang-jwt/jwt/v5"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"software.sslmate.com/src/go-pkcs12"
)

// TokenClaims represents relevant claims from Microsoft Entra ID JWT tokens
type TokenClaims struct {
	AppDisplayName       string   `json:"app_displayname"` // Application display name from Entra ID
	Roles                []string `json:"roles"`           // Assigned application roles (e.g., Mail.ReadWrite)
	Scopes               string   `json:"scp"`             // Delegated scopes (space-delimited)
	jwt.RegisteredClaims          // Standard JWT claims (exp, iss, etc.)
}

// BearerTokenCredential implements azcore.TokenCredential for pre-obtained bearer tokens.
// This allows using tokens obtained externally (e.g., via OAuth2 flows, CLI tools).
type BearerTokenCredential struct {
	token string
}

// NewBearerTokenCredential creates a new credential from a pre-obtained bearer token.
func NewBearerTokenCredential(token string) *BearerTokenCredential {
	return &BearerTokenCredential{token: token}
}

// GetToken returns the pre-obtained bearer token. Since the token is pre-obtained,
// expiration is not tracked - users should ensure the token is valid.
func (c *BearerTokenCredential) GetToken(ctx context.Context, options policy.TokenRequestOptions) (azcore.AccessToken, error) {
	// Parse expiration from JWT if possible, otherwise set a default
	expiresOn := time.Now().Add(1 * time.Hour) // Default: 1 hour from now

	// Try to extract expiration from JWT claims
	token, _, err := new(jwt.Parser).ParseUnverified(c.token, &TokenClaims{})
	if err == nil {
		if claims, ok := token.Claims.(*TokenClaims); ok {
			if claims.ExpiresAt != nil {
				expiresOn = claims.ExpiresAt.Time
			}
		}
	}

	return azcore.AccessToken{
		Token:     c.token,
		ExpiresOn: expiresOn,
	}, nil
}

// NewGraphServiceClient creates credentials and initializes the Microsoft Graph SDK client.
func NewGraphServiceClient(ctx context.Context, config *Config, logger *slog.Logger) (*msgraphsdk.GraphServiceClient, error) {
	// Setup Authentication
	logDebug(logger, "Setting up Microsoft Graph client", "tenantID", security.MaskGUID(config.TenantID), "clientID", security.MaskGUID(config.ClientID))
	scopes := effectiveScopes(config)

	cred, err := getCredential(config.TenantID, config.ClientID, config.Secret, config.PfxPath, config.PfxPass, config.Thumbprint, config, logger)
	if err != nil {
		return nil, fmt.Errorf("authentication setup failed: %w", err)
	}

	// Get and display token information if verbose
	if config.VerboseMode {
		token, err := cred.GetToken(ctx, policy.TokenRequestOptions{
			Scopes: scopes,
		})
		if err != nil {
			logVerbose(config.VerboseMode, "Warning: Could not retrieve token for verbose display: %v", err)
		} else {
			printTokenInfo(token)
		}
	}

	client, err := msgraphsdk.NewGraphServiceClientWithCredentials(cred, scopes)
	if err != nil {
		return nil, fmt.Errorf("graph client initialization failed: %w", err)
	}

	if config.VerboseMode {
		logVerbose(config.VerboseMode, "Graph SDK client initialized successfully")
		logVerbose(config.VerboseMode, "Target scopes: %s", strings.Join(scopes, ", "))
	}

	return client, nil
}

func getCredential(tenantID, clientID, secret, pfxPath, pfxPass, thumbprint string, config *Config, logger *slog.Logger) (azcore.TokenCredential, error) {
	if config.Delegated {
		return getDelegatedCredential(config, logger)
	}
	// 1. Bearer Token (pre-obtained)
	if config.BearerToken != "" {
		logDebug(logger, "Authentication method: Bearer Token")
		logDebug(logger, "Using pre-obtained bearer token")
		return NewBearerTokenCredential(config.BearerToken), nil
	}

	// 2. Client Secret
	if secret != "" {
		logDebug(logger, "Authentication method: Client Secret")
		logDebug(logger, "Creating ClientSecretCredential")
		return azidentity.NewClientSecretCredential(tenantID, clientID, secret, nil)
	}

	// 3. PFX File
	if pfxPath != "" {
		logDebug(logger, "Authentication method: PFX Certificate File", "path", pfxPath)
		pfxData, err := os.ReadFile(pfxPath)
		if err != nil {
			logError(logger, "Failed to read PFX file", "path", pfxPath, "error", err)
			return nil, fmt.Errorf("failed to read PFX file: %w", err)
		}
		logDebug(logger, "PFX file read successfully", "bytes", len(pfxData))
		return createCertCredential(tenantID, clientID, pfxData, pfxPass, logger)
	}

	// 4. Windows Cert Store (Thumbprint)
	if thumbprint != "" {
		logDebug(logger, "Authentication method: Windows Certificate Store", "thumbprint", thumbprint)
		logDebug(logger, "Exporting certificate from CurrentUser\\My store")
		pfxData, tempPass, err := exportCertFromStore(thumbprint)
		if err != nil {
			return nil, fmt.Errorf("failed to export cert from store: %w", err)
		}
		logDebug(logger, "Certificate exported successfully", "bytes", len(pfxData))
		return createCertCredential(tenantID, clientID, pfxData, tempPass, logger)
	}

	return nil, fmt.Errorf("no valid authentication method provided (use -secret, -pfx, -thumbprint, or -bearertoken)")
}

func getDelegatedCredential(config *Config, logger *slog.Logger) (azcore.TokenCredential, error) {
	// 1. Access token (pre-obtained)
	if config.BearerToken != "" {
		logDebug(logger, "Delegated authentication method: Access token")
		return NewBearerTokenCredential(config.BearerToken), nil
	}

	switch config.AuthFlow {
	case "", "devicecode":
		logDebug(logger, "Delegated authentication method: Device Code")
		return azidentity.NewDeviceCodeCredential(&azidentity.DeviceCodeCredentialOptions{
			TenantID: config.TenantID,
			ClientID: config.ClientID,
			UserPrompt: func(ctx context.Context, message azidentity.DeviceCodeMessage) error {
				_, err := fmt.Fprintln(os.Stderr, message.Message)
				return err
			},
		})
	case "browser":
		logDebug(logger, "Delegated authentication method: Interactive Browser")
		return azidentity.NewInteractiveBrowserCredential(&azidentity.InteractiveBrowserCredentialOptions{
			TenantID:    config.TenantID,
			ClientID:    config.ClientID,
			RedirectURL: config.RedirectURL,
		})
	default:
		return nil, fmt.Errorf("unsupported delegated auth flow: %s", config.AuthFlow)
	}
}

func effectiveScopes(config *Config) []string {
	if config.Delegated {
		if len(config.Scopes) > 0 {
			return append([]string(nil), config.Scopes...)
		}
		return []string{
			"https://graph.microsoft.com/Mail.ReadWrite",
			"https://graph.microsoft.com/Mail.Send",
			"https://graph.microsoft.com/Calendars.ReadWrite",
			"offline_access",
		}
	}
	return []string{"https://graph.microsoft.com/.default"}
}

func createCertCredential(tenantID, clientID string, pfxData []byte, password string, logger *slog.Logger) (*azidentity.ClientCertificateCredential, error) {
	// Decode PFX using go-pkcs12 library (supports SHA-256 and other modern algorithms)
	// pkcs12.DecodeChain returns private key and full certificate chain
	key, cert, caCerts, err := pkcs12.DecodeChain(pfxData, password)
	if err != nil {
		return nil, fmt.Errorf("failed to decode PFX: %w", err)
	}

	// Ensure key is a crypto.PrivateKey (it should be)
	privKey, ok := key.(crypto.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("decoded key is not a valid crypto.PrivateKey")
	}

	// Build certificate chain: primary cert + CA certs
	// azidentity expects a slice of certs with the leaf certificate first
	certs := []*x509.Certificate{cert}
	if len(caCerts) > 0 {
		certs = append(certs, caCerts...)
	}

	// Options - send full certificate chain for better compatibility
	opts := &azidentity.ClientCertificateCredentialOptions{
		SendCertificateChain: true,
	}

	// Create Credential
	return azidentity.NewClientCertificateCredential(tenantID, clientID, certs, privKey, opts)
}

// Print token information
func printTokenInfo(token azcore.AccessToken) {
	fmt.Println()
	fmt.Println("Token Information:")
	fmt.Println("------------------")
	fmt.Printf("Token acquired successfully\n")
	fmt.Printf("Expires at: %s\n", token.ExpiresOn.Format("2006-01-02 15:04:05 MST"))

	// Calculate time until expiration
	timeUntilExpiry := time.Until(token.ExpiresOn)
	fmt.Printf("Valid for: %s\n", timeUntilExpiry.Round(time.Second))

	// Show truncated token (always truncate for security, even short tokens)
	tokenStr := token.Token
	if len(tokenStr) > 40 {
		fmt.Printf("Token (truncated): %s...%s\n", tokenStr[:20], tokenStr[len(tokenStr)-20:])
	} else {
		// Even short tokens should be masked for security
		maxLen := 10
		if len(tokenStr) < maxLen {
			maxLen = len(tokenStr)
		}
		fmt.Printf("Token (truncated): %s...\n", tokenStr[:maxLen])
	}
	fmt.Printf("Token length: %d characters\n", len(tokenStr))

	// Parse and display JWT claims (application name and roles)
	fmt.Println()
	fmt.Println("JWT Claims:")
	appName, roles, err := parseTokenClaims(tokenStr)
	if err != nil {
		fmt.Printf("  (Could not parse JWT claims: %v)\n", err)
	} else {
		fmt.Printf("  Application Name: %s\n", appName)
		fmt.Printf("  Assigned Roles: %s\n", roles)
	}

	fmt.Println()
}

// parseTokenClaims extracts application name and assigned roles from a JWT access token.
func parseTokenClaims(tokenString string) (string, string, error) {
	// Parse without verification (token already validated by Azure SDK)
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, &TokenClaims{})
	if err != nil {
		return "", "", fmt.Errorf("failed to parse JWT: %w", err)
	}

	claims, ok := token.Claims.(*TokenClaims)
	if !ok {
		return "", "", fmt.Errorf("failed to extract claims from token")
	}

	// Extract app display name (may be empty)
	appName := claims.AppDisplayName
	if appName == "" {
		appName = "(not available)"
	}

	// Extract roles (may be empty array)
	rolesStr := "(none)"
	if len(claims.Roles) > 0 {
		rolesStr = strings.Join(claims.Roles, ", ")
	}

	return appName, rolesStr, nil
}
