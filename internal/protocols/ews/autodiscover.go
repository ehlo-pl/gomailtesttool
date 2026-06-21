package ews

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
)

// Autodiscover SOAP response structures
type autodiscoverResponseEnvelope struct {
	XMLName struct{}                 `xml:"Envelope"`
	Body    autodiscoverResponseBody `xml:"Body"`
}

type autodiscoverResponseBody struct {
	GetUserSettingsResponse getUserSettingsResponse `xml:"GetUserSettingsResponseMessage"`
}

type getUserSettingsResponse struct {
	Response getUserSettingsResponseInner `xml:"Response"`
}

type getUserSettingsResponseInner struct {
	ErrorCode    string          `xml:"ErrorCode"`
	ErrorMessage string          `xml:"ErrorMessage"`
	UserSettings userSettingList `xml:"UserResponses>UserResponse"`
}

type userSettingList struct {
	UserResponse userSettingResponse `xml:"UserResponse"`
}

type userSettingResponse struct {
	Mailbox      string        `xml:"Mailbox"`
	ErrorCode    string        `xml:"ErrorCode"`
	ErrorMessage string        `xml:"ErrorMessage"`
	UserSettings []userSetting `xml:"UserSettings>UserSetting"`
}

type userSetting struct {
	Name  string `xml:"Name"`
	Value string `xml:"Value"`
}

// autodiscover sends a SOAP GetUserSettings request to the Autodiscover endpoint.
func autodiscover(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	fmt.Printf("Testing EWS Autodiscover for %s\n", config.Username)

	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		if err := csvLogger.WriteHeader([]string{
			"Action", "Status", "Server", "Port", "Email",
			"InternalEwsUrl", "ExternalEwsUrl", "UserDisplayName", "ADServer",
			"Response_Time_ms", "Error",
		}); err != nil {
			logger.LogError(slogLogger, "Failed to write CSV header", "error", err)
		}
	}

	ewsClient, err := NewEWSClient(config)
	if err != nil {
		writeAutodiscoverCSV(csvLogger, slogLogger, config, nil, 0, err.Error())
		return err
	}

	fmt.Printf("  Endpoint: %s\n\n", ewsClient.AutodiscoverUrl())

	logger.LogDebug(slogLogger, "Sending Autodiscover GetUserSettings", "email", config.Username)

	start := time.Now()
	respBytes, err := ewsClient.SendAutodiscover(ctx, config.Username)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		fmt.Printf("✗ Autodiscover failed: %s\n", err)
		writeAutodiscoverCSV(csvLogger, slogLogger, config, nil, elapsed, err.Error())
		logger.LogError(slogLogger, "autodiscover failed", "error", err)
		return err
	}

	var envelope autodiscoverResponseEnvelope
	if xmlErr := xml.Unmarshal(respBytes, &envelope); xmlErr != nil {
		fmt.Printf("✗ Failed to parse Autodiscover response: %s\n", xmlErr)
		writeAutodiscoverCSV(csvLogger, slogLogger, config, nil, elapsed, xmlErr.Error())
		return xmlErr
	}

	inner := envelope.Body.GetUserSettingsResponse.Response
	if inner.ErrorCode != "" && inner.ErrorCode != "NoError" {
		errMsg := fmt.Sprintf("Autodiscover error: %s — %s", inner.ErrorCode, inner.ErrorMessage)
		fmt.Printf("✗ %s\n", errMsg)
		writeAutodiscoverCSV(csvLogger, slogLogger, config, nil, elapsed, errMsg)
		return errors.New(errMsg)
	}

	userResp := inner.UserSettings.UserResponse
	if userResp.ErrorCode != "" && userResp.ErrorCode != "NoError" {
		errMsg := fmt.Sprintf("User settings error: %s — %s", userResp.ErrorCode, userResp.ErrorMessage)
		fmt.Printf("✗ %s\n", errMsg)
		writeAutodiscoverCSV(csvLogger, slogLogger, config, nil, elapsed, errMsg)
		return errors.New(errMsg)
	}

	// Collect settings into a map for display
	settings := make(map[string]string)
	for _, s := range userResp.UserSettings {
		settings[s.Name] = s.Value
	}

	fmt.Println("✓ Autodiscover succeeded")
	fmt.Printf("\nUser Settings for %s:\n", userResp.Mailbox)
	if v, ok := settings["UserDisplayName"]; ok && v != "" {
		fmt.Printf("  Display Name:       %s\n", v)
	}
	if v, ok := settings["InternalEwsUrl"]; ok && v != "" {
		fmt.Printf("  Internal EWS URL:   %s\n", v)
	}
	if v, ok := settings["ExternalEwsUrl"]; ok && v != "" {
		fmt.Printf("  External EWS URL:   %s\n", v)
	}
	if v, ok := settings["ActiveDirectoryServer"]; ok && v != "" {
		fmt.Printf("  AD Server:          %s\n", v)
	}
	fmt.Printf("\n  Response time: %d ms\n\n", elapsed)
	fmt.Println("✓ Autodiscover test completed")

	logger.LogInfo(slogLogger, "autodiscover completed",
		"mailbox", userResp.Mailbox,
		"internal_ews", settings["InternalEwsUrl"],
		"elapsed_ms", elapsed)

	writeAutodiscoverCSV(csvLogger, slogLogger, config, settings, elapsed, "")
	return nil
}

func writeAutodiscoverCSV(
	csvLogger logger.Logger, slogLogger *slog.Logger,
	config *Config, settings map[string]string, elapsed int64, errStr string,
) {
	status := "SUCCESS"
	if errStr != "" {
		status = "FAILURE"
	}
	internalEWS, externalEWS, displayName, adServer := "", "", "", ""
	if settings != nil {
		internalEWS = settings["InternalEwsUrl"]
		externalEWS = settings["ExternalEwsUrl"]
		displayName = settings["UserDisplayName"]
		adServer = settings["ActiveDirectoryServer"]
	}
	if logErr := csvLogger.WriteRow([]string{
		config.Action, status, config.Host,
		fmt.Sprintf("%d", config.Port), config.Username,
		internalEWS, externalEWS, displayName, adServer,
		fmt.Sprintf("%d", elapsed), errStr,
	}); logErr != nil {
		logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
	}
}
