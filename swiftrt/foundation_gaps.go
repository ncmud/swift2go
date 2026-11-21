package swiftrt

import (
	"encoding/binary"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DateISO8601Format is Foundation `Date.ISO8601Format()`: UTC, whole seconds.
func DateISO8601Format(d Date) string {
	return d.Time().UTC().Format("2006-01-02T15:04:05Z")
}

// ISO8601FormatStyle is Foundation `Date.ISO8601FormatStyle` with the fields chosen by its builder methods.
type ISO8601FormatStyle struct{ year, month, day bool }

// ISO8601FormatStyleIso8601 is `.iso8601`.
var ISO8601FormatStyleIso8601 = ISO8601FormatStyle{}

// ISO8601FormatStyleYear is `ISO8601FormatStyle.year()`.
func ISO8601FormatStyleYear(s ISO8601FormatStyle) ISO8601FormatStyle { s.year = true; return s }

// ISO8601FormatStyleMonth is `ISO8601FormatStyle.month()`.
func ISO8601FormatStyleMonth(s ISO8601FormatStyle) ISO8601FormatStyle { s.month = true; return s }

// ISO8601FormatStyleDay is `ISO8601FormatStyle.day()`.
func ISO8601FormatStyleDay(s ISO8601FormatStyle) ISO8601FormatStyle { s.day = true; return s }

// DateFormatted is Foundation `Date.formatted(_:)` with an ISO8601FormatStyle in GMT.
func DateFormatted(d Date, s ISO8601FormatStyle) string {
	t := d.Time().UTC()
	if !s.year && !s.month && !s.day {
		return t.Format("2006-01-02T15:04:05Z")
	}
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
	return strings.Join(parts, "-")
}

// ISO8601DateFormatOptions is Foundation `ISO8601DateFormatter.Options`.
type ISO8601DateFormatOptions struct{ RawValue uint }

// ISO8601DateFormatter.Options members, under the names the emitter gives imported Objective-C constants.
const (
	NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithInternetDateTime     = 1907
	NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithFractionalSeconds    = 2048
	NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithFullDate             = 275
	NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithFullTime             = 1632
	NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithColonSeparatorInTime = 512
)

// DateFormatterStyle is Foundation `DateFormatter.Style`.
type DateFormatterStyle = int

// DateFormatter.Style cases.
const (
	DateFormatterStyleNone DateFormatterStyle = iota
	DateFormatterStyleShort
	DateFormatterStyleMedium
	DateFormatterStyleLong
	DateFormatterStyleFull
)

// DateComponentsInitYearMonthDay is Foundation `DateComponents(year:month:day:)`.
func DateComponentsInitYearMonthDay(y, m, d *int) DateComponents {
	return DateComponents{Year: y, Month: m, Day: d}
}

// DateComponentsSecond is `DateComponents.second`.
func DateComponentsSecond(d DateComponents) *int { return d.Second }

// DateComponentsWeekday is `DateComponents.weekday`.
func DateComponentsWeekday(d DateComponents) *int { return d.Weekday }

func dateOf(t time.Time) Date { return Date(float64(t.UnixNano()) / 1e9) }

// CalendarDateFrom is Foundation `Calendar.date(from:)`; missing fields take their smallest value.
func CalendarDateFrom(c Calendar, dc DateComponents) *Date {
	get := func(p *int, d int) int {
		if p == nil {
			return d
		}
		return *p
	}
	t := time.Date(get(dc.Year, 1), time.Month(get(dc.Month, 1)), get(dc.Day, 1), get(dc.Hour, 0),
		get(dc.Minute, 0), get(dc.Second, 0), get(dc.Nanosecond, 0), c.TimeZone.Loc)
	d := dateOf(t)
	return &d
}

func calendarAdd(t time.Time, k CalendarComponent, n int) time.Time {
	switch k {
	case CalendarComponentYear, CalendarComponentMonth:
		months := n
		if k == CalendarComponentYear {
			months = n * 12
		}
		y, m, d := t.Date()
		first := time.Date(y, m+time.Month(months), 1, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
		last := first.AddDate(0, 1, -1).Day()
		return first.AddDate(0, 0, min(d, last)-1)
	case CalendarComponentDay:
		return t.AddDate(0, 0, n)
	case CalendarComponentWeekday:
		return t.AddDate(0, 0, n)
	case CalendarComponentHour:
		return t.Add(time.Duration(n) * time.Hour)
	case CalendarComponentMinute:
		return t.Add(time.Duration(n) * time.Minute)
	case CalendarComponentSecond:
		return t.Add(time.Duration(n) * time.Second)
	case CalendarComponentNanosecond:
		return t.Add(time.Duration(n))
	}
	panic("swiftrt: unsupported calendar component")
}

// CalendarDateByAddingValueTo is Foundation `Calendar.date(byAdding:value:to:)`.
func CalendarDateByAddingValueTo(c Calendar, k CalendarComponent, n int, d Date) *Date {
	r := dateOf(calendarAdd(d.Time().In(c.TimeZone.Loc), k, n))
	return &r
}

// CalendarDateComponentsFromTo is Foundation `Calendar.dateComponents(_:from:to:)`.
func CalendarDateComponentsFromTo(c Calendar, comps map[CalendarComponent]struct{}, from, to Date) DateComponents {
	cur := from.Time().In(c.TimeZone.Loc)
	end := to.Time().In(c.TimeZone.Loc)
	var out DateComponents
	order := []struct {
		k   CalendarComponent
		dst **int
	}{
		{CalendarComponentYear, &out.Year}, {CalendarComponentMonth, &out.Month}, {CalendarComponentDay, &out.Day},
		{CalendarComponentHour, &out.Hour}, {CalendarComponentMinute, &out.Minute},
		{CalendarComponentSecond, &out.Second}, {CalendarComponentNanosecond, &out.Nanosecond},
	}
	for _, o := range order {
		if _, ok := comps[o.k]; !ok {
			continue
		}
		step := 1
		if end.Before(cur) {
			step = -1
		}
		within := func(t time.Time) bool {
			if step > 0 {
				return !t.After(end)
			}
			return !t.Before(end)
		}
		n := 0
		for within(calendarAdd(cur, o.k, n+step)) {
			n += step
		}
		cur = calendarAdd(cur, o.k, n)
		v := n
		*o.dst = &v
	}
	return out
}

func gmtName(s int) string {
	if s == 0 {
		return "GMT"
	}
	sign := "+"
	if s < 0 {
		sign, s = "-", -s
	}
	name := fmt.Sprintf("GMT%s%02d%02d", sign, s/3600, s%3600/60)
	if s%60 != 0 {
		name += fmt.Sprintf("%02d", s%60)
	}
	return name
}

// TimeZoneOptInitSecondsFromGMT is Foundation `TimeZone(secondsFromGMT:)`; nil beyond ±18 hours.
func TimeZoneOptInitSecondsFromGMT(s int) *TimeZone {
	if s < -18*3600 || s > 18*3600 {
		return nil
	}
	return &TimeZone{time.FixedZone(gmtName(s), s)}
}

// TimeZoneAbbreviationFor is Foundation `TimeZone.abbreviation(for:)`.
func TimeZoneAbbreviationFor(z TimeZone, d Date) *string {
	name, off := d.Time().In(z.Loc).Zone()
	if name == "UTC" || name == "" || name[0] == '+' || name[0] == '-' || strings.HasPrefix(name, "GMT") {
		name = "GMT"
		if off != 0 {
			sign := "+"
			if off < 0 {
				sign, off = "-", -off
			}
			name += sign + strconv.Itoa(off/3600)
			if off%3600 != 0 {
				name += fmt.Sprintf(":%02d", off%3600/60)
			}
		}
	}
	return &name
}

// IntegerFormatStyle is Foundation `IntegerFormatStyle`.
type IntegerFormatStyle struct{ Locale *Locale }

// IntegerFormatStyleNumber is `.number`.
var IntegerFormatStyleNumber = IntegerFormatStyle{}

// IntegerFormatStyleLocale is `IntegerFormatStyle.locale(_:)`.
func IntegerFormatStyleLocale(s IntegerFormatStyle, l Locale) IntegerFormatStyle {
	s.Locale = &l
	return s
}

// IntegerFormatStyleInitLocale is `IntegerFormatStyle(locale:)`.
func IntegerFormatStyleInitLocale(l Locale) IntegerFormatStyle { return IntegerFormatStyle{&l} }

func groupDigits(digits, sep string) string {
	var b strings.Builder
	for i, c := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteString(sep)
		}
		b.WriteRune(c)
	}
	return b.String()
}

