package ews

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ehlo-pl/gomailtesttool/internal/common/email"
	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	tmpl "github.com/ehlo-pl/gomailtesttool/internal/common/template"
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

// resolveTemplate renders --template (if set) and applies it to the config.
// HTML mode (non-.eml extension) fills BodyHTML with the rendered content;
// EML mode parses the rendered message and maps its recognised fields onto
// the config consumed by EWS CreateItem: recipients fall back to the EML
// headers where flags were not provided, subject and bodies come from the
// EML. The From header is ignored (EWS sends as the authenticated user) and
// Bcc recipients and unmapped headers are logged as skipped — CreateItem
// carries only To and Cc.
func resolveTemplate(config *Config, slogLogger *slog.Logger) error {
	if config.Template == "" {
		return nil
	}

	vars, err := tmpl.ParseVars(config.TemplateVars)
	if err != nil {
		return fmt.Errorf("invalid --template-vars: %w", err)
	}
	rendered, err := tmpl.Render(config.Template, vars)
	if err != nil {
		return err
	}

	if !tmpl.IsEML(config.Template) {
		config.BodyHTML = rendered
		return nil
	}

	parsed, err := tmpl.ParseEML(rendered)
	if err != nil {
		return fmt.Errorf("template %s: %w", config.Template, err)
	}
	if parsed.TextBody == "" && parsed.HTMLBody == "" {
		return fmt.Errorf("template %s has no text or HTML body", config.Template)
	}
	if len(config.To) == 0 {
		config.To = parsed.To
	}
	if len(config.Cc) == 0 {
		config.Cc = parsed.Cc
	}
	if len(config.To) == 0 {
		return fmt.Errorf("template %s has no To recipients; provide --to", config.Template)
	}
	if parsed.Subject != "" {
		config.Subject = parsed.Subject
	}
	config.Body = parsed.TextBody
	config.BodyHTML = parsed.HTMLBody

	if parsed.From != "" {
		logger.LogDebug(slogLogger, "Template From header is ignored; EWS sends as the authenticated user", "from", parsed.From)
	}
	if len(parsed.Bcc) > 0 {
		logger.LogWarn(slogLogger, "Template Bcc recipients are not supported by ews sendmail; skipped", "bcc", strings.Join(parsed.Bcc, ", "))
	}
	if len(parsed.UnmappedHeaders) > 0 {
		logger.LogWarn(slogLogger, "Template headers not mapped to EWS CreateItem", "headers", strings.Join(parsed.UnmappedHeaders, ", "))
	}
	return nil
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

	// Load attachments.
	onSkip := func(path string, err error) {
		logger.LogWarn(slogLogger, "Skipping attachment", "path", path, "error", err)
	}
	attachments, err := email.LoadAttachments(config.AttachmentFiles, onSkip)
	if err != nil {
		writeRow("FAILURE", 0, err.Error())
		return err
	}
	inlineAttachments, err := email.LoadInlineAttachments(config.InlineAttachmentFiles, onSkip)
	if err != nil {
		writeRow("FAILURE", 0, err.Error())
		return err
	}
	attachmentsXML := buildAttachmentsXML(append(attachments, inlineAttachments...))

	// Determine body type and content.
	bodyType := "Text"
	bodyContent := config.Body
	if config.BodyHTML != "" {
		bodyType = "HTML"
		bodyContent = config.BodyHTML
	}

	tpl := createItemSOAPBodySendOnlyFmt
	if config.SaveToSent {
		tpl = createItemSOAPBodySaveToSentFmt
	}
	soapBody := fmt.Sprintf(tpl,
		xmlEscape(config.Subject),
		bodyType,
		xmlEscape(bodyContent),
		attachmentsXML,
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

// buildAttachmentsXML builds the EWS <t:Attachments> XML block for a slice of
// attachments. Returns an empty string when there are no attachments.
func buildAttachmentsXML(attachments []email.Attachment) string {
	if len(attachments) == 0 {
		return ""
	}
	var b bytes.Buffer
	b.WriteString("          <t:Attachments>\n")
	for _, att := range attachments {
		b.WriteString("            <t:FileAttachment>\n")
		fmt.Fprintf(&b, "              <t:Name>%s</t:Name>\n", xmlEscape(att.Name))
		fmt.Fprintf(&b, "              <t:ContentType>%s</t:ContentType>\n", xmlEscape(att.ContentType))
		if att.Inline {
			b.WriteString("              <t:IsInline>true</t:IsInline>\n")
			fmt.Fprintf(&b, "              <t:ContentId>%s</t:ContentId>\n", xmlEscape(att.ContentID))
		}
		fmt.Fprintf(&b, "              <t:Content>%s</t:Content>\n", base64.StdEncoding.EncodeToString(att.Data))
		b.WriteString("            </t:FileAttachment>\n")
	}
	b.WriteString("          </t:Attachments>\n")
	return b.String()
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

// createItemSOAPBodySaveToSentFmt is the SOAP body for CreateItem with
// MessageDisposition="SendAndSaveCopy" — saves a copy in Sent Items.
// Args: XML-escaped subject, body type ("Text"/"HTML"), XML-escaped body,
// attachments XML block (empty string when none), recipient XML blocks for To
// and Cc. The Attachments block precedes the recipient elements because the EWS
// schema orders t:Item members (Attachments) before t:Message members (recipients).
const createItemSOAPBodySaveToSentFmt = `    <m:CreateItem MessageDisposition="SendAndSaveCopy">
      <m:SavedItemFolderId>
        <t:DistinguishedFolderId Id="sentitems"/>
      </m:SavedItemFolderId>
      <m:Items>
        <t:Message>
          <t:Subject>%s</t:Subject>
          <t:Body BodyType="%s">%s</t:Body>
%s          <t:ToRecipients>
%s      </t:ToRecipients>
          <t:CcRecipients>
%s      </t:CcRecipients>
        </t:Message>
      </m:Items>
    </m:CreateItem>`

// createItemSOAPBodySendOnlyFmt is the SOAP body for CreateItem with
// MessageDisposition="SendOnly" — sends without saving a copy.
// Args match createItemSOAPBodySaveToSentFmt.
const createItemSOAPBodySendOnlyFmt = `    <m:CreateItem MessageDisposition="SendOnly">
      <m:Items>
        <t:Message>
          <t:Subject>%s</t:Subject>
          <t:Body BodyType="%s">%s</t:Body>
%s          <t:ToRecipients>
%s      </t:ToRecipients>
          <t:CcRecipients>
%s      </t:CcRecipients>
        </t:Message>
      </m:Items>
    </m:CreateItem>`
