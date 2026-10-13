package msgraph

import (
	"testing"
	"time"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
)

func TestBuildAvailabilityView(t *testing.T) {
	start := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	events := []eventSlot{
		{start: start.Add(30 * time.Minute), end: start.Add(90 * time.Minute), status: models.BUSY_FREEBUSYSTATUS},
		{start: start.Add(time.Hour), end: start.Add(2 * time.Hour), status: models.TENTATIVE_FREEBUSYSTATUS},
		{start: start.Add(3 * time.Hour), end: start.Add(4 * time.Hour), status: models.OOF_FREEBUSYSTATUS},
	}
	got := buildAvailabilityView(events, start, start.Add(5*time.Hour), time.Hour)
	if got != "22030" {
		t.Errorf("got %q, want 22030", got)
	}
}