func (s IntegerFormatStyle) format(neg bool, digits string) string {
	id := "en_US"
	if s.Locale != nil {
		id = s.Locale.Identifier
	}
	lang := strings.SplitN(strings.SplitN(id, "_", 2)[0], "-", 2)[0]
	var sep string
	switch lang {
	case "en", "ja", "zh", "ko", "he", "th":
		sep = ","
	case "de", "es", "it", "nl", "pt", "da", "id", "tr", "el":
		sep = "."
	default:
		panic("swiftrt: IntegerFormatStyle has no grouping rule for locale " + id)
	}
	if id == "en_US_POSIX" {
		sep = ""
	}
	out := groupDigits(digits, sep)
	if neg {
		return "-" + out
	}
	return out
}

// IntegerFormatStyleFormat is `IntegerFormatStyle.format(_:)`.
func IntegerFormatStyleFormat(s IntegerFormatStyle, v any) string {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n := rv.Int()
		return s.format(n < 0, strconv.FormatUint(uint64(IntMagnitude(n)), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return s.format(false, strconv.FormatUint(rv.Uint(), 10))
	}
	panic("swiftrt.IntegerFormatStyleFormat: not an integer: " + rv.Type().String())
}

// IntFormatted is `BinaryInteger.formatted(_:)`.
func IntFormatted[T Integer](v T, s IntegerFormatStyle) string { return IntegerFormatStyleFormat(s, v) }

// URL is Foundation `URL`, kept as its absolute string.
type URL struct{ Raw string }

// Description is `URL.description`.
func (u URL) Description() string { return u.Raw }

// URLInitFileURLWithPath is Foundation `URL(fileURLWithPath:)`; relative paths resolve against the working directory.
func URLInitFileURLWithPath(p string) URL {
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = p
	}
	if strings.HasSuffix(p, "/") && !strings.HasSuffix(abs, "/") {
		abs += "/"
	}
	return URL{(&url.URL{Scheme: "file", Path: abs}).String()}
}

