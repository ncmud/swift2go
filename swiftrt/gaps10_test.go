package swiftrt

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestTaskVariantsAndContinuation(t *testing.T) {
	var executor any = "executor"
	task := TaskInitExecutorPreferenceOperation(&executor, func() int { return 7 })
	if got := TaskValue(task); got != 7 {
		t.Errorf("task value = %v", got)
	}
	var boxed any = TaskDetachedOperation(func() {})
	if got := TaskValue(&boxed); got != struct{}{} {
		t.Errorf("detached task value = %v", got)
	}

	var waiting []any
	var mu sync.Mutex
	done := make(chan struct{})
	go func() {
		WithCheckedContinuation(func(c any) {
			mu.Lock()
			waiting = append(waiting, c)
			mu.Unlock()
		})
		close(done)
	}()
	for {
		mu.Lock()
		n := len(waiting)
		mu.Unlock()
		if n == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case <-done:
		t.Fatal("withCheckedContinuation returned before its continuation was resumed")
	case <-time.After(10 * time.Millisecond):
	}
	CheckedContinuationResume(waiting[0])
	<-done
	if !panics(func() { CheckedContinuationResume(waiting[0]) }) {
		t.Error("resuming a continuation twice did not trap")
	}
}

func TestDispatchSemaphore(t *testing.T) {
	finished := OS_dispatch_semaphoreInitValue(0)
	start := time.Now()
	if got := OS_dispatch_semaphoreWaitTimeout(finished, DispatchTimeNow()+DispatchTimeIntervalMilliseconds(30)); got != DispatchTimeoutResultTimedOut {
		t.Errorf("wait on an unsignalled semaphore = %v, want timedOut", got)
	}
	if waited := time.Since(start); waited < 25*time.Millisecond {
		t.Errorf("timed out after %v, want about 30ms", waited)
	}
	if got := OS_dispatch_semaphoreWaitTimeout(finished, DispatchTimeNow()); got != DispatchTimeoutResultTimedOut {
		t.Errorf("wait until now = %v, want timedOut", got)
	}
	if woke := OS_dispatch_semaphoreSignal(finished); woke != 0 {
		t.Errorf("signal with no waiter returned %d", woke)
	}
	if got := OS_dispatch_semaphoreWaitTimeout(finished, DispatchTimeNow()+DispatchTimeIntervalSeconds(3)); got != DispatchTimeoutResultSuccess {
		t.Errorf("wait after a signal = %v, want success", got)
	}

	woken := make(chan DispatchTimeoutResult)
	go func() { woken <- OS_dispatch_semaphoreWaitTimeout(finished, DispatchTimeDistantFuture) }()
	for {
		finished.mu.Lock()
		n := len(finished.waiters)
		finished.mu.Unlock()
		if n == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if woke := OS_dispatch_semaphoreSignal(finished); woke == 0 {
		t.Error("signal with a waiter returned 0")
	}
	if got := <-woken; got != DispatchTimeoutResultSuccess {
		t.Errorf("woken wait = %v, want success", got)
	}
	if got := OS_dispatch_semaphoreWaitTimeout(finished, DispatchTimeNow()); got != DispatchTimeoutResultTimedOut {
		t.Error("a signal that woke a waiter also left a count behind")
	}

	pool := OS_dispatch_semaphoreInitValue(2)
	OS_dispatch_semaphoreWait(pool)
	OS_dispatch_semaphoreWait(pool)
	if got := OS_dispatch_semaphoreWaitTimeout(pool, DispatchTimeNow()); got != DispatchTimeoutResultTimedOut {
		t.Error("a semaphore of 2 let a third waiter through")
	}
	if !panics(func() { OS_dispatch_semaphoreInitValue(-1) }) {
		t.Error("a negative semaphore value did not trap")
	}
}

func shell(t *testing.T) URL {
	t.Helper()
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("no /bin/sh")
	}
	return URLInitFileURLWithPath("/bin/sh")
}

