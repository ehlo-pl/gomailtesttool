package ews

import (
	"context"
	"log/slog"

	"github.com/ehlo-pl/gomailtesttool/internal/common/wikicontent"
)

// applyDefaultBody fills a sendmail message configured without any body (no
// --body, --bodyhtml or --template, by flag, env var or config file) with a
// random featured Wikipedia article when --body-source is "wikipedia", a random
// RFC when it is "rfc" or a random poem when it is "poetry". The default,
// "static", keeps the placeholder body without contacting any of these sites.
// Content already named in this action's default log files is skipped. A
// failed fetch is logged and the static body is kept.
func applyDefaultBody(ctx context.Context, config *Config, slogLogger *slog.Logger) {
	if config.BodySource == "" {
		config.BodySource = wikicontent.BodySourceStatic
	}
	if !wikicontent.NoBodyGiven(config.Body, defaultSendMailBody, config.BodyHTML, config.Template) {
		return
	}
	history := wikicontent.History{Tool: "ewstool", Action: ActionSendMail}
	wikicontent.Fill(ctx, wikicontent.NewFetcher(config.BodyCategories), config.BodySource, wikicontent.Message{
		Subject:  &config.Subject,
		Body:     &config.Body,
		BodyHTML: &config.BodyHTML,
	}, "Automated Tool Notification", history, slogLogger)
}
