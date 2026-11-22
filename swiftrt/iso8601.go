package swiftrt

import (
	"fmt"
	"strings"
	"time"
)

// DateISO8601Format is Foundation `Date.ISO8601Format()`: UTC, whole seconds.
func DateISO8601Format(d Date) string {
	return d.Time().UTC().Format("2006-01-02T15:04:05Z")
}

// ISO8601DateSeparator is Foundation `Date.ISO8601FormatStyle.DateSeparator`.
type ISO8601DateSeparator int

// Date.ISO8601FormatStyle.DateSeparator cases.
const (
	DateISO8601FormatStyleDateSeparatorDash ISO8601DateSeparator = iota
	DateISO8601FormatStyleDateSeparatorOmitted
)

// ISO8601DateTimeSeparator is Foundation `Date.ISO8601FormatStyle.DateTimeSeparator`.
type ISO8601DateTimeSeparator int

// Date.ISO8601FormatStyle.DateTimeSeparator cases.
const (
	DateISO8601FormatStyleDateTimeSeparatorStandard ISO8601DateTimeSeparator = iota
	DateISO8601FormatStyleDateTimeSeparatorSpace
)

// ISO8601TimeSeparator is Foundation `Date.ISO8601FormatStyle.TimeSeparator`.
type ISO8601TimeSeparator int

// Date.ISO8601FormatStyle.TimeSeparator cases.
const (
	DateISO8601FormatStyleTimeSeparatorColon ISO8601TimeSeparator = iota
	DateISO8601FormatStyleTimeSeparatorOmitted
)

// ISO8601TimeZoneSeparator is Foundation `Date.ISO8601FormatStyle.TimeZoneSeparator`.
type ISO8601TimeZoneSeparator int

// Date.ISO8601FormatStyle.TimeZoneSeparator cases.
const (
	DateISO8601FormatStyleTimeZoneSeparatorOmitted ISO8601TimeZoneSeparator = iota
	DateISO8601FormatStyleTimeZoneSeparatorColon
)

// ISO8601FormatStyle is Foundation `Date.ISO8601FormatStyle`: its separators, time zone, and the
// fields chosen by its builder methods. With no field chosen it writes the date, time and zone.
type ISO8601FormatStyle struct {
	year, month, day bool

	dateSeparator     ISO8601DateSeparator
	dateTimeSeparator ISO8601DateTimeSeparator
	timeSeparator     ISO8601TimeSeparator
	zoneSeparator     ISO8601TimeZoneSeparator
	fractional        bool
	zone              *time.Location
}

// DateISO8601FormatStyle is the name the emitter gives `Date.ISO8601FormatStyle` as a type.
type DateISO8601FormatStyle = ISO8601FormatStyle

// ISO8601FormatStyleIso8601 is `.iso8601`.
var ISO8601FormatStyleIso8601 = ISO8601FormatStyle{}

// ISO8601FormatStyleInitDateSeparatorDateTimeSeparatorTimeSeparatorTimeZoneSeparatorIncludingFractionalSecondsTimeZone
// is Foundation
// `Date.ISO8601FormatStyle(dateSeparator:dateTimeSeparator:timeSeparator:timeZoneSeparator:includingFractionalSeconds:timeZone:)`.
func ISO8601FormatStyleInitDateSeparatorDateTimeSeparatorTimeSeparatorTimeZoneSeparatorIncludingFractionalSecondsTimeZone(date ISO8601DateSeparator, dateTime ISO8601DateTimeSeparator, timeSep ISO8601TimeSeparator, zoneSep ISO8601TimeZoneSeparator, fractional bool, zone TimeZone) ISO8601FormatStyle {
	return ISO8601FormatStyle{dateSeparator: date, dateTimeSeparator: dateTime, timeSeparator: timeSep, zoneSeparator: zoneSep, fractional: fractional, zone: zone.Loc}
}

// ISO8601FormatStyleYear is `ISO8601FormatStyle.year()`.
func ISO8601FormatStyleYear(s ISO8601FormatStyle) ISO8601FormatStyle { s.year = true; return s }

// ISO8601FormatStyleMonth is `ISO8601FormatStyle.month()`.
func ISO8601FormatStyleMonth(s ISO8601FormatStyle) ISO8601FormatStyle { s.month = true; return s }

// ISO8601FormatStyleDay is `ISO8601FormatStyle.day()`.
func ISO8601FormatStyleDay(s ISO8601FormatStyle) ISO8601FormatStyle { s.day = true; return s }

// iso8601Zone is an ISO 8601 zone designator: `Z` for GMT, and otherwise the offset as hours and
// minutes, with seconds when it has them.
func iso8601Zone(t time.Time, colon bool) string {
	_, offset := t.Zone()
	if offset == 0 {
		return "Z"
	}
	sign := "+"
	if offset < 0 {
		sign, offset = "-", -offset
	}
	sep := ""
	if colon {
		sep = ":"
	}
	out := fmt.Sprintf("%s%02d%s%02d", sign, offset/3600, sep, offset%3600/60)
	if offset%60 != 0 {
		out += fmt.Sprintf("%s%02d", sep, offset%60)
	}
	return out
}

