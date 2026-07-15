package ews

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
)

// EWS response type for CreateItem.

type createItemResponseEnvelope struct {
	XMLName struct{}              `xml:"Envelope"`
	Body    createItemRespBody    `xml:"Body"`
}

type createItemRespBody struct {
	Response createItemResponse `xml:"CreateItemResponse"`
}

type createItemResponse struct {
	ResponseMessages createItemRespMessages `xml:"ResponseMessages"`
}

type createItemRespMessages struct {
	Message createItemRespMessage `xml:"CreateItemResponseMessage"`
}

type createItemRespMessage struct {
	ResponseClass string `xml:"ResponseClass,attr"`
	ResponseCode  string `xml:"ResponseCode"`
	MessageText   string `xml:"MessageText"`
}

// sendMail sends an email via EWS CreateItem with MessageDisposition="SendAndSaveCopy".
func sendMail(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	fmt.Printf("Sending mail via EWS at https://%s:%d%s...\n\n", config.Host, config.Port, config.EWSPath)

	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		_ = csvLogger.WriteHeader([]string{
			"Action", "Status", "Server", "Port", "Username",
			"To", "Subject", "Response_Time_ms", "Error",
		})
	}

	toStr := strings.Join(config.To, "; ")

	writeRow := func(status string, elapsed int64, errStr string) {
		if logErr := csvLogger.WriteRow([]string{
			config.Action, status, config.Host,
			fmt.Sprintf("%d", config.Port), config.Username,
			toStr, config.Subject, fmt.Sprintf("%d", elapsed), errStr,
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
	}

	ewsClient, err := NewEWSClient(config)
	if err != nil {
		writeRow("FAILURE", 0, err.Error())
		return err
	}

	// Build XML for recipients.
	toXML := buildRecipientsXML(config.To)
	ccXML := buildRecipientsXML(config.Cc)

	// Determine body type and content.
	bodyType := "Text"
	bodyContent := config.Body
	if config.BodyHTML != "" {
		bodyType = "HTML"
		bodyContent = config.BodyHTML
	}

	soapBody := fmt.Sprintf(createItemSOAPBodyFmt,
		xmlEscape(config.Subject),
		bodyType,
		xmlEscape(bodyContent),
		toXML,
		ccXML,
	)

	start := time.Now()
	respBytes, err := ewsClient.SendSOAP(ctx, soapBody)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		fmt.Printf("✗ CreateItem failed: %s\n", err)
		writeRow("FAILURE", elapsed, err.Error())
		logger.LogError(slogLogger, "sendmail failed", "error", err)
		return err
	}

	var envelope createItemResponseEnvelope
	if xmlErr := xml.Unmarshal(respBytes, &envelope); xmlErr != nil {
		fmt.Printf("✗ Failed to parse EWS response: %s\n", xmlErr)
		writeRow("FAILURE", elapsed, xmlErr.Error())
		return xmlErr
	}

	msg := envelope.Body.Response.ResponseMessages.Message
	if msg.ResponseClass != "Success" {
		errMsg := fmt.Sprintf("EWS error: %s — %s", msg.ResponseCode, msg.MessageText)
		fmt.Printf("✗ %s\n", errMsg)
		writeRow("FAILURE", elapsed, errMsg)
		return errors.New(errMsg)
	}

	fmt.Printf("✓ Email sent successfully (response time: %d ms)\n", elapsed)
	fmt.Printf("  To:      %s\n", toStr)
	fmt.Printf("  Subject: %s\n", config.Subject)

	writeRow("SUCCESS", elapsed, "")
	logger.LogInfo(slogLogger, "sendmail completed", "to", toStr, "subject", config.Subject, "elapsed_ms", elapsed)
	return nil
}

// buildRecipientsXML builds the EWS Mailbox XML for a list of email addresses.
func buildRecipientsXML(addrs []string) string {
	if len(addrs) == 0 {
		return ""
	}
	var b bytes.Buffer
	for _, addr := range addrs {
		fmt.Fprintf(&b, `        <t:Mailbox><t:EmailAddress>%s</t:EmailAddress></t:Mailbox>`, xmlEscape(addr))
		b.WriteByte('\n')
	}
	return b.String()
}

// xmlEscape escapes a string for safe inclusion in an XML text node.
func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// createItemSOAPBodyFmt is the SOAP body for CreateItem (sendmail).
// Args: XML-escaped subject, body type ("Text"/"HTML"), XML-escaped body,
// recipient XML blocks for To and Cc.
const createItemSOAPBodyFmt = `    <m:CreateItem MessageDisposition="SendAndSaveCopy">
      <m:SavedItemFolderId>
        <t:DistinguishedFolderId Id="sentitems"/>
      </m:SavedItemFolderId>
      <m:Items>
        <t:Message>
          <t:Subject>%s</t:Subject>
          <t:Body BodyType="%s">%s</t:Body>
          <t:ToRecipients>
%s      </t:ToRecipients>
          <t:CcRecipients>
%s      </t:CcRecipients>
        </t:Message>
      </m:Items>
    </m:CreateItem>`
