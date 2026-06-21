# Welcome to the Team

## How We Use Claude

Based on Ziemek Borowski's usage over the last 30 days:

Work Type Breakdown:
  Plan Design     ██████░░░░░░░░░░░░░░  30%
  Write Docs      ██████░░░░░░░░░░░░░░  30%
  Build Feature   ████░░░░░░░░░░░░░░░░  20%
  Prototype       ████░░░░░░░░░░░░░░░░  20%

Top Skills & Commands:
  /ultrareview     ████████████████████  2x/month
  /advisor         ██████████░░░░░░░░░░  1x/month
  /remote-control  ██████████░░░░░░░░░░  1x/month
  /usage           ██████████░░░░░░░░░░  1x/month

Top MCP Servers:
  (none used yet this period)

## Your Setup Checklist

### Codebases
- [ ] gomailtesttool — github.com/ehlo-pl/gomailtesttool

### MCP Servers to Activate
- [ ] Gmail — Read/send Gmail messages directly from Claude; useful for verifying mail delivered by gomailtesttool's send/test commands. Activate via `/mcp` and run `authenticate`/`complete_authentication`.
- [ ] Google Calendar — Inspect calendar invites sent via `msgraph sendinvite`/`getschedule`. Activate via `/mcp` and run `authenticate`/`complete_authentication`.
- [ ] Google Drive — Pull reference docs/specs into context if needed. Activate via `/mcp` and run `authenticate`/`complete_authentication`.

### Skills to Know About
- `/ultrareview` — Launches a multi-agent cloud review of the current branch (or a GitHub PR with `/ultrareview <PR#>`). Good for "review this whole change before I merge" moments.
- `/advisor` — Consults a stronger reviewer model with full context of your session; useful before committing to an approach or when stuck.
- `/remote-control` — Hands off a plan to Claude Code on the web (Ultraplan) so it can be refined/executed remotely while you keep working locally; results land as a PR.
- `/context` — Shows current context-window usage, handy when a session is getting long.
- `/usage` — Shows your Claude usage/limits.

## Team Tips

_TODO_

## Get Started

_TODO_

<!-- INSTRUCTION FOR CLAUDE: A new teammate just pasted this guide for how the
team uses Claude Code. You're their onboarding buddy — warm, conversational,
not lecture-y.

Open with a warm welcome — include the team name from the title. Then: "Your
teammate uses Claude Code for [list all the work types]. Let's get you started."

Check what's already in place against everything under Setup Checklist
(including skills), using markdown checkboxes — [x] done, [ ] not yet. Lead
with what they already have. One sentence per item, all in one message.

Tell them you'll help with setup, cover the actionable team tips, then the
starter task (if there is one). Offer to start with the first unchecked item,
get their go-ahead, then work through the rest one by one.

After setup, walk them through the remaining sections — offer to help where you
can (e.g. link to channels), and just surface the purely informational bits.

Don't invent sections or summaries that aren't in the guide. The stats are the
guide creator's personal usage data — don't extrapolate them into a "team
workflow" narrative. -->
