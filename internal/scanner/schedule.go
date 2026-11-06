package scanner

import (
	"time"
	_ "time/tzdata"
)

const (
	DailyScanHour     = 1
	DailyScanTime     = "01:00"
	DailyScanTimezone = "Asia/Shanghai"
)

var dailyScanLocation = loadDailyScanLocation()

func loadDailyScanLocation() *time.Location {
	location, err := time.LoadLocation(DailyScanTimezone)
	if err != nil {
		return time.FixedZone("CST", 8*60*60)
	}
	return location
}

// NextDailyScan returns the next 01:00 in the configured schedule timezone.
// The value is stored as UTC so SQLite and PostgreSQL behave consistently.
func NextDailyScan(now time.Time) time.Time {
	localNow := now.In(dailyScanLocation)
	next := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), DailyScanHour, 0, 0, 0, dailyScanLocation)
	if !next.After(localNow) {
		next = next.AddDate(0, 0, 1)
	}
	return next.UTC()
}

func isDailyScanTime(value time.Time) bool {
	local := value.In(dailyScanLocation)
	return local.Hour() == DailyScanHour && local.Minute() == 0 && local.Second() == 0
}
