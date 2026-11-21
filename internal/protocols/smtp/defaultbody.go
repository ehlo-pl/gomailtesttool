package smtp

import (
	"context"
	"log/slog"

	"github.com/ehlo-pl/gomailtesttool/internal/common/wikicontent"
)

// applyDefaultBody fills a sendmail message configured without any body (no
// --body, --bodyhtml or --template, by flag, env var or config file) with a
// random featured Wikipedia article when --body-source is "wikipedia". The
// default, "static", keeps the placeholder body without contacting Wikipedia.
// Articles already named in this action's default log files are skipped. A
// failed fetch is logged and the static body is kept.
func applyDefaultBody(ctx context.Context, config *Config, slogLogger *slog.Logger) {
	if config.BodySource == "" {
		config.BodySource = wikicontent.BodySourceStatic
	}
	if !wikicontent.NoBodyGiven(config.Body, NewConfig().Body, config.BodyHTML, config.Template) {
		return
	}
	history := wikicontent.History{Tool: "smtptool", Action: ActionSendMail}
	wikicontent.Fill(ctx, wikicontent.NewFetcher(config.BodyCategories), config.BodySource, wikicontent.Message{
		Subject:  &config.Subject,
		Body:     &config.Body,
		BodyHTML: &config.BodyHTML,
	}, NewConfig().Subject, history, slogLogger)
}
