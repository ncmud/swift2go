package swiftrt

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// DateStyle is Foundation `Date.FormatStyle.DateStyle`.
type DateStyle int

// Date.FormatStyle.DateStyle members.
const (
	DateStyleOmitted DateStyle = iota
	DateStyleNumeric
	DateStyleAbbreviated
	DateStyleLong
	DateStyleComplete
)

// TimeStyle is Foundation `Date.FormatStyle.TimeStyle`.
type TimeStyle int

// Date.FormatStyle.TimeStyle members.
const (
	TimeStyleOmitted TimeStyle = iota
	TimeStyleShortened
	TimeStyleStandard
	TimeStyleComplete
)

// DateFormattedDateTime is Foundation `Date.formatted(date:time:)` in the current time zone and the
// en_001 locale, which is Locale.current on Linux whatever LANG says: "22 Nov 2026" and "3:04 PM",
// joined by ", " after a numeric date and " at " after any other. The numeric date is "22/11/2026", a
// narrow no-break space precedes AM and PM, and the complete time names the zone by its GMT offset.
// With both styles omitted it is the numeric date and shortened time.
func DateFormattedDateTime(d Date, date DateStyle, clock TimeStyle) string {
	if date == DateStyleOmitted && clock == TimeStyleOmitted {
		date, clock = DateStyleNumeric, TimeStyleShortened
	}
	t := d.Time().In(time.Local)
	var day, at string
	switch date {
	case DateStyleNumeric:
		day, at = t.Format("02/01/2006"), ", "
	case DateStyleAbbreviated:
		day, at = t.Format("2 Jan 2006"), " at "
	case DateStyleLong:
		day, at = t.Format("2 January 2006"), " at "
	case DateStyleComplete:
		day, at = t.Format("Monday, 2 January 2006"), " at "
	}
	var hour string
	switch clock {
	case TimeStyleShortened:
		hour = t.Format("3:04\u202fPM")
	case TimeStyleStandard:
		hour = t.Format("3:04:05\u202fPM")
	case TimeStyleComplete:
		_, off := t.Zone()
		hour = t.Format("3:04:05\u202fPM") + " " + gmtOffsetName(off)
	}
	if day == "" || hour == "" {
		return day + hour
	}
	return day + at + hour
}

// gmtOffsetName is ICU's localized GMT format for a zone offset in seconds: "GMT", "GMT-5", "GMT+5:30".
func gmtOffsetName(off int) string {
	if off == 0 {
		return "GMT"
	}
	sign := "+"
	if off < 0 {
		sign, off = "-", -off
	}
	name := "GMT" + sign + strconv.Itoa(off/3600)
	if off%3600 != 0 {
		name += ":" + fmt.Sprintf("%02d", off%3600/60)
	}
	return name
}

// DurationUnit is Foundation `Duration.UnitsFormatStyle.Unit`, as the unit's length in nanoseconds.
type DurationUnit int64

// Duration.UnitsFormatStyle.Unit members.
const (
	UnitNanoseconds  DurationUnit = 1
	UnitMicroseconds DurationUnit = 1_000
	UnitMilliseconds DurationUnit = 1_000_000
	UnitSeconds      DurationUnit = 1_000_000_000
	UnitMinutes                   = 60 * UnitSeconds
	UnitHours                     = 60 * UnitMinutes
	UnitDays                      = 24 * UnitHours
	UnitWeeks                     = 7 * UnitDays
)

// UnitWidth is Foundation `Duration.UnitsFormatStyle.UnitWidth`.
type UnitWidth int

// Duration.UnitsFormatStyle.UnitWidth members.
const (
	UnitWidthWide UnitWidth = iota
	UnitWidthAbbreviated
	UnitWidthNarrow
)

// UnitsFormatStyle is Foundation `Duration.UnitsFormatStyle`.
type UnitsFormatStyle struct {
	units  []DurationUnit
	width  UnitWidth
	locale *Locale
}

// UnitsFormatStyleUnitsAllowedWidth is Foundation `Duration.UnitsFormatStyle.units(allowed:width:)`.
// allowed is a Set of DurationUnit.
func UnitsFormatStyleUnitsAllowedWidth(allowed any, width UnitWidth) UnitsFormatStyle {
	var units []DurationUnit
	switch set := allowed.(type) {
	case map[any]struct{}:
		for u := range set {
			units = append(units, u.(DurationUnit))
		}
	case map[DurationUnit]struct{}:
		for u := range set {
			units = append(units, u)
		}
	default:
		units = Iterate[DurationUnit](allowed)
	}
	slices.Sort(units)
	slices.Reverse(units)
	return UnitsFormatStyle{units: slices.Compact(units), width: width}
}

// UnitsFormatStyleLocale is `Duration.UnitsFormatStyle.locale(_:)`.
func UnitsFormatStyleLocale(s UnitsFormatStyle, l Locale) UnitsFormatStyle {
	s.locale = &l
	return s
}

