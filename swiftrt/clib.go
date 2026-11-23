package swiftrt

import (
	"runtime/debug"
	"strings"
	"time"
)

// CTimeval is the C `struct timeval`.
type CTimeval struct {
	Tv_sec  int64 //nolint:revive // C field name
	Tv_usec int32 //nolint:revive // C field name
}

// CTimevalInit is `timeval()`, all zero.
func CTimevalInit() CTimeval { return CTimeval{} }

// CTimespec is the C `struct timespec`.
type CTimespec struct {
	Tv_sec  int64 //nolint:revive // C field name
	Tv_nsec int64 //nolint:revive // C field name
}

// CTimespecInit is `timespec()`, all zero.
func CTimespecInit() CTimespec { return CTimespec{} }

// CFGettimeofday is C `gettimeofday(&tv, nil)`: the wall clock in seconds and microseconds.
func CFGettimeofday(tv *CTimeval, _ any) int32 {
	now := time.Now()
	tv.Tv_sec = now.Unix()
	tv.Tv_usec = int32(now.Nanosecond() / 1000) //nolint:gosec // below 1e6
	return 0
}

// CEAClockid_t_CLOCK_MONOTONIC is C `CLOCK_MONOTONIC`.
const CEAClockid_t_CLOCK_MONOTONIC int32 = 1 //nolint:revive // C name

// CFClock_gettime is C `clock_gettime`. Only CLOCK_MONOTONIC is modeled, measured from process start.
func CFClock_gettime(_ int32, ts *CTimespec) int32 { //nolint:revive // C name
	d := time.Since(processStart)
	ts.Tv_sec = int64(d / time.Second)
	ts.Tv_nsec = int64(d % time.Second)
	return 0
}

// CStat is the C `struct stat`, with the fields the runtime fills. St_dev and St_mode are wide
// enough for both macOS and Linux, and the timestamps are under both platforms' field names.
type CStat struct {
	St_dev       uint64    //nolint:revive // C field name
	St_ino       uint64    //nolint:revive // C field name
	St_mode      uint32    //nolint:revive // C field name
	St_uid       uint32    //nolint:revive // C field name
	St_gid       uint32    //nolint:revive // C field name
	St_size      int64     //nolint:revive // C field name
	St_blocks    int64     //nolint:revive // C field name
	St_atimespec CTimespec //nolint:revive // C field name
	St_mtimespec CTimespec //nolint:revive // C field name
	St_ctimespec CTimespec //nolint:revive // C field name
	St_atim      CTimespec //nolint:revive // C field name: Linux's St_atimespec
	St_mtim      CTimespec //nolint:revive // C field name: Linux's St_mtimespec
	St_ctim      CTimespec //nolint:revive // C field name: Linux's St_ctimespec
}

// CStatInit is `stat()`, all zero.
func CStatInit() CStat { return CStat{} }

// CFFstat is C `fstat(_:_:)`: it fills st with the status of the open descriptor fd and returns 0,
// or -1 with errno set, leaving st unchanged.
func CFFstat(fd int32, st *CStat) int32 {
	if err := sysFstat(int(fd), st); err != nil {
		return failed[int32](err)
	}
	return 0
}

// CFStat is C `stat(_:_:)`: it fills st with the status of the file at path, following symbolic
// links, and returns 0, or -1 with errno set, leaving st unchanged.
func CFStat(path *any, st *CStat) int32 {
	if err := sysStat(cStringArg(path), st); err != nil {
		return failed[int32](err)
	}
	return 0
}

// ContinuousClockNow is `ContinuousClock.now`, read from the type or from a clock.
func ContinuousClockNow(_ ...ContinuousClock) ContinuousClockInstant { return InstantNow() }

// NSThreadCallStackSymbols is Foundation `Thread.callStackSymbols`: one line per frame of the
// current goroutine's stack.
func NSThreadCallStackSymbols() []string {
	return strings.Split(strings.TrimSpace(string(debug.Stack())), "\n")
}
