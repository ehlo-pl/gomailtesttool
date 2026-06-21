# Tools for testing mail services

## Basic functional testing

| Tool | Language | Supported Protocols | Focus / Best Use Case | Link |
| :--- | :--- | :--- | :--- | :--- |
| **SMTP-EDC** | Go | SMTP (Deep auth/TLS support) | Enhanced SMTP debugging and pipeline automation; includes an optional modern Desktop GUI. | [Website](https://smtp-edc.com/) / [GitHub](https://github.com/asachs01/smtp-edc) |
| **Himalaya** | Rust | SMTP, IMAP, JMAP, Maildir, Notmuch | Modern CLI email client engine; ideal for pipeline automation and state validation using its native JSON output. | [GitHub](https://github.com/pimalaya/himalaya) |
| **SWAKS** | Perl | SMTP, ESMTP, LMTP | The "Swiss Army Knife for SMTP"; the gold standard for step-by-step raw transaction and routing debugging. | [Website](https://jetmore.org/john/code/swaks/) / [GitHub](https://github.com/jetmore/swaks) |
| ***tluyben/go-smtp-cli*** | Go | SMTP | Go implementation of Perl smtp-cli | [github](https://github.com/tluyben/go-smtp-cli/) |

## Mail tools for relay mail traffic

| Tool | Language | Supported Protocols | Focus / Best Use Case | Link |
| :--- | :--- | :--- | :--- | :--- |

| ***grafana/smtprelay*** | Go | SMTP | simple SMTP relay by Grafana | [github](https://github.com/grafana/smtprelay) |