// ISO8601FormatStyleFormat is Foundation `Date.ISO8601FormatStyle.format(_:)`. Fractional seconds
// are three digits, truncated.
func ISO8601FormatStyleFormat(s ISO8601FormatStyle, d Date) string {
	loc := s.zone
	if loc == nil {
		loc = time.UTC
	}
	t := d.Time().In(loc)
	dateSep := "-"
	if s.dateSeparator == DateISO8601FormatStyleDateSeparatorOmitted {
		dateSep = ""
	}
	if s.year || s.month || s.day {
		var parts []string
		if s.year {
			parts = append(parts, t.Format("2006"))
		}
		if s.month {
			parts = append(parts, t.Format("01"))
		}
		if s.day {
			parts = append(parts, t.Format("02"))
		}
		return strings.Join(parts, dateSep)
	}
	layout := "2006" + dateSep + "01" + dateSep + "02"
	if s.dateTimeSeparator == DateISO8601FormatStyleDateTimeSeparatorSpace {
		layout += " "
	} else {
		layout += "T"
	}
	if s.timeSeparator == DateISO8601FormatStyleTimeSeparatorOmitted {
		layout += "150405"
	} else {
		layout += "15:04:05"
	}
	if s.fractional {
		layout += ".000"
	}
	return t.Format(layout) + iso8601Zone(t, s.zoneSeparator == DateISO8601FormatStyleTimeZoneSeparatorColon)
}

// DateFormatted is Foundation `Date.formatted(_:)` with an ISO8601FormatStyle.
func DateFormatted(d Date, s ISO8601FormatStyle) string { return ISO8601FormatStyleFormat(s, d) }

// ISO8601DateFormatOptions is Foundation `ISO8601DateFormatter.Options`: the members of the set,
// each one a constant's bits.
type ISO8601DateFormatOptions []uint

// ISO8601DateFormatter.Options members, under the names the emitter gives imported Objective-C constants.
const (
	NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithYear                     = 1
	NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithMonth                    = 2
	NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithWeekOfYear               = 4
	NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithDay                      = 16
	NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithTime                     = 32
	NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithTimeZone                 = 64
	NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithSpaceBetweenDateAndTime  = 128
	NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithDashSeparatorInDate      = 256
	NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithColonSeparatorInTime     = 512
	NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithColonSeparatorInTimeZone = 1024
	NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithFractionalSeconds        = 2048
	NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithFullDate                 = 275
	NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithFullTime                 = 1632
	NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithInternetDateTime         = 1907
)

func (o ISO8601DateFormatOptions) raw() uint {
	var bits uint
	for _, member := range o {
		bits |= member
	}
	return bits
}

// ISO8601DateFormatter is Foundation `ISO8601DateFormatter`, in GMT.
type ISO8601DateFormatter struct{ FormatOptions ISO8601DateFormatOptions }

// NSISO8601DateFormatterInit is Foundation `ISO8601DateFormatter()`, whose options default to `.withInternetDateTime`.
func NSISO8601DateFormatterInit() *ISO8601DateFormatter {
	return &ISO8601DateFormatter{ISO8601DateFormatOptions{NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithInternetDateTime}}
}

// layout is the Go time layout for the formatter's options: the calendar date, time and zone
// fields they select, with the separators they select. Week-of-year dates are not supported.
func (f *ISO8601DateFormatter) layout() string {
	o := f.FormatOptions.raw()
	has := func(bit uint) bool { return o&bit != 0 }
	if has(NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithWeekOfYear) {
		panic("swiftrt: ISO8601DateFormatter week-of-year dates are not supported")
	}
	dateSep, timeSep, zone := "", "", "Z0700"
	if has(NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithDashSeparatorInDate) {
		dateSep = "-"
	}
	if has(NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithColonSeparatorInTime) {
		timeSep = ":"
	}
	if has(NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithColonSeparatorInTimeZone) {
		zone = "Z07:00"
	}
	var date []string
	for _, field := range []struct {
		bit    uint
		layout string
	}{
		{NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithYear, "2006"},
		{NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithMonth, "01"},
		{NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithDay, "02"},
	} {
		if has(field.bit) {
			date = append(date, field.layout)
		}
	}
	layout := strings.Join(date, dateSep)
	if has(NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithTime) {
		if layout != "" {
			if has(NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithSpaceBetweenDateAndTime) {
				layout += " "
			} else {
				layout += "T"
			}
		}
		layout += "15" + timeSep + "04" + timeSep + "05"
		if has(NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithFractionalSeconds) {
			layout += ".000"
		}
	}
	if has(NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithTimeZone) {
		layout += zone
	}
	return layout
}

func (f *ISO8601DateFormatter) fractional() bool {
	return f.FormatOptions.raw()&NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithFractionalSeconds != 0
}

// NSISO8601DateFormatterStringFrom is `ISO8601DateFormatter.string(from:)`: the date in GMT with
// the fields and separators of the formatter's options, fractional seconds rounded to milliseconds.
func NSISO8601DateFormatterStringFrom(f *ISO8601DateFormatter, d Date) string {
	t := d.Time().UTC()
	if f.fractional() {
		t = t.Round(time.Millisecond)
	}
	return t.Format(f.layout())
}

// NSISO8601DateFormatterDateFrom is `ISO8601DateFormatter.date(from:)`: the date the string names
// in the form of the formatter's options, or nil when it has another form. Fractional seconds
// must be present exactly when the options include them.
func NSISO8601DateFormatterDateFrom(f *ISO8601DateFormatter, s string) *Date {
	layout := f.layout()
	if !f.fractional() && strings.Contains(layout, "05") && strings.ContainsAny(s, ".,") {
		return nil
	}
	if f.fractional() {
		// Any number of digits may follow the point.
		layout = strings.Replace(layout, ".000", ".999999999", 1)
		if !strings.Contains(s, ".") {
			return nil
		}
	}
	t, err := time.Parse(layout, s)
	if err != nil {
		return nil
	}
	d := dateOf(t)
	return &d
}