// URLOptInitString is Foundation `URL(string:)`.
func URLOptInitString(s string) *URL {
	if s == "" {
		return nil
	}
	if _, err := url.Parse(s); err != nil {
		return nil
	}
	return &URL{s}
}

// URLPath is Foundation `URL.path`: percent-decoded, without a trailing slash.
func URLPath(u URL) string {
	p, err := url.Parse(u.Raw)
	if err != nil {
		return ""
	}
	if len(p.Path) > 1 {
		return strings.TrimSuffix(p.Path, "/")
	}
	return p.Path
}

// URLAppendingPathComponentString is Foundation `URL.appendingPathComponent(_:)`.
func URLAppendingPathComponentString(u URL, c string) URL {
	p, err := url.Parse(u.Raw)
	if err != nil {
		return u
	}
	if !strings.HasSuffix(p.Path, "/") {
		p.Path += "/"
	}
	p.Path += c
	p.RawPath = ""
	return URL{p.String()}
}

func fileURLPath(u URL) string {
	p, err := url.Parse(u.Raw)
	if err != nil || p.Scheme != "file" {
		panic(Throw(fmt.Errorf("the file couldn't be opened because %q is not a file URL", u.Raw)))
	}
	return p.Path
}

// DataInitContentsOf is Foundation `Data(contentsOf:)`.
func DataInitContentsOf(u URL) []byte {
	b, err := os.ReadFile(fileURLPath(u))
	if err != nil {
		panic(Throw(err))
	}
	return b
}

// DataWriteTo is Foundation `Data.write(to:)`.
func DataWriteTo(b []byte, u URL) {
	if err := os.WriteFile(fileURLPath(u), b, 0o644); err != nil {
		panic(Throw(err))
	}
}

// StringInitContentsOfFileEncoding is Foundation `String(contentsOfFile:encoding:)`.
func StringInitContentsOfFileEncoding(path string, e StringEncoding) string {
	b, err := os.ReadFile(path)
	if err != nil {
		panic(Throw(err))
	}
	s := StringOptInitBytesEncoding(b, e)
	if s == nil {
		panic(Throw(fmt.Errorf("the file %q couldn't be opened using text encoding UTF-8", path)))
	}
	return *s
}

// StringWriteToFileAtomicallyEncoding is Foundation `String.write(toFile:atomically:encoding:)`.
func StringWriteToFileAtomicallyEncoding(s, path string, atomically bool, _ StringEncoding) {
	if !atomically {
		if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
			panic(Throw(err))
		}
		return
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		panic(Throw(err))
	}
	_, werr := f.WriteString(s)
	cerr := f.Close()
	if werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(f.Name(), 0o644)
	}
	if werr == nil {
		werr = os.Rename(f.Name(), path)
	}
	if werr != nil {
		os.Remove(f.Name())
		panic(Throw(werr))
	}
}

// DataInit is Foundation `Data()`.
func DataInit() []byte { return []byte{} }

// DataInitData is Foundation `Data(data)`.
func DataInitData(b []byte) []byte { return Clone(b) }

// DataCount is `Data.count`.
func DataCount(b []byte) int { return len(b) }

// DataIsEmpty is `Data.isEmpty`.
func DataIsEmpty(b []byte) bool { return len(b) == 0 }

