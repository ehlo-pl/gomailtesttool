package msgraph

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"
	"github.com/microsoftgraph/msgraph-sdk-go/users"

	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
)

// testAuth verifies the configured credential. Token acquisition is the
// pass/fail verdict — for app-only Graph, obtaining a token from Entra ID is
// what "can I authenticate?" means. The token's roles claim already reports the
// granted application permissions, so no Graph data call is required.
//
// When --mailbox is supplied, one lightweight authenticated Graph call is made
// as supplementary end-to-end verification. See verifyMailboxAccess for why a
// 403 there is reported as success rather than failure.
func testAuth(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	fmt.Println("Testing Microsoft Graph authentication...")

	writeAuthHeader(csvLogger, slogLogger)

	cred, err := getCredential(config, slogLogger)
	if err != nil {
		writeAuthCSV(csvLogger, slogLogger, config, "", "", "", "", err.Error())
		return err
	}

	scopes := effectiveScopes(config)

	// Token acquisition is the actual authentication test.
	token, err := cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: scopes, EnableCAE: true})
	if err != nil {
		fmt.Printf("✗ Authentication failed: %s\n", err)
		writeAuthCSV(csvLogger, slogLogger, config, "", "", "", "", err.Error())
		logError(slogLogger, "testauth failed during token acquisition", "error", err)
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Decode claims for display (token is already validated by the credential).
	appName, roles, delegatedScopes, claimErr := parseTokenClaims(token.Token)
	if claimErr != nil {
		appName, roles, delegatedScopes = "(unavailable)", "(unavailable)", "(unavailable)"
	}
	tokenExpiry := token.ExpiresOn.Format("2006-01-02 15:04:05 MST")

	// Optional supplementary end-to-end check.
	mailboxCheck := "not checked (no --mailbox provided)"
	graphAccepted := false
	if config.Mailbox != "" {
		mailboxCheck, graphAccepted, err = verifyMailboxAccess(ctx, cred, scopes, config, csvLogger, slogLogger)
		if err != nil {
			// Only a genuine token rejection (401) reaches here.
			fmt.Printf("✗ %s\n", err)
			writeAuthCSV(csvLogger, slogLogger, config, appName, roles, tokenExpiry, "FAILURE", err.Error())
			return err
		}
	}

	// A pre-obtained bearer token (--bearertoken) is echoed by
	// BearerTokenCredential without contacting Entra ID, so token acquisition
	// alone does not verify it. It is only verified when Graph accepts it (the
	// --mailbox check). Every other credential flow acquires the token from
	// Entra ID, so acquisition is itself the verification.
	verified := config.BearerToken == "" || graphAccepted

	if config.OutputFormat == "json" {
		printJSON(map[string]interface{}{
			"action":          ActionTestAuth,
			"status":          StatusSuccess,
			"verified":        verified,
			"appName":         appName,
			"assignedRoles":   roles,
			"delegatedScopes": delegatedScopes,
			"tokenExpiry":     tokenExpiry,
			"mailboxCheck":    mailboxCheck,
		})
	} else {
		if verified {
			fmt.Println("✓ Authentication successful")
		} else {
			fmt.Println("⚠ Pre-obtained token accepted locally but NOT verified against Microsoft Graph")
			fmt.Println("  (--bearertoken is not validated by acquisition; pass --mailbox to verify it end to end)")
		}
		fmt.Printf("  Application:      %s\n", appName)
		fmt.Printf("  Assigned Roles:   %s\n", roles)
		if config.Delegated {
			fmt.Printf("  Delegated Scopes: %s\n", delegatedScopes)
		}
		fmt.Printf("  Token Expires:    %s\n", tokenExpiry)
		fmt.Printf("  Mailbox Check:    %s\n", mailboxCheck)
	}

	logVerbose(config.VerboseMode, "testauth completed: app=%q roles=%q", appName, roles)

	writeAuthCSV(csvLogger, slogLogger, config, appName, roles, tokenExpiry, mailboxCheck, "")
	return nil
}