func TestProcessRun(t *testing.T) {
	sh := shell(t)
	dir := t.TempDir()
	out := filepath.Join(dir, "out.txt")
	if !NSFileManagerCreateFileAtPathContents(NSFileManagerDefault, out, nil) {
		t.Fatal("createFile failed")
	}
	process := NSTaskInit()
	if NSTaskIsRunning(process) || NSTaskProcessIdentifier(process) != 0 {
		t.Error("a process that has not run is running")
	}
	process.ExecutableURL = &sh
	process.Arguments = new([]string{"-c", `printf '%s:%s' "$GREETING" "$1" >&2; exit 3`, "sh", "arg one"})
	process.Environment = new(map[string]string{"GREETING": "hello"})
	process.StandardOutput = NSFileHandleNullDevice
	process.StandardError = TryNil(func() any { return NSFileHandleInitForWritingTo(URLInitFileURLWithPath(out)) })
	NSTaskRun(process)
	held := struct{ F1 Task }{process}
	if NSTaskProcessIdentifier(held.F1) <= 0 {
		t.Error("a running process has no identifier")
	}
	NSTaskWaitUntilExit(held.F1)
	if NSTaskIsRunning(held.F1) || NSTaskTerminationStatus(held.F1) != 3 {
		t.Errorf("after exit: running %v, status %d, want false and 3", NSTaskIsRunning(held.F1), NSTaskTerminationStatus(held.F1))
	}
	if got := StringInitContentsOfEncoding(URLInitFileURLWithPath(out), EncodingUtf8); got != "hello:arg one" {
		t.Errorf("child wrote %q", got)
	}
	if !panics(func() { NSTaskRun(process) }) {
		t.Error("running a process twice did not trap")
	}
}

func TestProcessKilledAndMissing(t *testing.T) {
	sh := shell(t)
	process := NSTaskInit()
	process.ExecutableURL = &sh
	process.Arguments = new([]string{"-c", "sleep 30"})
	process.StandardOutput = NSFileHandleNullDevice
	process.StandardError = nil
	NSTaskRun(process)
	if !NSTaskIsRunning(process) {
		t.Fatal("a sleeping child is not running")
	}
	if CFKill(NSTaskProcessIdentifier(process), CMacroSIGKILL) != 0 {
		t.Fatal("kill failed")
	}
	NSTaskWaitUntilExit(process)
	if got := NSTaskTerminationStatus(process); got != int32(syscall.SIGKILL) {
		t.Errorf("termination status of a killed child = %d, want %d", got, syscall.SIGKILL)
	}

	missing := NSTaskInit()
	missing.ExecutableURL = new(URLInitFileURLWithPath(filepath.Join(t.TempDir(), "no-such-tool")))
	err := Catch(func() { NSTaskRun(missing) })
	if ns, ok := err.(NSError); !ok || ns.Domain != "NSCocoaErrorDomain" || ns.Code != 4 {
		t.Errorf("running a missing executable threw %#v", err)
	}
	if NSTaskIsRunning(missing) {
		t.Error("a process that failed to start is running")
	}
	if err := Catch(func() { NSTaskRun(NSTaskInit()) }); err == nil {
		t.Error("running without an executable did not throw")
	}
}