// DataPrefixInt is `Data.prefix(_:)`.
func DataPrefixInt(b []byte, n int) []byte { return SlicePrefixInt(b, n) }

// DataRemoveLast is `Data.removeLast()`.
func DataRemoveLast(b *[]byte) byte { return SliceRemoveLast(b) }

// UUIDTuple is Foundation `uuid_t`.
type UUIDTuple = struct {
	F0, F1, F2, F3, F4, F5, F6, F7, F8, F9, F10, F11, F12, F13, F14, F15 uint8
}

// UUIDInitUuid is Foundation `UUID(uuid:)`.
func UUIDInitUuid(t UUIDTuple) UUID {
	return UUID{t.F0, t.F1, t.F2, t.F3, t.F4, t.F5, t.F6, t.F7, t.F8, t.F9, t.F10, t.F11, t.F12, t.F13, t.F14, t.F15}
}

// UUIDUuid is Foundation `UUID.uuid`.
func UUIDUuid(u UUID) UUIDTuple {
	return UUIDTuple{u[0], u[1], u[2], u[3], u[4], u[5], u[6], u[7], u[8], u[9], u[10], u[11], u[12], u[13], u[14], u[15]}
}

// UUIDOptInitString is Foundation `UUID(uuidString:)`.
func UUIDOptInitString(s string) *UUID { return UUIDOptInitUuidString(s) }

// WithUnsafeBytesOf is Swift `withUnsafeBytes(of:_:)` for values with a fixed, padding-free layout such as uuid_t.
func WithUnsafeBytesOf[T, R any](v T, body func(any) R) R {
	var buf []byte
	buf, err := binary.Append(buf, binary.NativeEndian, v)
	if err != nil || len(buf) != int(reflect.TypeFor[T]().Size()) {
		panic("swiftrt.WithUnsafeBytesOf: unsupported layout for " + reflect.TypeFor[T]().String())
	}
	return body(buf)
}

// UnsafeRawBufferPointerPrefixInt is `UnsafeRawBufferPointer.prefix(_:)`.
func UnsafeRawBufferPointerPrefixInt(p any, n int) []byte { return SlicePrefixInt(p.([]byte), n) }

// CustomStringConvertibleDescription is `CustomStringConvertible.description`.
func CustomStringConvertibleDescription(v any) string { return Str(v) }

// AtomicInitInt is Synchronization `Atomic<Int>(_:)`.
func AtomicInitInt(v int) *Atomic[int] { return AtomicInit(v) }

// AtomicLoadOrderingRelaxed is `AtomicLoadOrdering.relaxed`.
var AtomicLoadOrderingRelaxed any = "relaxed"

// AtomicStoreOrderingRelaxed is `AtomicStoreOrdering.relaxed`.
var AtomicStoreOrderingRelaxed any = "relaxed"

// DispatchQueue is Dispatch `DispatchQueue`: a serial queue drained by one goroutine.
type DispatchQueue struct {
	Label string
	once  sync.Once
	jobs  chan func()
}

// OS_dispatch_queue is the Objective-C class name the emitter uses for `DispatchQueue`.
type OS_dispatch_queue = *DispatchQueue

func (q *DispatchQueue) start() {
	q.once.Do(func() {
		q.jobs = make(chan func(), 1024)
		go func() {
			for job := range q.jobs {
				job()
			}
		}()
	})
}

// OS_dispatch_queueInitLabel is Dispatch `DispatchQueue(label:)`.
func OS_dispatch_queueInitLabel(label string) *DispatchQueue { return &DispatchQueue{Label: label} }

// OS_dispatch_queueAsyncExecute is Dispatch `DispatchQueue.async(execute:)`.
func OS_dispatch_queueAsyncExecute(q *DispatchQueue, f func()) {
	q.start()
	q.jobs <- f
}

// OS_dispatch_queueSyncExecute is Dispatch `DispatchQueue.sync(execute:)` returning a value.
func OS_dispatch_queueSyncExecute[R any](q *DispatchQueue, f func() R) R {
	q.start()
	done := make(chan struct{})
	var out R
	var panicked any
	q.jobs <- func() {
		defer func() {
			panicked = recover()
			close(done)
		}()
		out = f()
	}
	<-done
	if panicked != nil {
		panic(panicked)
	}
	return out
}

// OS_dispatch_queueCFDispatch_syncExecute is Dispatch `DispatchQueue.sync(execute:)` with a Void block.
func OS_dispatch_queueCFDispatch_syncExecute[R any](q *DispatchQueue, f func() R) {
	OS_dispatch_queueSyncExecute(q, f)
}
