package swiftrt

import (
	"runtime/debug"
	"strings"
	"time"

	"golang.org/x/sys/unix"
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
// enough for both macOS and Linux, and the timestamps carry the macOS field names.
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
}

// CStatInit is `stat()`, all zero.
func CStatInit() CStat { return CStat{} }

func cStat(s *unix.Stat_t) CStat {
	timespec := func(t unix.Timespec) CTimespec {
		return CTimespec{Tv_sec: int64(t.Sec), Tv_nsec: int64(t.Nsec)} //nolint:unconvert // the fields are narrower on 32-bit platforms
	}
	return CStat{
		St_dev:       uint64(s.Dev), //nolint:unconvert // signed and narrower on macOS
		St_ino:       s.Ino,
		St_mode:      uint32(s.Mode), //nolint:unconvert // narrower on macOS
		St_uid:       s.Uid,
		St_gid:       s.Gid,
		St_size:      s.Size,
		St_blocks:    s.Blocks,
		St_atimespec: timespec(s.Atim),
		St_mtimespec: timespec(s.Mtim),
		St_ctimespec: timespec(s.Ctim),
	}
}

// CFFstat is C `fstat(_:_:)`: it fills st with the status of the open descriptor fd and returns 0,
// or -1 with errno set, leaving st unchanged.
func CFFstat(fd int32, st *CStat) int32 {
	var s unix.Stat_t
	if err := unix.Fstat(int(fd), &s); err != nil {
		return failed[int32](err)
	}
	*st = cStat(&s)
	return 0
}

// CFStat is C `stat(_:_:)`: it fills st with the status of the file at path, following symbolic
// links, and returns 0, or -1 with errno set, leaving st unchanged.
func CFStat(path *any, st *CStat) int32 {
	var s unix.Stat_t
	if err := unix.Stat(cStringArg(path), &s); err != nil {
		return failed[int32](err)
	}
	*st = cStat(&s)
	return 0
}

// ContinuousClockNow is `ContinuousClock.now`.
func ContinuousClockNow() ContinuousClockInstant { return InstantNow() }

// NSThreadCallStackSymbols is Foundation `Thread.callStackSymbols`: one line per frame of the
// current goroutine's stack.
func NSThreadCallStackSymbols() []string {
	return strings.Split(strings.TrimSpace(string(debug.Stack())), "\n")
}