func TestFileHandleGaps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	if NSFileHandleOptInitForWritingAtPath(path) != nil || NSFileHandleOptInitForReadingAtPath(path) != nil {
		t.Error("opening a missing file gave a handle")
	}
	err := Catch(func() { NSFileHandleInitForWritingTo(URLInitFileURLWithPath(path)) })
	if ns, ok := err.(NSError); !ok || ns.Code != 4 {
		t.Errorf("FileHandle(forWritingTo:) of a missing file threw %#v", err)
	}
	if !NSFileManagerCreateFileAtPathContents(NSFileManagerDefault, path, new([]byte("0123456789"))) {
		t.Fatal("createFile failed")
	}
	if NSFileManagerCreateFileAtPathContents(NSFileManagerDefault, filepath.Join(dir, "missing", "f"), nil) {
		t.Error("createFile in a missing directory succeeded")
	}

	w := NSFileHandleOptInitForWritingAtPath(path)
	if w == nil {
		t.Fatal("FileHandle(forWritingAtPath:) is nil")
	}
	NSFileHandleWriteContentsOf(*w, []byte("ab"))
	if end := NSFileHandleSeekToEndOfFile(*w); end != 10 {
		t.Errorf("seekToEndOfFile = %d, want 10", end)
	}
	NSFileHandleWriteContentsOf(*w, []byte("XY"))
	NSFileHandleSynchronize(*w)
	NSFileHandleCloseFile(*w)
	NSFileHandleCloseFile(*w)
	if err := Catch(func() { NSFileHandleWriteContentsOf(*w, []byte("z")) }); err == nil {
		t.Error("writing to a closed handle did not throw")
	}

	r := NSFileHandleOptInitForReadingAtPath(path)
	if r == nil {
		t.Fatal("FileHandle(forReadingAtPath:) is nil")
	}
	if end := NSFileHandleSeekToEnd(*r); end != 12 {
		t.Errorf("seekToEnd = %d, want 12", end)
	}
	NSFileHandleSeekToOffset(*r, 8)
	if got := string(NSFileHandleReadDataToEndOfFile(*r)); got != "89XY" {
		t.Errorf("readDataToEndOfFile = %q, want %q", got, "89XY")
	}
	if got := NSFileHandleReadDataToEndOfFile(*r); len(got) != 0 {
		t.Errorf("reading at the end gave %q", got)
	}
	if err := Catch(func() { NSFileHandleWriteContentsOf(*r, []byte("z")) }); err == nil {
		t.Error("writing to a read-only handle did not throw")
	}
	NSFileHandleClose(*r)

	NSFileHandleWriteContentsOf(NSFileHandleNullDevice, []byte("discarded"))
	NSFileHandleSynchronize(NSFileHandleNullDevice)
	NSFileHandleCloseFile(NSFileHandleNullDevice)
	if got := NSFileHandleReadDataToEndOfFile(NSFileHandleNullDevice); len(got) != 0 {
		t.Errorf("reading the null device gave %q", got)
	}
}

