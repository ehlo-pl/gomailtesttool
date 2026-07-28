//go:build !integration
// +build !integration

package msgraph

import (
	"testing"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
)

func TestCreateAttendees(t *testing.T) {
	tests := []struct {
		name         string
		emails       []string
		attendeeType models.AttendeeType
	}{
		{"empty", nil, models.REQUIRED_ATTENDEETYPE},
		{"single required", []string{"a@example.com"}, models.REQUIRED_ATTENDEETYPE},
		{"multiple optional", []string{"a@example.com", "b@example.com"}, models.OPTIONAL_ATTENDEETYPE},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attendees := createAttendees(tt.emails, tt.attendeeType)
			if len(attendees) != len(tt.emails) {
				t.Fatalf("got %d attendees, want %d", len(attendees), len(tt.emails))
			}
			for i, a := range attendees {
				addr := a.GetEmailAddress()
				if addr == nil || addr.GetAddress() == nil || *addr.GetAddress() != tt.emails[i] {
					t.Errorf("attendee[%d] address = %v, want %s", i, addr, tt.emails[i])
				}
				gotType := a.GetTypeEscaped()
				if gotType == nil || *gotType != tt.attendeeType {
					t.Errorf("attendee[%d] type = %v, want %v", i, gotType, tt.attendeeType)
				}
			}
		})
	}
}
