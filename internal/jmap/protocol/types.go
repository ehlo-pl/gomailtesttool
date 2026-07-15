// Package protocol provides JMAP protocol types and utilities.
package protocol

import (
	"encoding/json"
)

// Id represents a JMAP identifier string.
type Id string

// Session represents a JMAP session resource.
// See RFC 8620 Section 2.
type Session struct {
	// Capabilities contains the capabilities of the server.
	Capabilities map[string]json.RawMessage `json:"capabilities"`

	// Accounts contains information about the accounts available.
	Accounts map[Id]Account `json:"accounts"`

	// PrimaryAccounts maps data type URIs to the primary account ID.
	PrimaryAccounts map[string]Id `json:"primaryAccounts"`

	// Username is the username associated with the session.
	Username string `json:"username"`

	// APIURL is the URL for JMAP API requests.
	APIURL string `json:"apiUrl"`

	// DownloadURL is the URL template for downloading blobs.
	DownloadURL string `json:"downloadUrl"`

	// UploadURL is the URL for uploading blobs.
	UploadURL string `json:"uploadUrl"`

	// EventSourceURL is the URL for push notifications.
	EventSourceURL string `json:"eventSourceUrl"`

	// State is an opaque string representing the current state.
	State string `json:"state"`
}

// Account represents a JMAP account.
type Account struct {
	// Name is a human-readable name for the account.
	Name string `json:"name"`

	// IsPersonal indicates if this is the user's personal account.
	IsPersonal bool `json:"isPersonal"`

	// IsReadOnly indicates if the account is read-only.
	IsReadOnly bool `json:"isReadOnly"`

	// AccountCapabilities contains account-specific capability data.
	AccountCapabilities map[string]json.RawMessage `json:"accountCapabilities"`
}

// Mailbox represents a JMAP mailbox.
type Mailbox struct {
	// Id is the unique identifier for the mailbox.
	Id Id `json:"id"`

	// Name is the user-visible name of the mailbox.
	Name string `json:"name"`

	// ParentId is the ID of the parent mailbox, or null for top-level.
	ParentId *Id `json:"parentId"`

	// Role is the mailbox role (inbox, drafts, sent, trash, etc.).
	Role *string `json:"role"`

	// SortOrder is the sort order for display.
	SortOrder uint32 `json:"sortOrder"`

	// TotalEmails is the total number of emails in the mailbox.
	TotalEmails uint32 `json:"totalEmails"`

	// UnreadEmails is the number of unread emails.
	UnreadEmails uint32 `json:"unreadEmails"`

	// TotalThreads is the total number of threads.
	TotalThreads uint32 `json:"totalThreads"`

	// UnreadThreads is the number of unread threads.
	UnreadThreads uint32 `json:"unreadThreads"`

	// MyRights contains the user's permissions on this mailbox.
	MyRights *MailboxRights `json:"myRights"`

	// IsSubscribed indicates if the mailbox is subscribed.
	IsSubscribed bool `json:"isSubscribed"`
}

// MailboxRights represents the user's permissions on a mailbox.
type MailboxRights struct {
	MayReadItems   bool `json:"mayReadItems"`
	MayAddItems    bool `json:"mayAddItems"`
	MayRemoveItems bool `json:"mayRemoveItems"`
	MaySetSeen     bool `json:"maySetSeen"`
	MaySetKeywords bool `json:"maySetKeywords"`
	MayCreateChild bool `json:"mayCreateChild"`
	MayRename      bool `json:"mayRename"`
	MayDelete      bool `json:"mayDelete"`
	MaySubmit      bool `json:"maySubmit"`
}

// Email represents a JMAP email object.
type Email struct {
	// Id is the unique identifier for the email.
	Id Id `json:"id"`

	// BlobId is the identifier for the raw email blob.
	BlobId Id `json:"blobId"`

	// ThreadId is the identifier of the thread.
	ThreadId Id `json:"threadId"`

	// MailboxIds maps mailbox IDs to true for each mailbox containing this email.
	MailboxIds map[Id]bool `json:"mailboxIds"`

	// Keywords contains the email's keywords/flags.
	Keywords map[string]bool `json:"keywords"`

	// Size is the size of the raw email in bytes.
	Size uint32 `json:"size"`

	// ReceivedAt is when the email was received.
	ReceivedAt string `json:"receivedAt"`

	// MessageId contains the Message-ID header values.
	MessageId []string `json:"messageId"`

	// InReplyTo contains the In-Reply-To header values.
	InReplyTo []string `json:"inReplyTo"`

	// References contains the References header values.
	References []string `json:"references"`

	// Sender contains the Sender header addresses.
	Sender []EmailAddress `json:"sender"`

	// From contains the From header addresses.
	From []EmailAddress `json:"from"`

	// To contains the To header addresses.
	To []EmailAddress `json:"to"`

	// Cc contains the Cc header addresses.
	Cc []EmailAddress `json:"cc"`

	// Bcc contains the Bcc header addresses.
	Bcc []EmailAddress `json:"bcc"`

	// ReplyTo contains the Reply-To header addresses.
	ReplyTo []EmailAddress `json:"replyTo"`

	// Subject is the email subject.
	Subject string `json:"subject"`

	// SentAt is when the email was sent.
	SentAt string `json:"sentAt"`

	// Preview is a short plaintext preview of the email.
	Preview string `json:"preview"`

	// HasAttachment indicates if there are attachments.
	HasAttachment bool `json:"hasAttachment"`
}

// EmailAddress represents an email address with optional name.
type EmailAddress struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// EmailBodyPart references a body value by partId for textBody/htmlBody.
type EmailBodyPart struct {
	PartId string `json:"partId"`
	Type   string `json:"type"`
}

// EmailBodyValue holds the content for a body part.
type EmailBodyValue struct {
	Value   string `json:"value"`
	Charset string `json:"charset,omitempty"`
}

// EmailCreate is the argument map for Email/set create.
type EmailCreate struct {
	MailboxIds map[Id]bool              `json:"mailboxIds"`
	Keywords   map[string]bool          `json:"keywords,omitempty"`
	From       []EmailAddress           `json:"from,omitempty"`
	To         []EmailAddress           `json:"to,omitempty"`
	Cc         []EmailAddress           `json:"cc,omitempty"`
	Bcc        []EmailAddress           `json:"bcc,omitempty"`
	Subject    string                   `json:"subject,omitempty"`
	BodyValues map[string]EmailBodyValue `json:"bodyValues,omitempty"`
	TextBody   []EmailBodyPart          `json:"textBody,omitempty"`
	HTMLBody   []EmailBodyPart          `json:"htmlBody,omitempty"`
}

// GetMailboxesResponse represents the response from Mailbox/get.
type GetMailboxesResponse struct {
	AccountId Id        `json:"accountId"`
	State     string    `json:"state"`
	List      []Mailbox `json:"list"`
	NotFound  []Id      `json:"notFound"`
}

// QueryEmailsResponse represents the response from Email/query.
type QueryEmailsResponse struct {
	AccountId      Id     `json:"accountId"`
	QueryState     string `json:"queryState"`
	CanCalculateChanges bool `json:"canCalculateChanges"`
	Position       uint32 `json:"position"`
	Total          uint32 `json:"total"`
	Ids            []Id   `json:"ids"`
}

// GetEmailsResponse represents the response from Email/get.
type GetEmailsResponse struct {
	AccountId Id      `json:"accountId"`
	State     string  `json:"state"`
	List      []Email `json:"list"`
	NotFound  []Id    `json:"notFound"`
}
