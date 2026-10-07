package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMethodCall_MarshalJSON(t *testing.T) {
	mc := MethodCall{
		Name: "Mailbox/get",
		Arguments: map[string]interface{}{
			"accountId": "A123",
		},
		CallId: "c0",
	}

	data, err := json.Marshal(mc)
	if err != nil {
		t.Fatalf("MarshalJSON() error: %v", err)
	}

	// Should be an array: ["Mailbox/get", {...}, "c0"]
	var arr []json.RawMessage
	if err := json.Unmarshal(data, &arr); err != nil {
		t.Fatalf("Result should be an array: %v", err)
	}

	if len(arr) != 3 {
		t.Errorf("Array length = %d, want 3", len(arr))
	}

	// Check method name
	var name string
	if err := json.Unmarshal(arr[0], &name); err != nil {
		t.Fatalf("Failed to unmarshal method name: %v", err)
	}
	if name != "Mailbox/get" {
		t.Errorf("Method name = %q, want %q", name, "Mailbox/get")
	}

	// Check call ID
	var callId string
	if err := json.Unmarshal(arr[2], &callId); err != nil {
		t.Fatalf("Failed to unmarshal call ID: %v", err)
	}
	if callId != "c0" {
		t.Errorf("Call ID = %q, want %q", callId, "c0")
	}
}

func TestMethodCall_UnmarshalJSON(t *testing.T) {
	data := `["Mailbox/get", {"accountId": "A123"}, "c0"]`

	var mc MethodCall
	if err := json.Unmarshal([]byte(data), &mc); err != nil {
		t.Fatalf("UnmarshalJSON() error: %v", err)
	}

	if mc.Name != "Mailbox/get" {
		t.Errorf("Name = %q, want %q", mc.Name, "Mailbox/get")
	}
	if mc.CallId != "c0" {
		t.Errorf("CallId = %q, want %q", mc.CallId, "c0")
	}
}

func TestMethodResponse_UnmarshalJSON(t *testing.T) {
	data := `["Mailbox/get", {"accountId": "A123", "state": "abc", "list": []}, "c0"]`

	var mr MethodResponse
	if err := json.Unmarshal([]byte(data), &mr); err != nil {
		t.Fatalf("UnmarshalJSON() error: %v", err)
	}

	if mr.Name != "Mailbox/get" {
		t.Errorf("Name = %q, want %q", mr.Name, "Mailbox/get")
	}
	if mr.CallId != "c0" {
		t.Errorf("CallId = %q, want %q", mr.CallId, "c0")
	}
	if mr.Arguments == nil {
		t.Error("Arguments should not be nil")
	}
}

