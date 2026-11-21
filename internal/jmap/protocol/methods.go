// Package protocol provides JMAP method request/response handling.
package protocol

import (
	"encoding/json"
	"strings"
)

// Request represents a JMAP API request.
// See RFC 8620 Section 3.2.
type Request struct {
	// Using contains the capability URIs used in this request.
	Using []string `json:"using"`

	// MethodCalls contains the method invocations.
	MethodCalls []MethodCall `json:"methodCalls"`

	// CreatedIds maps creation IDs to server-assigned IDs.
	CreatedIds map[Id]Id `json:"createdIds,omitempty"`
}

// Response represents a JMAP API response.
type Response struct {
	// MethodResponses contains the method responses.
	MethodResponses []MethodResponse `json:"methodResponses"`

	// CreatedIds maps creation IDs to server-assigned IDs.
	CreatedIds map[Id]Id `json:"createdIds,omitempty"`

	// SessionState is the new session state if changed.
	SessionState string `json:"sessionState,omitempty"`
}

// MethodCall represents a single method invocation.
// Format: [name, arguments, methodCallId]
type MethodCall struct {
	Name      string
	Arguments interface{}
	CallId    string
}

// MarshalJSON implements custom JSON marshaling for MethodCall.
func (m MethodCall) MarshalJSON() ([]byte, error) {
	return json.Marshal([]interface{}{m.Name, m.Arguments, m.CallId})
}

// UnmarshalJSON implements custom JSON unmarshaling for MethodCall.
func (m *MethodCall) UnmarshalJSON(data []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if len(raw) != 3 {
		return nil
	}
	if err := json.Unmarshal(raw[0], &m.Name); err != nil {
		return err
	}
	m.Arguments = raw[1] // Keep as raw JSON
	if err := json.Unmarshal(raw[2], &m.CallId); err != nil {
		return err
	}
	return nil
}

// MethodResponse represents a single method response.
// Format: [name, arguments, methodCallId]
type MethodResponse struct {
	Name      string
	Arguments json.RawMessage
	CallId    string
}

// UnmarshalJSON implements custom JSON unmarshaling for MethodResponse.
func (m *MethodResponse) UnmarshalJSON(data []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if len(raw) != 3 {
		return nil
	}
	if err := json.Unmarshal(raw[0], &m.Name); err != nil {
		return err
	}
	m.Arguments = raw[1]
	if err := json.Unmarshal(raw[2], &m.CallId); err != nil {
		return err
	}
	return nil
}

// Error represents a JMAP method-level error.
type Error struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// Common capability URIs.
const (
	CoreCapability       = "urn:ietf:params:jmap:core"
	MailCapability       = "urn:ietf:params:jmap:mail"
	SubmissionCapability = "urn:ietf:params:jmap:submission"
)

// Common method names.
const (
	MethodMailboxGet         = "Mailbox/get"
	MethodMailboxQuery       = "Mailbox/query"
	MethodEmailGet           = "Email/get"
	MethodEmailQuery         = "Email/query"
	MethodEmailSet           = "Email/set"
	MethodEmailImport        = "Email/import"
	MethodEmailSubmissionSet = "EmailSubmission/set"
	MethodIdentityGet        = "Identity/get"
)

// GetRequest creates arguments for a /get method.
type GetRequest struct {
	AccountId  Id       `json:"accountId"`
	Ids        []Id     `json:"ids,omitempty"`
	Properties []string `json:"properties,omitempty"`
}

// QueryRequest creates arguments for a /query method.
type QueryRequest struct {
	AccountId      Id          `json:"accountId"`
	Filter         interface{} `json:"filter,omitempty"`
	Sort           []SortOrder `json:"sort,omitempty"`
	Position       uint32      `json:"position,omitempty"`
	Anchor         *Id         `json:"anchor,omitempty"`
	AnchorOffset   int32       `json:"anchorOffset,omitempty"`
	Limit          *uint32     `json:"limit,omitempty"`
	CalculateTotal bool        `json:"calculateTotal,omitempty"`
}

// SortOrder specifies how to sort results.
type SortOrder struct {
	Property    string `json:"property"`
	IsAscending bool   `json:"isAscending"`
}

// NewMailboxGetRequest creates a request to get all mailboxes.
func NewMailboxGetRequest(accountId Id) *Request {
	return &Request{
		Using: []string{CoreCapability, MailCapability},
		MethodCalls: []MethodCall{
			{
				Name: MethodMailboxGet,
				Arguments: GetRequest{
					AccountId: accountId,
				},
				CallId: "0",
			},
		},
	}
}

// NewMailboxGetWithPropertiesRequest creates a request to get mailboxes with specific properties.
func NewMailboxGetWithPropertiesRequest(accountId Id, properties []string) *Request {
	return &Request{
		Using: []string{CoreCapability, MailCapability},
		MethodCalls: []MethodCall{
			{
				Name: MethodMailboxGet,
				Arguments: GetRequest{
					AccountId:  accountId,
					Properties: properties,
				},
				CallId: "0",
			},
		},
	}
}