func TestFileManagerGaps(t *testing.T) {
	fm := NSFileManagerDefault
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	backups := filepath.Join(root, "a", "backups")
	NSFileManagerCreateDirectoryAtWithIntermediateDirectoriesAttributes(fm, URLInitFileURLWithPath(backups), true, new(map[FileAttributeKey]any{NSFileAttributeKeyCNSFilePosixPermissions: 0o700}))
	NSFileManagerCreateDirectoryAtWithIntermediateDirectories(fm, URLInitFileURLWithPath(backups), true)
	if info, err := os.Stat(backups); err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("created directory: %v, %v", info, err)
	}
	if err := Catch(func() {
		NSFileManagerCreateDirectoryAtWithIntermediateDirectories(fm, URLInitFileURLWithPath(filepath.Join(root, "x", "y")), false)
	}); err == nil {
		t.Error("creating a directory under a missing parent without intermediates did not throw")
	}

	partial, final := filepath.Join(backups, "db.partial"), filepath.Join(backups, "db.sqlite")
	if !NSFileManagerCreateFileAtPathContents(fm, partial, new([]byte("data"))) {
		t.Fatal("createFile failed")
	}
	NSFileManagerSetAttributesOfItemAtPath(fm, map[FileAttributeKey]any{NSFileAttributeKeyCNSFilePosixPermissions: uint16(0o600)}, partial)
	NSFileManagerMoveItemAtTo(fm, URLInitFileURLWithPath(partial), URLInitFileURLWithPath(final))
	if info, err := os.Stat(final); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("moved file: %v, %v", info, err)
	}
	if _, err := os.Stat(partial); err == nil {
		t.Error("the moved file is still at its source")
	}
	if !NSFileManagerCreateFileAtPathContents(fm, partial, nil) {
		t.Fatal("createFile failed")
	}
	moveErr := Catch(func() { NSFileManagerMoveItemAtTo(fm, URLInitFileURLWithPath(partial), URLInitFileURLWithPath(final)) })
	if ns, ok := moveErr.(NSError); !ok || ns.Code != 516 {
		t.Errorf("moving onto an existing file threw %#v", moveErr)
	}
	if err := Catch(func() {
		NSFileManagerSetAttributesOfItemAtPath(fm, map[FileAttributeKey]any{NSFileAttributeKeyCNSFilePosixPermissions: 0o600}, filepath.Join(root, "gone"))
	}); err == nil {
		t.Error("setting attributes of a missing file did not throw")
	}

	NSFileManagerRemoveItemAtPath(fm, filepath.Join(root, "a"))
	if _, err := os.Stat(backups); err == nil {
		t.Error("removeItem(atPath:) left the directory")
	}
	if TryVoid(func() { NSFileManagerRemoveItemAtPath(fm, filepath.Join(root, "a")) }) != nil {
		t.Error("removing a missing item did not throw")
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if got := NSFileManagerCurrentDirectoryPath(fm); got != wd {
		t.Errorf("currentDirectoryPath = %q, want %q", got, wd)
	}
}

func TestDateAndDurationFormatStyles(t *testing.T) {
	previous := time.Local
	time.Local = time.FixedZone("EST", -5*3600)
	defer func() { time.Local = previous }()
	morning := DateInitTimeIntervalSince1970(1645514520) // 2022-02-22 07:22:00 UTC
	evening := DateInitTimeIntervalSince1970(1645572145) // 2022-02-22 23:22:25 UTC
	cases := []struct {
		d     Date
		date  DateStyle
		clock TimeStyle
		want  string
	}{
		{morning, DateStyleAbbreviated, TimeStyleOmitted, "Feb 22, 2022"},
		{morning, DateStyleAbbreviated, TimeStyleShortened, "Feb 22, 2022, 2:22 AM"},
		{evening, DateStyleAbbreviated, TimeStyleShortened, "Feb 22, 2022, 6:22 PM"},
		{evening, DateStyleLong, TimeStyleStandard, "February 22, 2022 at 6:22:25 PM"},
		{evening, DateStyleComplete, TimeStyleComplete, "Tuesday, February 22, 2022 at 6:22:25 PM EST"},
		{morning, DateStyleNumeric, TimeStyleOmitted, "2/22/2022"},
		{morning, DateStyleOmitted, TimeStyleShortened, "2:22 AM"},
		{morning, DateStyleOmitted, TimeStyleOmitted, "2/22/2022, 2:22 AM"},
	}
	for _, c := range cases {
		if got := DateFormattedDateTime(c.d, c.date, c.clock); got != c.want {
			t.Errorf("formatted(date: %d, time: %d) = %q, want %q", c.date, c.clock, got, c.want)
		}
	}

	hoursMinutes := UnitsFormatStyleLocale(UnitsFormatStyleUnitsAllowedWidth(map[any]struct{}{UnitHours: {}, UnitMinutes: {}}, UnitWidthWide), LocaleInitIdentifier("en_US"))
	durations := []struct {
		seconds int64
		style   UnitsFormatStyle
		want    string
	}{
		{3*3600 + 12*60, hoursMinutes, "3 hours, 12 minutes"},
		{3600 + 60, hoursMinutes, "1 hour, 1 minute"},
		{2 * 3600, hoursMinutes, "2 hours"},
		{45 * 60, hoursMinutes, "45 minutes"},
		{0, hoursMinutes, "0 minutes"},
		{90, hoursMinutes, "2 minutes"},
		{150, hoursMinutes, "2 minutes"},
		{1500 * 3600, hoursMinutes, "1,500 hours"},
		{100, UnitsFormatStyleUnitsAllowedWidth(map[DurationUnit]struct{}{UnitMinutes: {}, UnitSeconds: {}}, UnitWidthAbbreviated), "1 min, 40 sec"},
		{8*86400 + 3600 + 5, UnitsFormatStyleUnitsAllowedWidth(map[any]struct{}{UnitWeeks: {}, UnitDays: {}, UnitHours: {}, UnitSeconds: {}}, UnitWidthNarrow), "1w 1d 1h 5s"},
	}
	for _, c := range durations {
		if got := DurationFormatted(DurationSeconds(c.seconds), c.style); got != c.want {
			t.Errorf("%ds formatted = %q, want %q", c.seconds, got, c.want)
		}
	}
	if !panics(func() {
		DurationFormatted(DurationSeconds(1), UnitsFormatStyleLocale(hoursMinutes, LocaleInitIdentifier("fr_FR")))
	}) {
		t.Error("an unverified locale did not panic")
	}
	if got := IntFormatted(1234567); got != "1,234,567" {
		t.Errorf("formatted() = %q", got)
	}
	if got := IntFormatted(int64(-1234), IntegerFormatStyleLocale(IntegerFormatStyleNumber, LocaleInitIdentifier("de_DE"))); got != "-1.234" {
		t.Errorf("formatted(.number.locale(de_DE)) = %q", got)
	}
}