func TestRequest_MarshalJSON(t *testing.T) {
	req := &Request{
		Using: []string{CoreCapability, MailCapability},
		MethodCalls: []MethodCall{
			{
				Name: "Mailbox/get",
				Arguments: map[string]interface{}{
					"accountId": "A123",
				},
				CallId: "c0",
			},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("MarshalJSON() error: %v", err)
	}

	// Unmarshal to verify structure
	var result map[string]json.RawMessage
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	if _, ok := result["using"]; !ok {
		t.Error("Result should have 'using' field")
	}
	if _, ok := result["methodCalls"]; !ok {
		t.Error("Result should have 'methodCalls' field")
	}
}

func TestResponse_UnmarshalJSON(t *testing.T) {
	data := `{
		"methodResponses": [
			["Mailbox/get", {"accountId": "A123", "state": "abc", "list": []}, "c0"]
		],
		"sessionState": "xyz789"
	}`

	var resp Response
	if err := json.Unmarshal([]byte(data), &resp); err != nil {
		t.Fatalf("UnmarshalJSON() error: %v", err)
	}

	if len(resp.MethodResponses) != 1 {
		t.Errorf("MethodResponses length = %d, want 1", len(resp.MethodResponses))
	}
	if resp.SessionState != "xyz789" {
		t.Errorf("SessionState = %q, want %q", resp.SessionState, "xyz789")
	}
}

func TestNewMailboxGetRequest(t *testing.T) {
	req := NewMailboxGetRequest("A123")

	if len(req.Using) != 2 {
		t.Errorf("Using length = %d, want 2", len(req.Using))
	}
	if req.Using[0] != CoreCapability {
		t.Errorf("Using[0] = %q, want %q", req.Using[0], CoreCapability)
	}
	if req.Using[1] != MailCapability {
		t.Errorf("Using[1] = %q, want %q", req.Using[1], MailCapability)
	}

	if len(req.MethodCalls) != 1 {
		t.Errorf("MethodCalls length = %d, want 1", len(req.MethodCalls))
	}
	if req.MethodCalls[0].Name != MethodMailboxGet {
		t.Errorf("MethodCalls[0].Name = %q, want %q", req.MethodCalls[0].Name, MethodMailboxGet)
	}
}

func TestNewMailboxGetWithPropertiesRequest(t *testing.T) {
	properties := []string{"id", "name", "role"}
	req := NewMailboxGetWithPropertiesRequest("A123", properties)

	if len(req.MethodCalls) != 1 {
		t.Errorf("MethodCalls length = %d, want 1", len(req.MethodCalls))
	}

	args, ok := req.MethodCalls[0].Arguments.(GetRequest)
	if !ok {
		t.Fatal("Arguments should be GetRequest")
	}

	if len(args.Properties) != 3 {
		t.Errorf("Properties length = %d, want 3", len(args.Properties))
	}
}

func TestNewEmailQueryRequest(t *testing.T) {
	filter := map[string]interface{}{
		"inMailbox": "inbox",
	}
	req := NewEmailQueryRequest("A123", filter, 50)

	if len(req.MethodCalls) != 1 {
		t.Errorf("MethodCalls length = %d, want 1", len(req.MethodCalls))
	}
	if req.MethodCalls[0].Name != MethodEmailQuery {
		t.Errorf("MethodCalls[0].Name = %q, want %q", req.MethodCalls[0].Name, MethodEmailQuery)
	}
}

func TestNewEmailImportAndSubmitRequest(t *testing.T) {
	req := NewEmailImportAndSubmitRequest("account", "blob", map[Id]bool{"sent": true}, "from@example.com", []EmailAddress{{Email: "to@example.com"}})
	if len(req.MethodCalls) != 2 || req.MethodCalls[0].Name != MethodEmailImport || req.MethodCalls[1].Name != MethodEmailSubmissionSet {
		t.Fatalf("unexpected method calls: %+v", req.MethodCalls)
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	for _, want := range []string{`"blobId":"blob"`, `"path":"/created/imported/id"`, `"email":"to@example.com"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("request %s missing %q", data, want)
		}
	}
}

func TestParseMailboxGetResponse(t *testing.T) {
	respJSON := `{
		"accountId": "A123",
		"state": "abc123",
		"list": [
			{"id": "mb1", "name": "Inbox", "role": "inbox", "totalEmails": 100, "unreadEmails": 5}
		],
		"notFound": []
	}`

	mr := &MethodResponse{
		Name:      "Mailbox/get",
		Arguments: json.RawMessage(respJSON),
		CallId:    "c0",
	}

	result, err := ParseMailboxGetResponse(mr)
	if err != nil {
		t.Fatalf("ParseMailboxGetResponse() error: %v", err)
	}

	if result.AccountId != "A123" {
		t.Errorf("AccountId = %q, want %q", result.AccountId, "A123")
	}
	if result.State != "abc123" {
		t.Errorf("State = %q, want %q", result.State, "abc123")
	}
	if len(result.List) != 1 {
		t.Errorf("List length = %d, want 1", len(result.List))
	}
	if result.List[0].Name != "Inbox" {
		t.Errorf("List[0].Name = %q, want %q", result.List[0].Name, "Inbox")
	}
}

func TestParseEmailQueryResponse(t *testing.T) {
	respJSON := `{
		"accountId": "A123",
		"queryState": "state123",
		"canCalculateChanges": true,
		"position": 0,
		"total": 100,
		"ids": ["email1", "email2", "email3"]
	}`

	mr := &MethodResponse{
		Name:      "Email/query",
		Arguments: json.RawMessage(respJSON),
		CallId:    "c0",
	}

	result, err := ParseEmailQueryResponse(mr)
	if err != nil {
		t.Fatalf("ParseEmailQueryResponse() error: %v", err)
	}

	if result.AccountId != "A123" {
		t.Errorf("AccountId = %q, want %q", result.AccountId, "A123")
	}
	if result.Total != 100 {
		t.Errorf("Total = %d, want 100", result.Total)
	}
	if len(result.Ids) != 3 {
		t.Errorf("Ids length = %d, want 3", len(result.Ids))
	}
}

func TestIsErrorResponse(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"error", true},
		{"Mailbox/get", false},
		{"Email/query", false},
		{"", false},
	}

	for _, tt := range tests {
		result := IsErrorResponse(tt.name)
		if result != tt.expected {
			t.Errorf("IsErrorResponse(%q) = %v, want %v", tt.name, result, tt.expected)
		}
	}
}

func TestConstants(t *testing.T) {
	// Verify capability constants
	if CoreCapability != "urn:ietf:params:jmap:core" {
		t.Errorf("CoreCapability = %q, want %q", CoreCapability, "urn:ietf:params:jmap:core")
	}
	if MailCapability != "urn:ietf:params:jmap:mail" {
		t.Errorf("MailCapability = %q, want %q", MailCapability, "urn:ietf:params:jmap:mail")
	}
	if SubmissionCapability != "urn:ietf:params:jmap:submission" {
		t.Errorf("SubmissionCapability = %q, want %q", SubmissionCapability, "urn:ietf:params:jmap:submission")
	}

	// Verify method constants
	if MethodMailboxGet != "Mailbox/get" {
		t.Errorf("MethodMailboxGet = %q, want %q", MethodMailboxGet, "Mailbox/get")
	}
	if MethodEmailQuery != "Email/query" {
		t.Errorf("MethodEmailQuery = %q, want %q", MethodEmailQuery, "Email/query")
	}
}
