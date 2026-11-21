package swiftrt

import (
	"testing"
	"time"
)

func TestDateFormatterAMPMSymbols(t *testing.T) {
	f := NSDateFormatterInit()
	format, am, pm := "MMM d, h:mm a", "am", "pm"
	f.DateFormat, f.AmSymbol, f.PmSymbol = &format, &am, &pm
	utc := TimeZone{Loc: time.UTC}
	f.TimeZone = &utc
	d := Date(float64(time.Date(2026, 11, 21, 15, 4, 0, 0, time.UTC).Unix()))
	if got := NSDateFormatterStringFrom(f, d); got != "Nov 21, 3:04 pm" {
		t.Fatalf("got %q", got)
	}
}
