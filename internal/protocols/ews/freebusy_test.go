package ews

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestMapFreeBusyStatus(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Free", FreeBusyStatusFree},
		{"free", FreeBusyStatusFree},
		{"FREE", FreeBusyStatusFree},
		{"Tentative", FreeBusyStatusTentative},
		{"tentative", FreeBusyStatusTentative},
		{"Busy", FreeBusyStatusBusy},
		{"busy", FreeBusyStatusBusy},
		{"OOF", FreeBusyStatusOOF},
		{"oof", FreeBusyStatusOOF},
		{"WorkingElsewhere", FreeBusyStatusWorkingElsewhere},
		{"workingelsewhere", FreeBusyStatusWorkingElsewhere},
		{"NoData", FreeBusyStatusNoData},
		{"nodata", FreeBusyStatusNoData},
		{"unknown", FreeBusyStatusNoData},
		{"", FreeBusyStatusNoData},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := mapFreeBusyStatus(tt.input)
			if got != tt.want {
				t.Errorf("mapFreeBusyStatus(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildMailboxDataXML(t *testing.T) {
	t.Run("empty list returns empty string", func(t *testing.T) {
		if got := buildMailboxDataXML(nil); got != "" {
			t.Errorf("buildMailboxDataXML(nil) = %q, want empty", got)
		}
	})

	t.Run("single mailbox", func(t *testing.T) {
		got := buildMailboxDataXML([]string{"user@example.com"})
		if !strings.Contains(got, "user@example.com") {
			t.Errorf("buildMailboxDataXML() = %q, missing mailbox address", got)
		}
		if !strings.Contains(got, "<t:MailboxData>") {
			t.Errorf("buildMailboxDataXML() = %q, missing MailboxData element", got)
		}
		if !strings.Contains(got, "<t:Address>") {
			t.Errorf("buildMailboxDataXML() = %q, missing Address element", got)
		}
	})

	t.Run("multiple mailboxes", func(t *testing.T) {
		got := buildMailboxDataXML([]string{"a@example.com", "b@example.com"})
		if strings.Count(got, "<t:MailboxData>") != 2 {
			t.Errorf("buildMailboxDataXML() = %q, want 2 MailboxData blocks, got %d",
				got, strings.Count(got, "<t:MailboxData>"))
		}
		if !strings.Contains(got, "a@example.com") || !strings.Contains(got, "b@example.com") {
			t.Errorf("buildMailboxDataXML() = %q, missing one or both addresses", got)
		}
	})

	t.Run("escapes XML special characters", func(t *testing.T) {
		got := buildMailboxDataXML([]string{"a&b@example.com"})
		if !strings.Contains(got, "a&amp;b@example.com") {
			t.Errorf("buildMailboxDataXML() = %q, expected XML-escaped ampersand", got)
		}
	})
}

func TestParseFreeBusyResponses_Success(t *testing.T) {
	// Minimal valid GetUserAvailability response with one mailbox.
	xmlInput := `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">
  <s:Body>
    <GetUserAvailabilityResponse xmlns="http://schemas.microsoft.com/exchange/services/2006/messages"
      xmlns:t="http://schemas.microsoft.com/exchange/services/2006/types">
      <FreeBusyResponseArray>
        <FreeBusyResponse>
          <ResponseMessage ResponseClass="Success">
            <ResponseCode>NoError</ResponseCode>
          </ResponseMessage>
          <FreeBusyView>
            <MergedFreeBusy>2200</MergedFreeBusy>
            <CalendarEventArray>
              <CalendarEvent>
                <StartTime>2026-08-01T09:00:00</StartTime>
                <EndTime>2026-08-01T10:00:00</EndTime>
                <BusyType>Busy</BusyType>
              </CalendarEvent>
              <CalendarEvent>
                <StartTime>2026-08-01T14:00:00</StartTime>
                <EndTime>2026-08-01T15:30:00</EndTime>
                <BusyType>Tentative</BusyType>
              </CalendarEvent>
            </CalendarEventArray>
          </FreeBusyView>
        </FreeBusyResponse>
      </FreeBusyResponseArray>
    </GetUserAvailabilityResponse>
  </s:Body>
</s:Envelope>`

	mailboxes := []string{"user@example.com"}
	slogger := slog.Default()
	loc := time.UTC

	results, err := parseFreeBusyResponses([]byte(xmlInput), mailboxes, loc, slogger)
	if err != nil {
		t.Fatalf("parseFreeBusyResponses() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}

	r := results[0]
	if !r.Success {
		t.Fatalf("result.Success = false, want true (ErrMsg: %s)", r.ErrMsg)
	}
	if r.Mailbox != "user@example.com" {
		t.Errorf("result.Mailbox = %q, want user@example.com", r.Mailbox)
	}
	if r.EventCount != 2 {
		t.Errorf("result.EventCount = %d, want 2", r.EventCount)
	}
	if len(r.Slots) != 2 {
		t.Fatalf("len(r.Slots) = %d, want 2", len(r.Slots))
	}
	if r.Slots[0].Status != FreeBusyStatusBusy {
		t.Errorf("Slots[0].Status = %q, want %q", r.Slots[0].Status, FreeBusyStatusBusy)
	}
	if r.Slots[1].Status != FreeBusyStatusTentative {
		t.Errorf("Slots[1].Status = %q, want %q", r.Slots[1].Status, FreeBusyStatusTentative)
	}
}

func TestParseFreeBusyResponses_EWSError(t *testing.T) {
	// Response with an EWS-level error for the mailbox.
	xmlInput := `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">
  <s:Body>
    <GetUserAvailabilityResponse xmlns="http://schemas.microsoft.com/exchange/services/2006/messages"
      xmlns:t="http://schemas.microsoft.com/exchange/services/2006/types">
      <FreeBusyResponseArray>
        <FreeBusyResponse>
          <ResponseMessage ResponseClass="Error">
            <ResponseCode>ErrorMailboxMoveInProgress</ResponseCode>
            <MessageText>The mailbox is being moved.</MessageText>
          </ResponseMessage>
          <FreeBusyView/>
        </FreeBusyResponse>
      </FreeBusyResponseArray>
    </GetUserAvailabilityResponse>
  </s:Body>
</s:Envelope>`

	mailboxes := []string{"mover@example.com"}
	slogger := slog.Default()
	loc := time.UTC

	results, err := parseFreeBusyResponses([]byte(xmlInput), mailboxes, loc, slogger)
	if err != nil {
		t.Fatalf("parseFreeBusyResponses() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}

	r := results[0]
	if r.Success {
		t.Errorf("result.Success = true, want false for EWS error response")
	}
	if !strings.Contains(r.ErrMsg, "ErrorMailboxMoveInProgress") {
		t.Errorf("result.ErrMsg = %q, want to contain ErrorMailboxMoveInProgress", r.ErrMsg)
	}
}

func TestParseFreeBusyResponses_MultiMailbox(t *testing.T) {
	// Response with two mailboxes: first succeeds, second fails.
	xmlInput := `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">
  <s:Body>
    <GetUserAvailabilityResponse xmlns="http://schemas.microsoft.com/exchange/services/2006/messages"
      xmlns:t="http://schemas.microsoft.com/exchange/services/2006/types">
      <FreeBusyResponseArray>
        <FreeBusyResponse>
          <ResponseMessage ResponseClass="Success">
            <ResponseCode>NoError</ResponseCode>
          </ResponseMessage>
          <FreeBusyView>
            <CalendarEventArray>
              <CalendarEvent>
                <StartTime>2026-08-01T10:00:00</StartTime>
                <EndTime>2026-08-01T11:00:00</EndTime>
                <BusyType>Busy</BusyType>
              </CalendarEvent>
            </CalendarEventArray>
          </FreeBusyView>
        </FreeBusyResponse>
        <FreeBusyResponse>
          <ResponseMessage ResponseClass="Error">
            <ResponseCode>ErrorCalendarNoFreeBusyAccess</ResponseCode>
            <MessageText>No access to free/busy.</MessageText>
          </ResponseMessage>
          <FreeBusyView/>
        </FreeBusyResponse>
      </FreeBusyResponseArray>
    </GetUserAvailabilityResponse>
  </s:Body>
</s:Envelope>`

	mailboxes := []string{"ok@example.com", "noaccess@example.com"}
	slogger := slog.Default()
	loc := time.UTC

	results, err := parseFreeBusyResponses([]byte(xmlInput), mailboxes, loc, slogger)
	if err != nil {
		t.Fatalf("parseFreeBusyResponses() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}

	if !results[0].Success || results[0].EventCount != 1 {
		t.Errorf("results[0]: Success=%v EventCount=%d, want Success=true EventCount=1",
			results[0].Success, results[0].EventCount)
	}
	if results[1].Success {
		t.Errorf("results[1].Success = true, want false")
	}
	if !strings.Contains(results[1].ErrMsg, "ErrorCalendarNoFreeBusyAccess") {
		t.Errorf("results[1].ErrMsg = %q, want ErrorCalendarNoFreeBusyAccess", results[1].ErrMsg)
	}
}

func TestParseFreeBusyResponses_EmptyResponse(t *testing.T) {
	// Response with no FreeBusyResponse entries.
	xmlInput := `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">
  <s:Body>
    <GetUserAvailabilityResponse xmlns="http://schemas.microsoft.com/exchange/services/2006/messages"/>
  </s:Body>
</s:Envelope>`

	mailboxes := []string{"user@example.com"}
	slogger := slog.Default()
	loc := time.UTC

	_, err := parseFreeBusyResponses([]byte(xmlInput), mailboxes, loc, slogger)
	if err == nil {
		t.Error("parseFreeBusyResponses() expected error for empty response, got nil")
	}
}

func TestParseFreeBusyResponses_InvalidXML(t *testing.T) {
	mailboxes := []string{"user@example.com"}
	slogger := slog.Default()
	loc := time.UTC

	_, err := parseFreeBusyResponses([]byte("not xml"), mailboxes, loc, slogger)
	if err == nil {
		t.Error("parseFreeBusyResponses() expected error for invalid XML, got nil")
	}
}

func TestFreeBusySlotStatusNormalization(t *testing.T) {
	// Verify that slots parsed from XML have their status normalized correctly.
	xmlInput := `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">
  <s:Body>
    <GetUserAvailabilityResponse xmlns="http://schemas.microsoft.com/exchange/services/2006/messages"
      xmlns:t="http://schemas.microsoft.com/exchange/services/2006/types">
      <FreeBusyResponseArray>
        <FreeBusyResponse>
          <ResponseMessage ResponseClass="Success">
            <ResponseCode>NoError</ResponseCode>
          </ResponseMessage>
          <FreeBusyView>
            <CalendarEventArray>
              <CalendarEvent>
                <StartTime>2026-08-01T08:00:00</StartTime>
                <EndTime>2026-08-01T09:00:00</EndTime>
                <BusyType>OOF</BusyType>
              </CalendarEvent>
              <CalendarEvent>
                <StartTime>2026-08-01T09:00:00</StartTime>
                <EndTime>2026-08-01T10:00:00</EndTime>
                <BusyType>WorkingElsewhere</BusyType>
              </CalendarEvent>
            </CalendarEventArray>
          </FreeBusyView>
        </FreeBusyResponse>
      </FreeBusyResponseArray>
    </GetUserAvailabilityResponse>
  </s:Body>
</s:Envelope>`

	results, err := parseFreeBusyResponses([]byte(xmlInput), []string{"user@example.com"}, time.UTC, slog.Default())
	if err != nil {
		t.Fatalf("parseFreeBusyResponses() error = %v", err)
	}
	if len(results) != 1 || !results[0].Success {
		t.Fatalf("unexpected result: %+v", results)
	}
	slots := results[0].Slots
	if len(slots) != 2 {
		t.Fatalf("len(slots) = %d, want 2", len(slots))
	}
	if slots[0].Status != FreeBusyStatusOOF {
		t.Errorf("slots[0].Status = %q, want %q", slots[0].Status, FreeBusyStatusOOF)
	}
	if slots[1].Status != FreeBusyStatusWorkingElsewhere {
		t.Errorf("slots[1].Status = %q, want %q", slots[1].Status, FreeBusyStatusWorkingElsewhere)
	}
}

func TestConfigFromViper_FreeBusyDefaults(t *testing.T) {
	// Verify new freebusy fields are included in ConfigFromViper defaults.
	cfg := NewConfig()
	if cfg.Timezone != "UTC" {
		t.Errorf("NewConfig().Timezone = %q, want UTC", cfg.Timezone)
	}
	if cfg.Interval != 30 {
		t.Errorf("NewConfig().Interval = %d, want 30", cfg.Interval)
	}
}
