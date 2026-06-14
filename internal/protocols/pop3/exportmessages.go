package pop3

import (
	"context"
	"fmt"
	"log/slog"
	"net/mail"
	"os"
	"path/filepath"
	"strings"

	"github.com/ziembor/gomailtesttool/internal/common/export"
	"github.com/ziembor/gomailtesttool/internal/common/logger"
)

// exportMessages fetches headers for each message via TOP, matches them
// against config.MessageID and/or config.Subject, and exports matches (via
// RETR) as raw .eml (RFC822) files.
func exportMessages(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	fmt.Printf("Searching mailbox on %s:%d for matching messages...\n", config.Host, config.Port)

	columns := []string{"Action", "Status", "Server", "Port", "Message_Number", "Subject", "Filename", "Error"}
	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		if err := csvLogger.WriteHeader(columns); err != nil {
			logger.LogError(slogLogger, "Failed to write CSV header", "error", err)
		}
	}

	client := NewPOP3Client(config)

	if err := client.Connect(ctx); err != nil {
		logger.LogError(slogLogger, "Connection failed", "error", err, "host", config.Host, "port", config.Port)
		writeExportRow(csvLogger, slogLogger, config, false, "", "", "", err.Error())
		return fmt.Errorf("connection failed: %w", err)
	}
	defer func() { _ = client.Quit() }()

	fmt.Printf("✓ Connected to %s:%d\n", config.Host, config.Port)

	if config.StartTLS && client.GetTLSState() == nil {
		fmt.Println("Upgrading to TLS via STLS...")
		if err := client.StartTLS(nil); err != nil {
			logger.LogError(slogLogger, "STLS upgrade failed", "error", err)
			writeExportRow(csvLogger, slogLogger, config, false, "", "", "", fmt.Sprintf("STLS failed: %v", err))
			return fmt.Errorf("STLS failed: %w", err)
		}
		fmt.Println("✓ TLS upgrade successful")
	}

	caps, _ := client.Capabilities(ctx)

	authMethod := config.AuthMethod
	if strings.EqualFold(authMethod, "auto") {
		if config.AccessToken != "" {
			if caps != nil && caps.SupportsXOAUTH2() {
				authMethod = "XOAUTH2"
			} else {
				authMethod = "USER"
			}
		} else {
			authMethod = "USER"
		}
	}

	var authErr error
	if config.AccessToken != "" && strings.EqualFold(authMethod, "XOAUTH2") {
		authErr = client.Auth(ctx, config.Username, "", config.AccessToken)
	} else {
		authErr = client.Auth(ctx, config.Username, config.Password, "")
	}
	if authErr != nil {
		logger.LogError(slogLogger, "Authentication failed", "error", authErr, "username", maskUsername(config.Username))
		writeExportRow(csvLogger, slogLogger, config, false, "", "", "", fmt.Sprintf("Auth failed: %v", authErr))
		return fmt.Errorf("authentication failed: %w", authErr)
	}
	fmt.Println("✓ Authentication successful")

	messages, err := client.List(ctx)
	if err != nil {
		logger.LogError(slogLogger, "LIST command failed", "error", err)
		writeExportRow(csvLogger, slogLogger, config, false, "", "", "", fmt.Sprintf("LIST failed: %v", err))
		return fmt.Errorf("LIST failed: %w", err)
	}

	exportDir, err := export.CreateExportDir(config.ExportDir)
	if err != nil {
		return err
	}
	fmt.Printf("Export directory: %s\n", exportDir)

	wantMessageID := strings.Trim(config.MessageID, "<>")
	wantSubject := strings.ToLower(config.Subject)

	successCount := 0
	for _, m := range messages {
		if successCount >= config.Count {
			break
		}

		header, err := client.Top(ctx, m.Number, 0)
		if err != nil {
			logger.LogError(slogLogger, "TOP failed", "error", err, "msgnum", m.Number)
			continue
		}

		parsed, err := mail.ReadMessage(strings.NewReader(string(header) + "\r\n"))
		if err != nil {
			logger.LogError(slogLogger, "Failed to parse message headers", "error", err, "msgnum", m.Number)
			continue
		}

		subject := parsed.Header.Get("Subject")
		messageID := strings.Trim(parsed.Header.Get("Message-Id"), "<>")

		if !matchesCriteria(messageID, subject, wantMessageID, wantSubject) {
			continue
		}

		body, err := client.Retr(ctx, m.Number)
		if err != nil {
			logger.LogError(slogLogger, "RETR failed", "error", err, "msgnum", m.Number)
			writeExportRow(csvLogger, slogLogger, config, false, fmt.Sprintf("%d", m.Number), subject, "", err.Error())
			continue
		}

		name := messageID
		if name == "" {
			name = fmt.Sprintf("msg_%d", m.Number)
		}
		filename := fmt.Sprintf("msg_%s.eml", export.SanitizeFilename(name))
		filePath := filepath.Join(exportDir, filename)
		if err := os.WriteFile(filePath, body, 0644); err != nil {
			logger.LogError(slogLogger, "Failed to write EML file", "error", err, "msgnum", m.Number)
			writeExportRow(csvLogger, slogLogger, config, false, fmt.Sprintf("%d", m.Number), subject, "", err.Error())
			continue
		}

		successCount++
		fmt.Printf("Successfully exported message %d -> %s\n", m.Number, filePath)
		writeExportRow(csvLogger, slogLogger, config, true, fmt.Sprintf("%d", m.Number), subject, filePath, "")
	}

	if successCount == 0 {
		fmt.Println("No messages found matching the given criteria.")
		writeExportRow(csvLogger, slogLogger, config, true, "", "", "", "No messages found (0 messages)")
		return nil
	}

	fmt.Printf("Successfully exported %d message(s).\n", successCount)
	return nil
}

// matchesCriteria reports whether a message's Message-Id/Subject headers
// satisfy the search criteria. At least one of wantMessageID/wantSubject is
// non-empty (enforced by validateConfiguration); both given criteria must
// match if both are provided.
func matchesCriteria(messageID, subject, wantMessageID, wantSubject string) bool {
	if wantMessageID != "" && messageID != wantMessageID {
		return false
	}
	if wantSubject != "" && !strings.Contains(strings.ToLower(subject), wantSubject) {
		return false
	}
	return true
}

// writeExportRow writes a single CSV row for the exportmessages action.
func writeExportRow(csvLogger logger.Logger, slogLogger *slog.Logger, config *Config, success bool, msgNum, subject, filename, errStr string) {
	status := "FAILURE"
	if success {
		status = "SUCCESS"
	}
	if logErr := csvLogger.WriteRow([]string{
		ActionExportMessages, status, config.Host, fmt.Sprintf("%d", config.Port),
		msgNum, subject, filename, errStr,
	}); logErr != nil {
		logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
	}
}