// verifyMailboxAccess makes one lightweight authenticated Graph call
// (GET /users/{mailbox}?$select=id,mail,displayName) to confirm the token is
// accepted end-to-end. It reuses the already-built credential so the token is
// not acquired twice.
//
// Interpreting the result matters: app-only Graph has no universally-available
// authenticated endpoint — every call needs a specific role, and this tool's
// apps typically hold Mail.*/Calendars.*, not User.Read.All. So:
//   - 2xx  → full success (token valid, Graph reachable, mailbox readable)
//   - 403  → token was accepted by Graph; the app just isn't authorized for this
//     resource → reported as success-with-note, NOT a failure
//   - 401  → token rejected → hard failure (returns an error)
//   - other → auth still considered verified; reported as a warning
//
// The returned bool reports whether Graph actually accepted the token (2xx or
// 403), which is the only real end-to-end verification for a pre-obtained
// bearer token — see testAuth.
func verifyMailboxAccess(ctx context.Context, cred azcore.TokenCredential, scopes []string, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) (string, bool, error) {
	client, err := msgraphsdk.NewGraphServiceClientWithCredentials(cred, scopes)
	if err != nil {
		return fmt.Sprintf("skipped (client init failed: %v)", err), false, nil
	}

	requestConfig := &users.UserItemRequestBuilderGetRequestConfiguration{
		QueryParameters: &users.UserItemRequestBuilderGetQueryParameters{
			Select: []string{"id", "mail", "displayName"},
		},
	}

	var user models.Userable
	callErr := retryWithBackoff(ctx, config.MaxRetries, config.RetryDelay, func() error {
		u, apiErr := client.Users().ByUserId(config.Mailbox).Get(ctx, requestConfig)
		if apiErr == nil {
			user = u
		}
		return apiErr
	})

	if callErr == nil {
		return fmt.Sprintf("SUCCESS (read %s)", derefOr(user.GetMail(), config.Mailbox)), true, nil
	}

	var odataErr *odataerrors.ODataError
	if errors.As(callErr, &odataErr) {
		switch odataErr.ResponseStatusCode {
		case http.StatusForbidden:
			fmt.Println("  (Mailbox read returned 403 — token accepted; app lacks User.Read.All. Expected unless that permission is granted.)")
			return "token accepted; app lacks User.Read.All (403, expected)", true, nil
		case http.StatusUnauthorized:
			enriched := enrichGraphAPIError(callErr, csvLogger, "testauth")
			return "", false, fmt.Errorf("token rejected by Graph (401): %w", enriched)
		}
	}

	// Any other error (network, throttling exhausted): Graph did not confirm the
	// token, but authentication is not proven failed either. Surface it as a
	// warning rather than failing.
	logError(slogLogger, "testauth mailbox check warning", "error", callErr)
	return fmt.Sprintf("warning: mailbox check failed (%v)", callErr), false, nil
}

func writeAuthHeader(csvLogger logger.Logger, slogLogger *slog.Logger) {
	if csvLogger == nil {
		return
	}
	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		if err := csvLogger.WriteHeader([]string{
			"Action", "Status", "App_Name", "Assigned_Roles",
			"Token_Expiry", "Mailbox_Check", "Error",
		}); err != nil {
			logError(slogLogger, "Failed to write CSV header", "error", err)
		}
	}
}

func writeAuthCSV(csvLogger logger.Logger, slogLogger *slog.Logger, config *Config, appName, roles, tokenExpiry, mailboxCheck, errStr string) {
	if csvLogger == nil {
		return
	}
	status := StatusSuccess
	if errStr != "" {
		status = StatusError
	}
	if logErr := csvLogger.WriteRow([]string{
		config.Action, status, appName, roles,
		tokenExpiry, mailboxCheck, errStr,
	}); logErr != nil {
		logError(slogLogger, "Failed to write CSV row", "error", logErr)
	}
}
