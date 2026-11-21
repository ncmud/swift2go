package swiftrt

import "time"

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