// NewEmailQueryRequest creates a request to query emails.
func NewEmailQueryRequest(accountId Id, filter interface{}, limit uint32) *Request {
	return &Request{
		Using: []string{CoreCapability, MailCapability},
		MethodCalls: []MethodCall{
			{
				Name: MethodEmailQuery,
				Arguments: QueryRequest{
					AccountId:      accountId,
					Filter:         filter,
					Limit:          &limit,
					CalculateTotal: true,
				},
				CallId: "0",
			},
		},
	}
}

// NewEmailGetRequest creates a request to get emails by ID with the given properties.
func NewEmailGetRequest(accountId Id, ids []Id, properties []string) *Request {
	return &Request{
		Using: []string{CoreCapability, MailCapability},
		MethodCalls: []MethodCall{
			{
				Name: MethodEmailGet,
				Arguments: GetRequest{
					AccountId:  accountId,
					Ids:        ids,
					Properties: properties,
				},
				CallId: "0",
			},
		},
	}
}

// ParseEmailGetResponse parses an Email/get response.
func ParseEmailGetResponse(resp *MethodResponse) (*GetEmailsResponse, error) {
	var result GetEmailsResponse
	if err := json.Unmarshal(resp.Arguments, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// BuildEmailSearchFilter builds an Email/query filter (RFC 8621 §4.4.1) that
// matches the given Message-ID header value and/or subject substring. At
// least one of messageID/subject must be non-empty. When both are given, the
// resulting filter requires both conditions (AND).
func BuildEmailSearchFilter(messageID, subject string) map[string]interface{} {
	var conditions []map[string]interface{}
	if messageID != "" {
		conditions = append(conditions, map[string]interface{}{
			"header": []string{"Message-ID", messageID},
		})
	}
	if subject != "" {
		conditions = append(conditions, map[string]interface{}{
			"subject": subject,
		})
	}

	if len(conditions) == 1 {
		return conditions[0]
	}
	return map[string]interface{}{
		"operator":   "AND",
		"conditions": conditions,
	}
}

// ParseMailboxGetResponse parses a Mailbox/get response.
func ParseMailboxGetResponse(resp *MethodResponse) (*GetMailboxesResponse, error) {
	var result GetMailboxesResponse
	if err := json.Unmarshal(resp.Arguments, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ParseEmailQueryResponse parses an Email/query response.
func ParseEmailQueryResponse(resp *MethodResponse) (*QueryEmailsResponse, error) {
	var result QueryEmailsResponse
	if err := json.Unmarshal(resp.Arguments, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// IsErrorResponse checks if a method response is an error.
func IsErrorResponse(name string) bool {
	return name == "error"
}

// Creation IDs used by the send requests. EmailSubmission/set references the
// created email by "#"+creation ID, so the IDs must match across method calls.
const (
	SendDraftCreationId      = "draft"
	SendImportCreationId     = "imported"
	SendSubmissionCreationId = "submission1"
)

// submissionArguments builds EmailSubmission/set arguments for one submission.
// emailId is either a "#creationId" string or a ResultReference object.
// identityId is required by RFC 8621 Section 7 (see Identity/get).
//
// When sentMailboxId is set, the submitted email is moved to that mailbox and
// its $draft keyword cleared (onSuccessUpdateEmail); otherwise the email is
// destroyed once submitted (onSuccessDestroyEmail), so no draft is left behind.
// See RFC 8621 Section 7.5.
func submissionArguments(accountId Id, emailId interface{}, identityId Id, mailFromEmail string, rcptTo []EmailAddress, sentMailboxId Id) map[string]interface{} {
	rcptList := make([]map[string]string, 0, len(rcptTo))
	for _, r := range rcptTo {
		rcptList = append(rcptList, map[string]string{"email": r.Email})
	}

	args := map[string]interface{}{
		"accountId": accountId,
		"create": map[string]interface{}{
			SendSubmissionCreationId: map[string]interface{}{
				"emailId":    emailId,
				"identityId": identityId,
				"envelope": map[string]interface{}{
					"mailFrom": map[string]string{"email": mailFromEmail},
					"rcptTo":   rcptList,
				},
			},
		},
	}

	ref := "#" + SendSubmissionCreationId
	if sentMailboxId != "" {
		args["onSuccessUpdateEmail"] = map[string]interface{}{
			ref: map[string]interface{}{
				"mailboxIds":      map[Id]bool{sentMailboxId: true},
				"keywords/$draft": nil,
			},
		}
	} else {
		args["onSuccessDestroyEmail"] = []string{ref}
	}
	return args
}

// NewEmailSetAndSubmitRequest builds a single-batch request that creates an
// email draft (Email/set) and immediately submits it (EmailSubmission/set).
// The submission references the draft by creation ID so the server links the
// two calls. draft.MailboxIds must be non-empty: RFC 8621 requires every Email
// to belong to at least one Mailbox. See submissionArguments for sentMailboxId.
func NewEmailSetAndSubmitRequest(accountId Id, draft EmailCreate, identityId Id, mailFromEmail string, rcptTo []EmailAddress, sentMailboxId Id) *Request {
	return &Request{
		Using: []string{CoreCapability, MailCapability, SubmissionCapability},
		MethodCalls: []MethodCall{
			{
				Name: MethodEmailSet,
				Arguments: map[string]interface{}{
					"accountId": accountId,
					"create": map[string]interface{}{
						SendDraftCreationId: draft,
					},
				},
				CallId: "c1",
			},
			{
				Name:      MethodEmailSubmissionSet,
				Arguments: submissionArguments(accountId, "#"+SendDraftCreationId, identityId, mailFromEmail, rcptTo, sentMailboxId),
				CallId:    "c2",
			},
		},
	}
}

// NewEmailImportAndSubmitRequest imports a complete RFC 5322 message and submits
// it in one JMAP request. This supports headers that Email/set cannot write,
// such as a caller-selected Message-ID. mailboxIds must be non-empty. See
// submissionArguments for sentMailboxId.
func NewEmailImportAndSubmitRequest(accountId Id, blobId Id, mailboxIds map[Id]bool, identityId Id, mailFromEmail string, rcptTo []EmailAddress, sentMailboxId Id) *Request {
	emailRef := map[string]string{
		"resultOf": "c1",
		"name":     MethodEmailImport,
		"path":     "/created/" + SendImportCreationId + "/id",
	}
	return &Request{
		Using: []string{CoreCapability, MailCapability, SubmissionCapability},
		MethodCalls: []MethodCall{
			{
				Name: MethodEmailImport,
				Arguments: map[string]interface{}{
					"accountId": accountId,
					"emails": map[string]interface{}{
						SendImportCreationId: map[string]interface{}{
							"blobId":     blobId,
							"mailboxIds": mailboxIds,
							"keywords":   map[string]bool{"$draft": true},
						},
					},
				},
				CallId: "c1",
			},
			{
				Name:      MethodEmailSubmissionSet,
				Arguments: submissionArguments(accountId, emailRef, identityId, mailFromEmail, rcptTo, sentMailboxId),
				CallId:    "c2",
			},
		},
	}
}

// Identity is a JMAP sending identity (RFC 8621 Section 6). Email may be a
// wildcard of the form "*@domain".
type Identity struct {
	Id    Id     `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// NewIdentityGetRequest creates a request for all identities of an account.
// Identity/get belongs to the submission capability.
func NewIdentityGetRequest(accountId Id) *Request {
	return &Request{
		Using: []string{CoreCapability, MailCapability, SubmissionCapability},
		MethodCalls: []MethodCall{
			{
				Name:      MethodIdentityGet,
				Arguments: GetRequest{AccountId: accountId},
				CallId:    "0",
			},
		},
	}
}

// ParseIdentityGetResponse extracts the identity list from an Identity/get response.
func ParseIdentityGetResponse(resp *MethodResponse) ([]Identity, error) {
	var result struct {
		List []Identity `json:"list"`
	}
	if err := json.Unmarshal(resp.Arguments, &result); err != nil {
		return nil, err
	}
	return result.List, nil
}

// SetError is a per-object error from a /set or /import method (RFC 8620
// Section 5.3), reported in notCreated / notUpdated / notDestroyed.
type SetError struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Properties  []string `json:"properties,omitempty"`
}

// String formats the error as "type (prop1, prop2): description".
func (e SetError) String() string {
	s := e.Type
	if len(e.Properties) > 0 {
		s += " (" + strings.Join(e.Properties, ", ") + ")"
	}
	if e.Description != "" {
		s += ": " + e.Description
	}
	return s
}

// ParseNotCreated extracts the notCreated map from a /set or /import method
// response. A method can succeed overall while rejecting individual objects,
// so callers must check this in addition to IsErrorResponse.
func ParseNotCreated(resp *MethodResponse) (map[string]SetError, error) {
	var result struct {
		NotCreated map[string]SetError `json:"notCreated"`
	}
	if err := json.Unmarshal(resp.Arguments, &result); err != nil {
		return nil, err
	}
	return result.NotCreated, nil
}

// ParseEmailSetResponse parses Email/set to extract the created email's server ID.
func ParseEmailSetResponse(resp *MethodResponse) (map[string]Id, error) {
	var result struct {
		Created map[string]struct {
			Id Id `json:"id"`
		} `json:"created"`
	}
	if err := json.Unmarshal(resp.Arguments, &result); err != nil {
		return nil, err
	}
	ids := make(map[string]Id, len(result.Created))
	for k, v := range result.Created {
		ids[k] = v.Id
	}
	return ids, nil
}