// durationUnitNames is the en_US name of each unit, singular then plural, at each width.
var durationUnitNames = map[DurationUnit][3][2]string{
	UnitWeeks:        {{" week", " weeks"}, {" wk", " wks"}, {"w", "w"}},
	UnitDays:         {{" day", " days"}, {" day", " days"}, {"d", "d"}},
	UnitHours:        {{" hour", " hours"}, {" hr", " hr"}, {"h", "h"}},
	UnitMinutes:      {{" minute", " minutes"}, {" min", " min"}, {"m", "m"}},
	UnitSeconds:      {{" second", " seconds"}, {" sec", " sec"}, {"s", "s"}},
	UnitMilliseconds: {{" millisecond", " milliseconds"}, {" ms", " ms"}, {"ms", "ms"}},
	UnitMicroseconds: {{" microsecond", " microseconds"}, {" μs", " μs"}, {"μs", "μs"}},
	UnitNanoseconds:  {{" nanosecond", " nanoseconds"}, {" ns", " ns"}, {"ns", "ns"}},
}

// DurationFormatted is Foundation `Duration.formatted(_:)` with a UnitsFormatStyle in the en_US
// locale: the duration as counts of the allowed units, largest first, such as
// "3 hours, 12 minutes", "3 hr, 12 min" or "3h 12m". The duration is rounded to the smallest
// allowed unit, half to even, and units that count zero are left out, the smallest staying when
// all do. Another locale, a width outside wide, abbreviated and narrow, or a negative duration is
// not supported and panics.
func DurationFormatted(d Duration, s UnitsFormatStyle) string {
	if s.locale != nil && s.locale.Identifier != "en_US" {
		panic("swiftrt: Duration.UnitsFormatStyle is unverified for locale " + s.locale.Identifier)
	}
	if len(s.units) == 0 || s.width < UnitWidthWide || s.width > UnitWidthNarrow {
		panic("swiftrt: Duration.UnitsFormatStyle needs allowed units and a wide, abbreviated or narrow width")
	}
	if d < 0 {
		panic("swiftrt: Duration.UnitsFormatStyle is unverified for negative durations")
	}
	smallest := int64(s.units[len(s.units)-1])
	rest := int64(d)
	count, over := rest/smallest, rest%smallest
	if over*2 > smallest || over*2 == smallest && count%2 == 1 {
		count++
	}
	rest = count * smallest
	rule := groupingRules["en_US"]
	var parts []string
	for i, unit := range s.units {
		n := rest / int64(unit)
		rest -= n * int64(unit)
		if n == 0 && !(i == len(s.units)-1 && len(parts) == 0) {
			continue
		}
		names := durationUnitNames[unit][s.width]
		name := names[1]
		if n == 1 {
			name = names[0]
		}
		parts = append(parts, groupDigits(strconv.FormatInt(n, 10), rule)+name)
	}
	if s.width == UnitWidthNarrow {
		return strings.Join(parts, " ")
	}
	return strings.Join(parts, ", ")
}

// zoneAreas are the areas of the tz database whose zones are named Area/Location.
var zoneAreas = []string{"Africa", "America", "Antarctica", "Arctic", "Asia", "Atlantic", "Australia", "Europe", "Indian", "Pacific"}

// knownTimeZoneIdentifiers lists the Area/Location zones of the first time zone database found
// where Go's time package looks for one, then "GMT", sorted.
func knownTimeZoneIdentifiers() []string {
	dirs := []string{"/usr/share/zoneinfo", "/usr/share/lib/zoneinfo", "/usr/lib/locale/TZ", "/etc/zoneinfo"}
	if env := os.Getenv("ZONEINFO"); env != "" {
		dirs = append([]string{env}, dirs...)
	}
	names := []string{}
	for _, root := range dirs {
		for _, area := range zoneAreas {
			_ = filepath.WalkDir(filepath.Join(root, area), func(path string, entry fs.DirEntry, err error) error { //nolint:gosec // reads the names in the time zone database, wherever ZONEINFO puts it
				if err != nil {
					return fs.SkipDir
				}
				if !entry.IsDir() && !strings.Contains(entry.Name(), ".") {
					if rel, err := filepath.Rel(root, path); err == nil {
						names = append(names, filepath.ToSlash(rel))
					}
				}
				return nil
			})
		}
		if len(names) > 0 {
			break
		}
	}
	names = append(names, "GMT")
	slices.Sort(names)
	return names
}

// TimeZoneKnownTimeZoneIdentifiers is Foundation `TimeZone.knownTimeZoneIdentifiers`: the
// Area/Location identifiers of the host's time zone database, and "GMT", sorted. The list is the
// host's own, so it can differ from the one Foundation's bundled database gives.
var TimeZoneKnownTimeZoneIdentifiers = knownTimeZoneIdentifiers()