func TestISO8601Styles(t *testing.T) {
	d := DateInitTimeIntervalSince1970(1645514545.6789) // 2022-02-22T07:22:25.6789Z
	storage := ISO8601FormatStyleInitDateSeparatorDateTimeSeparatorTimeSeparatorTimeZoneSeparatorIncludingFractionalSecondsTimeZone(DateISO8601FormatStyleDateSeparatorDash, DateISO8601FormatStyleDateTimeSeparatorStandard, DateISO8601FormatStyleTimeSeparatorColon, DateISO8601FormatStyleTimeZoneSeparatorOmitted, false, TimeZoneGmt())
	if got := ISO8601FormatStyleFormat([]DateISO8601FormatStyle{storage}[0], d); got != "2022-02-22T07:22:25Z" {
		t.Errorf("storage style = %q", got)
	}
	paris := TimeZoneOptInitIdentifier("Europe/Paris")
	if paris == nil {
		t.Skip("no time zone database")
	}
	compact := ISO8601FormatStyleInitDateSeparatorDateTimeSeparatorTimeSeparatorTimeZoneSeparatorIncludingFractionalSecondsTimeZone(DateISO8601FormatStyleDateSeparatorOmitted, DateISO8601FormatStyleDateTimeSeparatorSpace, DateISO8601FormatStyleTimeSeparatorOmitted, DateISO8601FormatStyleTimeZoneSeparatorColon, true, *paris)
	if got := ISO8601FormatStyleFormat(compact, d); got != "20220222 082225.678+01:00" {
		t.Errorf("compact style = %q", got)
	}
	if got := DateFormatted(d, ISO8601FormatStyleDay(ISO8601FormatStyleMonth(ISO8601FormatStyleYear(ISO8601FormatStyleIso8601)))); got != "2022-02-22" {
		t.Errorf(".iso8601.year().month().day() = %q", got)
	}
	if got := DateFormatted(d, ISO8601FormatStyleIso8601); got != "2022-02-22T07:22:25Z" {
		t.Errorf(".iso8601 = %q", got)
	}

	internet := NSISO8601DateFormatterInit()
	fractional := NSISO8601DateFormatterInit()
	fractional.FormatOptions = ISO8601DateFormatOptions{NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithInternetDateTime, NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithFractionalSeconds}
	fullDate := NSISO8601DateFormatterInit()
	fullDate.FormatOptions = ISO8601DateFormatOptions{NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithFullDate}
	if got := NSISO8601DateFormatterStringFrom(internet, d); got != "2022-02-22T07:22:25Z" {
		t.Errorf("internet date-time = %q", got)
	}
	if got := NSISO8601DateFormatterStringFrom(fractional, d); got != "2022-02-22T07:22:25.679Z" {
		t.Errorf("fractional date-time = %q", got)
	}
	if got := NSISO8601DateFormatterStringFrom(fullDate, d); got != "2022-02-22" {
		t.Errorf("full date = %q", got)
	}
	parses := []struct {
		f    *ISO8601DateFormatter
		in   string
		want float64
		ok   bool
	}{
		{internet, "2022-02-22T07:22:25Z", 1645514545, true},
		{internet, "2022-02-22T08:22:25+01:00", 1645514545, true},
		{internet, "2022-02-22T07:22:25.500Z", 0, false},
		{internet, "2022-02-22", 0, false},
		{fractional, "2022-02-22T07:22:25.500Z", 1645514545.5, true},
		{fractional, "2022-02-22T07:22:25Z", 0, false},
		{fullDate, "2022-02-22", 1645488000, true},
		{fullDate, "2022-02-22T07:22:25Z", 0, false},
	}
	for _, c := range parses {
		got := NSISO8601DateFormatterDateFrom(c.f, c.in)
		if (got != nil) != c.ok || got != nil && float64(*got) != c.want {
			t.Errorf("date(from: %q) with options %v = %v, want %v (%v)", c.in, c.f.FormatOptions, got, c.want, c.ok)
		}
	}
}

