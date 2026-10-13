package msgraph

import (
	"testing"
	"time"
)

func TestFreeRangesFromView(t *testing.T) {
	start := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
	got := freeRangesFromView("00201", start, time.Hour)
	if len(got) != 2 {
		t.Fatalf("expected 2 ranges, got %d", len(got))
	}
	if !got[0].End.Equal(start.Add(2*time.Hour)) || !got[1].Start.Equal(start.Add(3*time.Hour)) {
		t.Errorf("unexpected ranges: %+v", got)
	}
}

func TestAvailabilityList(t *testing.T) {
	start := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
	got := availabilityList("02", start, time.Hour)
	if len(got) != 2 || got[1].Status != "Busy" || !got[1].Start.Equal(start.Add(time.Hour)) {
		t.Errorf("unexpected list: %+v", got)
	}
}
