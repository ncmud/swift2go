package swiftrt

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
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

func dateOf(t time.Time) Date { return Date(float64(t.Unix()) + float64(t.Nanosecond())/1e9) }

// CalendarDateFrom is Foundation `Calendar.date(from:)` for the Gregorian calendar; missing fields take their smallest
// value. Like Foundation, dates before 1582-10-15 are read in the Julian calendar. Other Calendar helpers here
// are proleptic Gregorian.
func CalendarDateFrom(c Calendar, dc DateComponents) *Date {
	get := func(p *int, d int) int {
		if p == nil {
			return d
		}
		return *p
	}
	first := time.Date(get(dc.Year, 1), time.Month(get(dc.Month, 1)), 1, get(dc.Hour, 0),
		get(dc.Minute, 0), get(dc.Second, 0), get(dc.Nanosecond, 0), c.TimeZone.Loc)
	y, m := first.Year(), int(first.Month())
	day := get(dc.Day, 1)
	t := first.AddDate(0, 0, day-1)
	if y < 1582 || (y == 1582 && (m < 10 || (m == 10 && day < 15))) {
		a := (14 - m) / 12
		yy := y + 4800 - a
		gregorianMinusJulian := -yy/100 + yy/400 + 38
		t = first.AddDate(0, 0, day-1-gregorianMinusJulian)
	}
	d := dateOf(t)
	return &d
}

