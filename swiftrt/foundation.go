package swiftrt

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
)

// UUID is Foundation `UUID`.
type UUID [16]byte

// UUIDInit is Foundation `UUID()`: a random version-4 UUID.
func UUIDInit() UUID {
	var u UUID
	if _, err := rand.Read(u[:]); err != nil {
		panic(err)
	}
	u[6] = u[6]&0x0f | 0x40
	u[8] = u[8]&0x3f | 0x80
	return u
}

// UUIDUuidString is Foundation `UUID.uuidString` (uppercase, hyphenated).
func UUIDUuidString(u UUID) string {
	h := strings.ToUpper(hex.EncodeToString(u[:]))
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// UUIDOptInitUuidString is Foundation `UUID(uuidString:)`.
func UUIDOptInitUuidString(s string) *UUID {
	h := strings.ReplaceAll(s, "-", "")
	if len(s) != 36 || len(h) != 32 {
		return nil
	}
	b, err := hex.DecodeString(h)
	if err != nil {
		return nil
	}
	var u UUID
	copy(u[:], b)
	return &u
}

// Description is the uuidString.
func (u UUID) Description() string { return UUIDUuidString(u) }

// Date is Foundation `Date`, stored as seconds since 1970 so it orders and compares like Swift's.
type Date float64

// DateInit is Foundation `Date()`.
func DateInit() Date { return Date(float64(time.Now().UnixNano()) / 1e9) }

// DateInitTimeIntervalSince1970 is Foundation `Date(timeIntervalSince1970:)`.
func DateInitTimeIntervalSince1970(s float64) Date { return Date(s) }

// DateInitTimeIntervalSinceNow is Foundation `Date(timeIntervalSinceNow:)`.
func DateInitTimeIntervalSinceNow(s float64) Date { return DateInit() + Date(s) }

// DateInitTimeIntervalSinceReferenceDate is Foundation `Date(timeIntervalSinceReferenceDate:)`.
func DateInitTimeIntervalSinceReferenceDate(s float64) Date { return Date(s + 978307200) }

// DateTimeIntervalSince1970 is Foundation `Date.timeIntervalSince1970`.
func DateTimeIntervalSince1970(d Date) float64 { return float64(d) }

// DateTimeIntervalSinceReferenceDate is Foundation `Date.timeIntervalSinceReferenceDate`.
func DateTimeIntervalSinceReferenceDate(d Date) float64 { return float64(d) - 978307200 }

// DateTimeIntervalSince is Foundation `Date.timeIntervalSince(_:)`.
func DateTimeIntervalSince(d, o Date) float64 { return float64(d - o) }

// DateTimeIntervalSinceNow is Foundation `Date.timeIntervalSinceNow`.
func DateTimeIntervalSinceNow(d Date) float64 { return float64(d - DateInit()) }

// DateAddingTimeIntervalFloat is Foundation `Date.addingTimeInterval(_:)`.
func DateAddingTimeIntervalFloat(d Date, s float64) Date { return d + Date(s) }

// DateAddingTimeInterval is Foundation `Date.addingTimeInterval(_:)`.
func DateAddingTimeInterval(d Date, s float64) Date { return d + Date(s) }

// DateDistantPast is Foundation `Date.distantPast`.
const DateDistantPast Date = -62135769600

// DateDistantFuture is Foundation `Date.distantFuture`.
const DateDistantFuture Date = 64092211200

// Time converts to a Go time.
func (d Date) Time() time.Time {
	sec, frac := math.Modf(float64(d))
	return time.Unix(int64(sec), int64(frac*1e9))
}

// Description matches Foundation's `Date.description`.
func (d Date) Description() string { return d.Time().UTC().Format("2006-01-02 15:04:05 +0000") }

// TimeZone is Foundation `TimeZone`.
type TimeZone struct{ Loc *time.Location }

// TimeZoneOptInitIdentifier is Foundation `TimeZone(identifier:)`.
func TimeZoneOptInitIdentifier(id string) *TimeZone {
	loc, err := time.LoadLocation(id)
	if err != nil {
		return nil
	}
	return &TimeZone{loc}
}

// TimeZoneCurrent is Foundation `TimeZone.current`.
func TimeZoneCurrent() TimeZone { return TimeZone{time.Local} }

// TimeZoneInitSecondsFromGMT is Foundation `TimeZone(secondsFromGMT:)`.
func TimeZoneInitSecondsFromGMT(s int) *TimeZone { return &TimeZone{time.FixedZone("", s)} }

// TimeZoneIdentifier is Foundation `TimeZone.identifier`.
func TimeZoneIdentifier(z TimeZone) string { return z.Loc.String() }

// TimeZoneSecondsFromGMTFor is Foundation `TimeZone.secondsFromGMT(for:)`.
func TimeZoneSecondsFromGMTFor(z TimeZone, d Date) int {
	_, off := d.Time().In(z.Loc).Zone()
	return off
}

// TimeZoneGmt is Foundation `TimeZone.gmt`.
func TimeZoneGmt() TimeZone { return TimeZone{time.UTC} }

// Locale is Foundation `Locale`.
type Locale struct{ Identifier string }

// LocaleInitIdentifier is Foundation `Locale(identifier:)`.
func LocaleInitIdentifier(id string) Locale { return Locale{id} }

// CalendarComponent is Foundation `Calendar.Component`.
type CalendarComponent int

// Calendar components.
const (
	CalendarComponentYear CalendarComponent = iota
	CalendarComponentMonth
	CalendarComponentDay
	CalendarComponentHour
	CalendarComponentMinute
	CalendarComponentSecond
	CalendarComponentWeekday
	CalendarComponentNanosecond
)

// CalendarIdentifier is Foundation `Calendar.Identifier`.
type CalendarIdentifier int

// CalendarIdentifierGregorian is `.gregorian`.
const CalendarIdentifierGregorian CalendarIdentifier = 0

// Calendar is Foundation `Calendar` (Gregorian only).
type Calendar struct{ TimeZone TimeZone }

// CalendarInitIdentifier is Foundation `Calendar(identifier:)`.
func CalendarInitIdentifier(CalendarIdentifier) Calendar { return Calendar{TimeZone{time.Local}} }

// CalendarCurrent is Foundation `Calendar.current`.
func CalendarCurrent() Calendar { return Calendar{TimeZone{time.Local}} }

// DateComponents is Foundation `DateComponents`.
type DateComponents struct {
	Year, Month, Day, Hour, Minute, Second, Weekday, Nanosecond *int
}

// CalendarDateComponentsFrom is Foundation `Calendar.dateComponents(_:from:)`.
func CalendarDateComponentsFrom(c Calendar, comps map[CalendarComponent]struct{}, d Date) DateComponents {
	t := d.Time().In(c.TimeZone.Loc)
	var out DateComponents
	set := func(k CalendarComponent, v int, dst **int) {
		if _, ok := comps[k]; ok {
			*dst = &v
		}
	}
	set(CalendarComponentYear, t.Year(), &out.Year)
	set(CalendarComponentMonth, int(t.Month()), &out.Month)
	set(CalendarComponentDay, t.Day(), &out.Day)
	set(CalendarComponentHour, t.Hour(), &out.Hour)
	set(CalendarComponentMinute, t.Minute(), &out.Minute)
	set(CalendarComponentSecond, t.Second(), &out.Second)
	set(CalendarComponentWeekday, int(t.Weekday())+1, &out.Weekday)
	set(CalendarComponentNanosecond, t.Nanosecond(), &out.Nanosecond)
	return out
}

// CalendarComponentFrom is Foundation `Calendar.component(_:from:)`.
func CalendarComponentFrom(c Calendar, k CalendarComponent, d Date) int {
	dc := CalendarDateComponentsFrom(c, map[CalendarComponent]struct{}{k: {}}, d)
	for _, p := range []*int{dc.Year, dc.Month, dc.Day, dc.Hour, dc.Minute, dc.Second, dc.Weekday, dc.Nanosecond} {
		if p != nil {
			return *p
		}
	}
	return 0
}

// DateComponentsYear is `DateComponents.year`.
func DateComponentsYear(d DateComponents) *int { return d.Year }

// DateComponentsMonth is `DateComponents.month`.
func DateComponentsMonth(d DateComponents) *int { return d.Month }

// DateComponentsDay is `DateComponents.day`.
func DateComponentsDay(d DateComponents) *int { return d.Day }

// DateComponentsHour is `DateComponents.hour`.
func DateComponentsHour(d DateComponents) *int { return d.Hour }

// DateComponentsMinute is `DateComponents.minute`.
func DateComponentsMinute(d DateComponents) *int { return d.Minute }

// DateFormatter is Foundation `DateFormatter` with ICU patterns translated to Go layouts.
type DateFormatter struct {
	DateFormat *string
	Locale     *Locale
	TimeZone   *TimeZone
	DateStyle  int
	TimeStyle  int
}

// NSDateFormatterInit is Foundation `DateFormatter()`.
func NSDateFormatterInit() *DateFormatter { return &DateFormatter{} }

func (f *DateFormatter) layout() string {
	if f.DateFormat == nil {
		return "1/2/06, 3:04 PM"
	}
	r := strings.NewReplacer("yyyy", "2006", "yy", "06", "MMMM", "January", "MMM", "Jan", "MM", "01", "M", "1",
		"dd", "02", "d", "2", "EEEE", "Monday", "EEE", "Mon", "HH", "15", "H", "15", "hh", "03", "h", "3",
		"mm", "04", "ss", "05", "SSS", "000", "a", "PM", "zzz", "MST", "Z", "-0700", "'T'", "T", "'", "")
	return r.Replace(*f.DateFormat)
}

func (f *DateFormatter) loc() *time.Location {
	if f.TimeZone != nil && f.TimeZone.Loc != nil {
		return f.TimeZone.Loc
	}
	return time.Local
}

// NSDateFormatterCObjcFrom is Foundation `DateFormatter.string(from:)`.
func NSDateFormatterCObjcFrom(f *DateFormatter, d Date) string {
	return d.Time().In(f.loc()).Format(f.layout())
}

// NSDateFormatterStringFrom is Foundation `DateFormatter.string(from:)`.
func NSDateFormatterStringFrom(f *DateFormatter, d Date) string {
	return NSDateFormatterCObjcFrom(f, d)
}

// NSDateFormatterDateFrom is Foundation `DateFormatter.date(from:)`.
func NSDateFormatterDateFrom(f *DateFormatter, s string) *Date {
	t, err := time.ParseInLocation(f.layout(), s, f.loc())
	if err != nil {
		return nil
	}
	d := dateOf(t)
	return &d
}

// ISO8601DateFormatter is Foundation `ISO8601DateFormatter`.
type ISO8601DateFormatter struct{ FormatOptions ISO8601DateFormatOptions }

// NSISO8601DateFormatterInit is Foundation `ISO8601DateFormatter()`, whose options default to `.withInternetDateTime`.
func NSISO8601DateFormatterInit() *ISO8601DateFormatter {
	return &ISO8601DateFormatter{ISO8601DateFormatOptions{NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithInternetDateTime}}
}

// NSISO8601DateFormatterCObjcFrom is `ISO8601DateFormatter.string(from:)` for internet date-time, optionally with fractional seconds.
func NSISO8601DateFormatterCObjcFrom(f *ISO8601DateFormatter, d Date) string {
	if f.FormatOptions.RawValue&NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithFractionalSeconds != 0 {
		return d.Time().UTC().Round(time.Millisecond).Format("2006-01-02T15:04:05.000Z")
	}
	return d.Time().UTC().Format("2006-01-02T15:04:05Z")
}

// NSISO8601DateFormatterDateFrom is `ISO8601DateFormatter.date(from:)`.
func NSISO8601DateFormatterDateFrom(f *ISO8601DateFormatter, s string) *Date {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return nil
	}
	d := dateOf(t)
	return &d
}