func TestKnownTimeZonesAndCompare(t *testing.T) {
	ids := TimeZoneKnownTimeZoneIdentifiers
	if !slices.IsSorted(ids) || !slices.Contains(ids, "GMT") || slices.Contains(ids, "UTC") {
		t.Errorf("known identifiers are unsorted, lack GMT, or hold UTC: %d of them", len(ids))
	}
	if len(ids) > 1 {
		if !slices.Contains(ids, "America/New_York") || !slices.Contains(ids, "America/Argentina/Buenos_Aires") {
			t.Error("known identifiers lack common zones")
		}
		for _, id := range ids {
			if id != "GMT" && !strings.Contains(id, "/") || strings.Contains(id, ".") {
				t.Errorf("unexpected identifier %q", id)
			}
		}
		if TimeZoneOptInitIdentifier(ids[0]) == nil {
			t.Errorf("TimeZone(identifier: %q) is nil", ids[0])
		}
	}

	const ci = NSStringCompareOptionsCENSStringCompareOptionsNSCaseInsensitiveSearch
	const literal = NSStringCompareOptionsCENSStringCompareOptionsNSLiteralSearch
	cases := []struct {
		a, b string
		opts NSStringCompareOptions
		want int
	}{
		{"Elric", "elric", ci, NSComparisonResultOrderedSame},
		{"Elric", "elric", 0, NSComparisonResultOrderedAscending},
		{"b", "A", ci, NSComparisonResultOrderedDescending},
		{"café", "café", 0, NSComparisonResultOrderedSame},
		{"café", "café", literal, NSComparisonResultOrderedDescending},
		{"\U0001F600", "�", literal, NSComparisonResultOrderedAscending},
	}
	for _, c := range cases {
		if got := StringCompareOptions(c.a, c.b, c.opts); got != c.want {
			t.Errorf("%q.compare(%q, options: %d) = %d, want %d", c.a, c.b, c.opts, got, c.want)
		}
	}
	if !reflect.DeepEqual(StringRangeOfOptions("Hello", "LL", ci), &Range[int]{2, 4}) {
		t.Error("range(of:options:) no longer takes the options")
	}
}
