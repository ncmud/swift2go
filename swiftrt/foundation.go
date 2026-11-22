package swiftrt

import (
	"encoding/hex"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf16"
	"uuid"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// UUID is Foundation `UUID`.
type UUID [16]byte

// UUIDInit is Foundation `UUID()`: a random version-4 UUID.
func UUIDInit() UUID {
	return UUID(uuid.NewV4())
}

// UUIDUuidString is Foundation `UUID.uuidString` (uppercase, hyphenated).
func UUIDUuidString(u UUID) string {
	return strings.ToUpper(uuid.UUID(u).String())
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
	AmSymbol   *string
	PmSymbol   *string
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

// NSDateFormatterStringFrom is Foundation `DateFormatter.string(from:)`.
func NSDateFormatterStringFrom(f *DateFormatter, d Date) string {
	t := d.Time().In(f.loc())
	if f.DateFormat == nil || (f.AmSymbol == nil && f.PmSymbol == nil) {
		return t.Format(f.layout())
	}
	// Custom AM/PM symbols: format a placeholder where each unquoted `a` is, then substitute.
	sym := "AM"
	if f.AmSymbol != nil {
		sym = *f.AmSymbol
	}
	if t.Hour() >= 12 {
		sym = "PM"
		if f.PmSymbol != nil {
			sym = *f.PmSymbol
		}
	}
	const mark = "\x01"
	saved := *f.DateFormat
	marked := replaceUnquoted(saved, 'a', "'"+mark+"'")
	f.DateFormat = &marked
	out := t.Format(f.layout())
	f.DateFormat = &saved
	return strings.ReplaceAll(out, mark, sym)
}

// replaceUnquoted replaces each occurrence of c outside '…' literal text in a date format.
func replaceUnquoted(format string, c rune, repl string) string {
	var b strings.Builder
	quoted := false
	for _, r := range format {
		switch {
		case r == '\'':
			quoted = !quoted
			b.WriteRune(r)
		case r == c && !quoted:
			b.WriteString(repl)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
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

// RecursiveLock is Foundation `NSRecursiveLock`: the goroutine that holds it may lock it again.
type RecursiveLock struct {
	mu    sync.Mutex
	owner atomic.Uint64
	depth int
}

// NSRecursiveLockInit is Foundation `NSRecursiveLock()`.
func NSRecursiveLockInit() *RecursiveLock { return &RecursiveLock{} }

// NSRecursiveLockLock is Foundation `NSRecursiveLock.lock()`.
func NSRecursiveLockLock(l *RecursiveLock) {
	g := goroutineID()
	if l.owner.Load() == g {
		l.depth++
		return
	}
	l.mu.Lock()
	l.owner.Store(g)
	l.depth = 1
}

// NSRecursiveLockUnlock is Foundation `NSRecursiveLock.unlock()`.
func NSRecursiveLockUnlock(l *RecursiveLock) {
	l.depth--
	if l.depth == 0 {
		l.owner.Store(0)
		l.mu.Unlock()
	}
}

// NSRecursiveLockWithLock is Foundation `NSRecursiveLock.withLock`.
func NSRecursiveLockWithLock[R any](l *RecursiveLock, body func() R) R {
	NSRecursiveLockLock(l)
	defer NSRecursiveLockUnlock(l)
	return body()
}

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

// AtomicExchangeOrdering is Synchronization `Atomic.exchange(_:ordering:)`: it stores v and returns
// the value it replaced.
func AtomicExchangeOrdering[T any](a *Atomic[T], v T, _ any) T {
	a.mu.Lock()
	defer a.mu.Unlock()
	old := a.v
	a.v = v
	return old
}

// AtomicUpdate is the result of an Atomic read-modify-write: the value before and after.
type AtomicUpdate[T any] struct {
	OldValue T
	NewValue T
}

func atomicUpdate[T any](a *Atomic[T], f func(T) T) AtomicUpdate[T] {
	a.mu.Lock()
	defer a.mu.Unlock()
	old := a.v
	a.v = f(old)
	return AtomicUpdate[T]{OldValue: old, NewValue: a.v}
}

// AtomicWrappingAddOrdering is Synchronization `Atomic.wrappingAdd(_:ordering:)`.
func AtomicWrappingAddOrdering[T Integer](a *Atomic[T], v T, _ any) AtomicUpdate[T] {
	return atomicUpdate(a, func(old T) T { return old + v })
}

// AtomicBitwiseOrOrdering is Synchronization `Atomic.bitwiseOr(_:ordering:)`.
func AtomicBitwiseOrOrdering[T Integer](a *Atomic[T], v T, _ any) AtomicUpdate[T] {
	return atomicUpdate(a, func(old T) T { return old | v })
}

// AtomicUpdateOrderingRelaxed is `AtomicUpdateOrdering.relaxed`.
var AtomicUpdateOrderingRelaxed any = "relaxed"

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

// Description is Swift's `Duration.description`: the length in seconds as a Double, as in "0.05 seconds".
func (d Duration) Description() string {
	whole, frac := int64(d)/1e9, int64(d)%1e9
	return formatDouble(float64(whole)+float64(frac)*1e-9) + " seconds"
}

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

// DurationMicroseconds is `Duration.microseconds(_:)`.
func DurationMicroseconds[T Integer](us T) Duration { return Duration(int64(us) * 1e3) }

// DurationNanoseconds is `Duration.nanoseconds(_:)`.
func DurationNanoseconds[T Integer](ns T) Duration { return Duration(int64(ns)) }

// StringInitFormatSlice is Foundation `String(format:_:)`.
func StringInitFormatSlice(format string, args []any) string {
	var b strings.Builder
	next := 0
	arg := func(i int) any {
		if i < 0 || i >= len(args) {
			return nil
		}
		return args[i]
	}
	for i := 0; i < len(format); i++ {
		c := format[i]
		if c != '%' {
			b.WriteByte(c)
			continue
		}
		spec, verb, n := parseFormatSpec(format[i+1:])
		i += n
		switch verb {
		case 0:
			b.WriteString(format[i-n : i+1])
			continue
		case '%':
			b.WriteByte('%')
			continue
		}
		if spec.position > 0 {
			next = spec.position - 1
		}
		if spec.widthArg {
			if w, ok := arg(next).(int); ok {
				spec.width = strconv.Itoa(w)
			}
			next++
		}
		if spec.precArg {
			if p, ok := arg(next).(int); ok {
				spec.prec = "." + strconv.Itoa(p)
			}
			next++
		}
		b.WriteString(formatOne(spec, verb, arg(next)))
		next++
	}
	return b.String()
}

type formatSpec struct {
	position          int
	flags, width      string
	prec, length      string
	widthArg, precArg bool
}

// parseFormatSpec reads one conversion after its '%': the spec, the conversion character (0 if
// there is none), and the number of bytes read.
func parseFormatSpec(f string) (formatSpec, byte, int) {
	var spec formatSpec
	i := 0
	digits := func() string {
		j := i
		for i < len(f) && f[i] >= '0' && f[i] <= '9' {
			i++
		}
		return f[j:i]
	}
	if d := digits(); d != "" && i < len(f) && f[i] == '$' {
		spec.position, _ = strconv.Atoi(d)
		i++
	} else {
		i -= len(d)
	}
	for i < len(f) && strings.IndexByte("-+ #0'", f[i]) >= 0 {
		if f[i] != '\'' {
			spec.flags += string(f[i])
		}
		i++
	}
	if i < len(f) && f[i] == '*' {
		spec.widthArg = true
		i++
	} else {
		spec.width = digits()
	}
	if i < len(f) && f[i] == '.' {
		i++
		if i < len(f) && f[i] == '*' {
			spec.precArg = true
			i++
		} else {
			spec.prec = "." + digits()
		}
	}
	for i < len(f) && strings.IndexByte("hlqLzjt", f[i]) >= 0 {
		spec.length += string(f[i])
		i++
	}
	if i >= len(f) {
		return spec, 0, i
	}
	return spec, f[i], i + 1
}

// formatOne formats one argument as Foundation does for the conversion verb.
func formatOne(spec formatSpec, verb byte, v any) string {
	pre := "%" + spec.flags + spec.width + spec.prec
	switch verb {
	case '@':
		// Foundation ignores the width and precision of %@.
		return Str(v)
	case 'd', 'i':
		return fmt.Sprintf(pre+"d", v)
	case 'u', 'x', 'X', 'o':
		if verb == 'u' {
			verb = 'd'
		}
		if n, ok := asInt64(v); ok && n < 0 {
			if spec.length == "" || spec.length == "h" || spec.length == "hh" {
				return fmt.Sprintf(pre+string(verb), uint32(n))
			}
			return fmt.Sprintf(pre+string(verb), uint64(n))
		}
		return fmt.Sprintf(pre+string(verb), v)
	case 'g', 'G':
		if spec.prec == "" {
			pre += ".6"
		}
		return fmt.Sprintf(pre+string(verb), v)
	case 'c', 'C':
		if n, ok := asInt64(v); ok {
			return string(rune(n))
		}
		return Str(v)
	case 's', 'S':
		return fmt.Sprintf(pre+"s", Str(v))
	}
	return fmt.Sprintf(pre+string(verb), v)
}

func asInt64(v any) (int64, bool) {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int(), true
	}
	return 0, false
}

// StringPaddingToLengthWithPadStartingAt is Foundation `padding(toLength:withPad:startingAt:)`,
// which counts UTF-16 code units.
func StringPaddingToLengthWithPadStartingAt(s string, n int, pad string, start int) string {
	units := utf16.Encode([]rune(s))
	if len(units) >= n {
		return string(utf16.Decode(units[:n]))
	}
	ps := utf16.Encode([]rune(pad))
	for i := 0; len(units) < n; i++ {
		units = append(units, ps[(start+i)%len(ps)])
	}
	return string(utf16.Decode(units))
}

// StringCapitalized is Foundation `capitalized`: each cased letter that starts a word is
// titlecased and the rest are lowercased. Marks, format characters, modifier letters and symbols,
// and apostrophes leave the word going; any other character ends it.
func StringCapitalized(s string) string {
	var b strings.Builder
	inWord := false
	for _, r := range s {
		switch {
		case unicode.In(r, unicode.Lu, unicode.Ll, unicode.Lt):
			if inWord {
				b.WriteString(mapRune(r, lowerSpecial, unicode.ToLower))
			} else {
				b.WriteString(mapRune(r, titleSpecial, unicode.ToTitle))
			}
			inWord = true
		case r == '\'' || r == '’' || unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf, unicode.Lm, unicode.Sk):
			b.WriteRune(r)
		default:
			b.WriteRune(r)
			inWord = false
		}
	}
	return b.String()
}

// StringLocalizedCaseInsensitiveCompare is Foundation `localizedCaseInsensitiveCompare`: the CLDR
// root collation, which ICU gives Foundation's localized comparisons, ignoring case. Punctuation
// orders by its collation weight, not its code point: "-" sorts before "(".
func StringLocalizedCaseInsensitiveCompare(a, b string) int {
	return collateCompare(&caseInsensitiveCollators, a, b, collate.IgnoreCase)
}

// StringLocalizedCompare is Foundation `localizedCompare`.
func StringLocalizedCompare(a, b string) int { return collateCompare(&localizedCollators, a, b) }

// StringLocalizedStandardCompare is Foundation `localizedStandardCompare`: collation with digits
// compared by their numeric value.
func StringLocalizedStandardCompare(a, b string) int {
	return collateCompare(&standardCollators, a, b, collate.Numeric)
}

// A Collator is not safe for concurrent use, so each comparison borrows one.
var caseInsensitiveCollators, localizedCollators, standardCollators sync.Pool

func collateCompare(pool *sync.Pool, a, b string, opts ...collate.Option) int {
	c, _ := pool.Get().(*collate.Collator)
	if c == nil {
		c = collate.New(language.Und, opts...)
	}
	r := c.CompareString(a, b)
	pool.Put(c)
	return r
}
