package scanner

import (
	"testing"
	"time"
)

func TestNextDailyScanUsesShanghaiOneAM(t *testing.T) {
	location, err := time.LoadLocation(DailyScanTimezone)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		now  time.Time
		day  int
	}{
		{name: "before one", now: time.Date(2026, 10, 21, 0, 30, 0, 0, location), day: 21},
		{name: "after one", now: time.Date(2026, 10, 21, 1, 30, 0, 0, location), day: 22},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			next := NextDailyScan(test.now).In(location)
			if next.Day() != test.day || next.Hour() != 1 || next.Minute() != 0 || next.Second() != 0 {
				t.Fatalf("unexpected next daily scan: %v", next)
			}
		})
	}
}