// Lock is Foundation `NSLock`.
type Lock struct{ mu sync.Mutex }

// NSLockInit is Foundation `NSLock()`.
func NSLockInit() *Lock { return &Lock{} }

// NSLockWithLock is Foundation `NSLock.withLock`.
func NSLockWithLock[R any](l *Lock, body func() R) R {
	l.mu.Lock()
	defer l.mu.Unlock()
	return body()
}

// NSLockCObjc is Foundation `NSLock.lock()` / `unlock()`; Objective-C selectors lose their names in the AST.
func NSLockCObjc(l *Lock) {}

// NSLockLock is Foundation `NSLock.lock()`.
func NSLockLock(l *Lock) { l.mu.Lock() }

// NSLockUnlock is Foundation `NSLock.unlock()`.
func NSLockUnlock(l *Lock) { l.mu.Unlock() }

// Mutex is Synchronization `Mutex`.
type Mutex[T any] struct {
	mu sync.Mutex
	v  T
}

// MutexInit is Synchronization `Mutex(_:)`.
func MutexInit[T any](v T) *Mutex[T] { return &Mutex[T]{v: v} }

// MutexWithLock is Synchronization `Mutex.withLock`.
func MutexWithLock[T, R any](m *Mutex[T], body func(*T) R) R {
	m.mu.Lock()
	defer m.mu.Unlock()
	return body(&m.v)
}