func calendarAdd(t time.Time, k CalendarComponent, n int) time.Time {
	shift := func(unit int64) time.Time {
		return time.Unix(Add(t.Unix(), Mul(int64(n), unit)), int64(t.Nanosecond())).In(t.Location())
	}
	switch k {
	case CalendarComponentYear, CalendarComponentMonth:
		months := n
		if k == CalendarComponentYear {
			months = Mul(n, 12)
		}
		y, m, d := t.Date()
		first := time.Date(y, m+time.Month(months), 1, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
		last := first.AddDate(0, 1, -1).Day()
		return first.AddDate(0, 0, min(d, last)-1)
	case CalendarComponentDay, CalendarComponentWeekday:
		return t.AddDate(0, 0, n)
	case CalendarComponentHour:
		return shift(3600)
	case CalendarComponentMinute:
		return shift(60)
	case CalendarComponentSecond:
		return shift(1)
	case CalendarComponentNanosecond:
		sec, ns := int64(n)/1e9, int64(n)%1e9
		return time.Unix(Add(t.Unix(), sec), int64(t.Nanosecond())+ns).In(t.Location())
	}
	panic("swiftrt: unsupported calendar component")
}

// CalendarDateByAddingValueTo is Foundation `Calendar.date(byAdding:value:to:)`.
func CalendarDateByAddingValueTo(c Calendar, k CalendarComponent, n int, d Date) *Date {
	r := dateOf(calendarAdd(d.Time().In(c.TimeZone.Loc), k, n))
	return &r
}

// CalendarDateComponentsFromTo is Foundation `Calendar.dateComponents(_:from:to:)`: each requested unit, largest first,
// is the most whole units that can be added without passing the end date.
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
		secs := float64(end.Unix()-cur.Unix()) + float64(end.Nanosecond()-cur.Nanosecond())/1e9
		var n int
		switch o.k {
		case CalendarComponentYear:
			n = end.Year() - cur.Year()
		case CalendarComponentMonth:
			n = (end.Year()-cur.Year())*12 + int(end.Month()) - int(cur.Month())
		case CalendarComponentDay:
			n = int(secs / 86400)
		case CalendarComponentHour:
			n = int(secs / 3600)
		case CalendarComponentMinute:
			n = int(secs / 60)
		case CalendarComponentSecond:
			n = int(secs)
		default:
			n = int((end.Unix()-cur.Unix())*1e9) + end.Nanosecond() - cur.Nanosecond()
		}
		for n != 0 && !within(calendarAdd(cur, o.k, n)) {
			n -= step
		}
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

// commonShortNames are the abbreviations of the metazones whose short names CLDR's English data marks as
// commonly used: Eastern, Central, Mountain, Pacific, Alaska, Hawaii-Aleutian and Atlantic time.
var commonShortNames = map[string]bool{
	"EST": true, "EDT": true, "CST": true, "CDT": true, "MST": true, "MDT": true, "PST": true, "PDT": true,
	"AKST": true, "AKDT": true, "HST": true, "HDT": true, "AST": true, "ADT": true,
}

// commonShortName reports whether ICU's English short specific name of the zone id, at a moment Go
// abbreviates as abbr, is abbr: the zone is one of the North American zones whose names are common.
// Cuba's zone shares Central time's abbreviations but not its metazone.
func commonShortName(id, abbr string) bool {
	if !commonShortNames[abbr] {
		return false
	}
	switch {
	case id == "America/Havana" || id == "Cuba":
		return false
	case strings.HasPrefix(id, "America/"), strings.HasPrefix(id, "US/"), strings.HasPrefix(id, "Canada/"),
		id == "Pacific/Honolulu", id == "Atlantic/Bermuda", id == "EST5EDT", id == "CST6CDT", id == "MST7MDT", id == "PST8PDT",
		id == "EST", id == "MST", id == "HST", id == "Local":
		return true
	}
	return false
}

// TimeZoneAbbreviationFor is Foundation `TimeZone.abbreviation(for:)`.
func TimeZoneAbbreviationFor(z TimeZone, d Date) *string {
	name, off := d.Time().In(z.Loc).Zone()
	if !commonShortName(z.Loc.String(), name) {
		// ICU names a zone by its English short name only where English speakers commonly use it, the
		// North American zones; it names every other zone by its offset from GMT.
		name = "GMT"
	}
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

type groupingRule struct {
	sep         string
	indian      bool
	minGrouping int
}

// groupingRules lists the locales whose IntegerFormatStyle output was checked against Foundation.
var groupingRules = map[string]groupingRule{
	"en_US": {",", false, 1}, "en_GB": {",", false, 1}, "ja_JP": {",", false, 1}, "zh_CN": {",", false, 1},
	"ko_KR": {",", false, 1}, "en_IN": {",", true, 1},
	"de_DE": {".", false, 1}, "pt_BR": {".", false, 1}, "nl_NL": {".", false, 1}, "da_DK": {".", false, 1},
	"tr_TR": {".", false, 1}, "id_ID": {".", false, 1}, "el_GR": {".", false, 1},
	"es_ES": {".", false, 2}, "it_IT": {".", false, 2}, "pt_PT": {"\u00a0", false, 2},
	"de_CH": {"'", false, 1}, "fr_FR": {"\u202f", false, 1}, "en_US_POSIX": {"", false, 1},
}

func groupDigits(digits string, r groupingRule) string {
	if r.sep == "" || len(digits) < 4 || len(digits) < 3+r.minGrouping {
		return digits
	}
	head, tail := digits[:len(digits)-3], digits[len(digits)-3:]
	size := 3
	if r.indian {
		size = 2
	}
	var groups []string
	for len(head) > size {
		groups = append([]string{head[len(head)-size:]}, groups...)
		head = head[:len(head)-size]
	}
	groups = append([]string{head}, groups...)
	return strings.Join(append(groups, tail), r.sep)
}

func (s IntegerFormatStyle) format(neg bool, digits string) string {
	id := "en_US"
	if s.Locale != nil {
		id = s.Locale.Identifier
	}
	r, ok := groupingRules[id]
	if !ok {
		panic("swiftrt: IntegerFormatStyle grouping is unverified for locale " + id)
	}
	out := groupDigits(digits, r)
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

// IntFormatted is `BinaryInteger.formatted(_:)`, and `formatted()` when no style is given: the
// number with the grouping of the style's locale.
func IntFormatted[T Integer](v T, style ...IntegerFormatStyle) string {
	if len(style) == 0 {
		return IntegerFormatStyleFormat(IntegerFormatStyleNumber, v)
	}
	return IntegerFormatStyleFormat(style[0], v)
}

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
		werr = os.Chmod(f.Name(), 0o644) //nolint:gosec // Data.write(to:) creates world-readable files, as Foundation does
	}
	if werr == nil {
		werr = os.Rename(f.Name(), path)
	}
	if werr != nil {
		_ = os.Remove(f.Name())
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

// DispatchQueue is Dispatch `DispatchQueue`: a serial queue whose unbounded FIFO is drained by one goroutine.
type DispatchQueue struct {
	Label   string
	mu      sync.Mutex
	jobs    []func()
	running bool
	// concurrent queues start every job on its own goroutine instead of draining them in order.
	concurrent bool
}

// OS_dispatch_queue is the Objective-C class name the emitter uses for `DispatchQueue`.
type OS_dispatch_queue = *DispatchQueue

// enqueue appends a job without blocking, starting the worker when the queue was idle.
func (q *DispatchQueue) enqueue(f func()) {
	if q.concurrent {
		go f()
		return
	}
	q.mu.Lock()
	q.jobs = append(q.jobs, f)
	start := !q.running
	q.running = true
	q.mu.Unlock()
	if start {
		go q.drain()
	}
}

func (q *DispatchQueue) drain() {
	for {
		q.mu.Lock()
		if len(q.jobs) == 0 {
			q.running = false
			q.mu.Unlock()
			return
		}
		job := q.jobs[0]
		q.jobs[0] = nil
		q.jobs = q.jobs[1:]
		q.mu.Unlock()
		job()
	}
}

// OS_dispatch_queueInitLabel is Dispatch `DispatchQueue(label:)`.
func OS_dispatch_queueInitLabel(label string) *DispatchQueue { return &DispatchQueue{Label: label} }

// DispatchQoS is Dispatch `DispatchQoS`. A queue's quality of service sets only its scheduling
// priority, which goroutines do not have.
type DispatchQoS int

// DispatchQoS classes.
const (
	DispatchQoSUnspecified DispatchQoS = iota
	DispatchQoSBackground
	DispatchQoSUtility
	DispatchQoSDefault
	DispatchQoSUserInitiated
	DispatchQoSUserInteractive
)

// DispatchQoSQoSClass is Dispatch `DispatchQoS.QoSClass`.
type DispatchQoSQoSClass int

// DispatchQoS.QoSClass cases.
const (
	DispatchQoSQoSClassUnspecified DispatchQoSQoSClass = iota
	DispatchQoSQoSClassBackground
	DispatchQoSQoSClassUtility
	DispatchQoSQoSClassDefault
	DispatchQoSQoSClassUserInitiated
	DispatchQoSQoSClassUserInteractive
)

var globalQueues = [...]*DispatchQueue{
	DispatchQoSQoSClassBackground:      {Label: "com.apple.root.background-qos", concurrent: true},
	DispatchQoSQoSClassUtility:         {Label: "com.apple.root.utility-qos", concurrent: true},
	DispatchQoSQoSClassDefault:         {Label: "com.apple.root.default-qos", concurrent: true},
	DispatchQoSQoSClassUserInitiated:   {Label: "com.apple.root.user-initiated-qos", concurrent: true},
	DispatchQoSQoSClassUserInteractive: {Label: "com.apple.root.user-interactive-qos", concurrent: true},
}

// OS_dispatch_queueGlobalQos is Dispatch `DispatchQueue.global(qos:)`: the shared concurrent queue
// for the class, the same instance on every call; `.unspecified` gives the default queue. `async`
// work on it runs on its own goroutine.
func OS_dispatch_queueGlobalQos(qos DispatchQoSQoSClass) *DispatchQueue {
	if qos <= DispatchQoSQoSClassUnspecified || int(qos) >= len(globalQueues) {
		qos = DispatchQoSQoSClassDefault
	}
	return globalQueues[qos]
}

// OS_dispatch_queueInitLabelQos is Dispatch `DispatchQueue(label:qos:)`: a serial queue.
func OS_dispatch_queueInitLabelQos(label string, _ DispatchQoS) *DispatchQueue {
	return OS_dispatch_queueInitLabel(label)
}

// OS_dispatch_queueAsyncExecute is Dispatch `DispatchQueue.async(execute:)`.
func OS_dispatch_queueAsyncExecute(q *DispatchQueue, f func()) { q.enqueue(f) }

// OS_dispatch_queueSyncExecute is Dispatch `DispatchQueue.sync(execute:)` returning a value.
func OS_dispatch_queueSyncExecute[R any](q *DispatchQueue, f func() R) R {
	done := make(chan struct{})
	var out R
	var panicked any
	q.enqueue(func() {
		defer func() {
			panicked = recover()
			close(done)
		}()
		out = f()
	})
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

// ErrorLocalizedDescription is Foundation `Error.localizedDescription`: a LocalizedError's
// `errorDescription` when it is non-nil, an NSError's message, and otherwise Linux Foundation's
// "The operation could not be completed. (Module.Type error N.)". A generated type names itself
// with SwiftTypeName; one that does not is named by its bare Go type. N is the raw value of an enum
// with integer raw values, the case index of another payload-free enum, and 1 for anything else.
// A payload enum's case tags are not recorded, so its cases get 1 as well.
func ErrorLocalizedDescription(err any) string {
	v := err
	if t, ok := v.(Thrown); ok {
		v = t.Err
	}
	v = Dynamic(v)
	if d, ok := v.(interface{ ErrorDescription() *string }); ok {
		if p := d.ErrorDescription(); p != nil {
			return *p
		}
	}
	switch e := v.(type) {
	case NSError:
		return e.Msg
	case URLError:
		return e.localizedDescription()
	case CancellationError:
		return "The operation could not be completed. (Swift.CancellationError error 1.)"
	}
	t := reflect.TypeOf(v)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if e, ok := v.(error); ok && (t == nil || !strings.Contains(t.PkgPath(), ".")) {
		return "The operation could not be completed. " + e.Error()
	}
	if n, ok := v.(SwiftNamed); ok {
		return "The operation could not be completed. (" + n.SwiftTypeName() + " error " + strconv.Itoa(swiftErrorCode(v)) + ".)"
	}
	return "The operation could not be completed. (" + typeName(t) + " error 1.)"
}

// swiftErrorCode is Swift's `Error._code` for a generated error value: an integer raw value, else
// the case index a payload-free enum is stored as, else 1.
func swiftErrorCode(v any) int {
	rv := reflect.ValueOf(v)
	if m := rv.MethodByName("RawValue"); m.IsValid() && m.Type().NumIn() == 0 && m.Type().NumOut() == 1 {
		if raw := m.Call(nil)[0]; raw.CanInt() {
			return int(raw.Int())
		}
	}
	if rv.CanInt() {
		return int(rv.Int())
	}
	return 1
}

// DataInitCapacity is Foundation `Data(capacity:)`: empty, with room for n bytes.
func DataInitCapacity(n int) []byte { return make([]byte, 0, max(n, 0)) }

// DataAppendInt is Foundation `Data.append(_:)` with one byte.
func DataAppendInt(b *[]byte, v uint8) { *b = append(*b, v) }

// DataInitUTF8View is Foundation `Data(_:)` over a String's or Substring's UTF-8 view.
func DataInitUTF8View(utf8 []byte) []byte { return slices.Clone(utf8) }

// DataBase64EncodedString is Foundation `Data.base64EncodedString()`: standard alphabet, padded,
// without line breaks.
func DataBase64EncodedString(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// DataInitUnsafeRawBufferPointer is Foundation `Data(_: UnsafeRawBufferPointer)`: a copy of the
// buffer's bytes.
func DataInitUnsafeRawBufferPointer(p any) []byte { return SliceInitUnsafeRawBufferPointer[[]byte](p) }

// DataInitBytesCount is Foundation `Data(bytes:count:)`: a copy of the n bytes at a C pointer.
func DataInitBytesCount(p any, n int) []byte {
	if n < 0 {
		Trap("Negative value is not representable")
	}
	buf, _ := cBuffer(p, n)
	if len(buf) < n {
		Trap("Data(bytes:count:) reads past the end of the buffer")
	}
	return slices.Clone(buf)
}

// WithExtendedLifetime is `withExtendedLifetime(_:_:)`: it runs body and keeps x reachable until body returns.
func WithExtendedLifetime(x any, body func()) {
	body()
	runtime.KeepAlive(x)
}

// DateComponentsInitYearMonthDayHour is `DateComponents(year:month:day:hour:)`.
func DateComponentsInitYearMonthDayHour(y, m, d, h *int) DateComponents {
	c := DateComponentsInitYearMonthDay(y, m, d)
	c.Hour = h
	return c
}

// DataInitRepeatingCount is `Data(repeating:count:)`.
func DataInitRepeatingCount(b uint8, n int) []byte {
	out := make([]byte, max(n, 0))
	for i := range out {
		out[i] = b
	}
	return out
}

// CFNSTemporaryDirectory is Foundation `NSTemporaryDirectory()`: the temporary directory, ending in a slash.
func CFNSTemporaryDirectory() string {
	d := os.TempDir()
	if !strings.HasSuffix(d, "/") {
		d += "/"
	}
	return d
}

// URLFragment is `URL.fragment`: the text after "#", or nil when the URL has none.
func URLFragment(u URL) *string {
	i := strings.IndexByte(u.Raw, '#')
	if i < 0 {
		return nil
	}
	f := u.Raw[i+1:]
	return &f
}