// Atomic is Synchronization `Atomic`.
type Atomic[T any] struct {
	mu sync.Mutex
	v  T
}

// AtomicInit is Synchronization `Atomic(_:)`.
func AtomicInit[T any](v T) *Atomic[T] { return &Atomic[T]{v: v} }

// AtomicLoadOrdering is Synchronization `Atomic.load(ordering:)`.
func AtomicLoadOrdering[T any](a *Atomic[T], _ any) T {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.v
}

// AtomicStoreOrdering is Synchronization `Atomic.store(_:ordering:)`.
func AtomicStoreOrdering[T any](a *Atomic[T], v T, _ any) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.v = v
}

// ContinuousClockInstant is `ContinuousClock.Instant`.
type ContinuousClockInstant time.Time

// InstantNow is `ContinuousClock.now`.
func InstantNow() ContinuousClockInstant { return ContinuousClockInstant(time.Now()) }

// DispatchTime is Dispatch `DispatchTime`.
type DispatchTime uint64

// DispatchTimeNow is Dispatch `DispatchTime.now()`.
func DispatchTimeNow() DispatchTime { return DispatchTime(time.Since(processStart).Nanoseconds()) }

// DispatchTimeUptimeNanoseconds is Dispatch `DispatchTime.uptimeNanoseconds`.
func DispatchTimeUptimeNanoseconds(t DispatchTime) uint64 { return uint64(t) }

var processStart = time.Now()

// Duration is Swift `Duration`, in nanoseconds.
type Duration int64

// DurationZero is `Duration.zero`.
const DurationZero Duration = 0

// DurationSecondsFloat is `Duration.seconds(Double)`.
func DurationSecondsFloat(s float64) Duration { return Duration(s * 1e9) }

// DurationSecondsInt is `Duration.seconds(Int)`.
func DurationSecondsInt(s int) Duration { return Duration(int64(s) * 1e9) }

// DurationMillisecondsInt is `Duration.milliseconds(Int)`.
func DurationMillisecondsInt(ms int) Duration { return Duration(int64(ms) * 1e6) }

// DurationMilliseconds is `Duration.milliseconds(_:)`.
func DurationMilliseconds[T Integer](ms T) Duration { return Duration(int64(ms) * 1e6) }

// StringInitFormatSlice is Foundation `String(format:_:)`, translating %@ and %ld to Go verbs.
func StringInitFormatSlice(format string, args []any) string {
	r := strings.NewReplacer("%@", "%v", "%ld", "%d", "%lld", "%d", "%lu", "%d", "%llu", "%d", "%li", "%d")
	return fmt.Sprintf(r.Replace(format), args...)
}

// StringPaddingToLengthWithPadStartingAt is Foundation `padding(toLength:withPad:startingAt:)`.
func StringPaddingToLengthWithPadStartingAt(s string, n int, pad string, start int) string {
	cs := StringChars(s)
	if len(cs) >= n {
		return strings.Join(cs[:n], "")
	}
	ps := StringChars(pad)
	var b strings.Builder
	b.WriteString(s)
	for i := 0; len(cs)+i < n; i++ {
		b.WriteString(ps[(start+i)%len(ps)])
	}
	return b.String()
}

// StringCapitalized is Foundation `capitalized`.
func StringCapitalized(s string) string {
	words := strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' })
	_ = words
	var b strings.Builder
	up := true
	for _, r := range s {
		if up {
			b.WriteString(strings.ToUpper(string(r)))
		} else {
			b.WriteString(strings.ToLower(string(r)))
		}
		up = r == ' ' || r == '\t' || r == '\n' || r == '-'
	}
	return b.String()
}

// StringLocalizedCaseInsensitiveCompare is Foundation `localizedCaseInsensitiveCompare`.
func StringLocalizedCaseInsensitiveCompare(a, b string) int {
	return StringCaseInsensitiveCompare(a, b)
}
