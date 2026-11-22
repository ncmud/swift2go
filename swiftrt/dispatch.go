package swiftrt

import (
	"math"
	"sync"
	"time"
)

// DispatchTimeIntervalSeconds is Dispatch `DispatchTimeInterval.seconds(_:)`, as the nanoseconds
// that `DispatchTime + DispatchTimeInterval` adds. A negative interval wraps, so adding it
// subtracts.
func DispatchTimeIntervalSeconds[T Integer](n T) DispatchTime {
	return DispatchTime(int64(n) * int64(time.Second))
}

// DispatchTimeIntervalMilliseconds is Dispatch `DispatchTimeInterval.milliseconds(_:)`.
func DispatchTimeIntervalMilliseconds[T Integer](n T) DispatchTime {
	return DispatchTime(int64(n) * int64(time.Millisecond))
}

// DispatchTimeIntervalMicroseconds is Dispatch `DispatchTimeInterval.microseconds(_:)`.
func DispatchTimeIntervalMicroseconds[T Integer](n T) DispatchTime {
	return DispatchTime(int64(n) * int64(time.Microsecond))
}

// DispatchTimeIntervalNanoseconds is Dispatch `DispatchTimeInterval.nanoseconds(_:)`.
func DispatchTimeIntervalNanoseconds[T Integer](n T) DispatchTime { return DispatchTime(int64(n)) }

// DispatchTimeDistantFuture is Dispatch `DispatchTime.distantFuture`: a time that never comes.
const DispatchTimeDistantFuture DispatchTime = math.MaxUint64

// DispatchTimeoutResult is Dispatch `DispatchTimeoutResult`.
type DispatchTimeoutResult int

// DispatchTimeoutResult cases.
const (
	DispatchTimeoutResultSuccess DispatchTimeoutResult = iota
	DispatchTimeoutResultTimedOut
)

// DispatchSemaphore is Dispatch `DispatchSemaphore`: a counting semaphore whose waiters wake in
// the order they began waiting.
type DispatchSemaphore struct {
	mu      sync.Mutex
	value   int
	waiters []chan struct{}
}

// OS_dispatch_semaphore is the Objective-C name the emitter gives DispatchSemaphore.
type OS_dispatch_semaphore = *DispatchSemaphore //nolint:revive // Objective-C name

// OS_dispatch_semaphoreInitValue is Dispatch `DispatchSemaphore(value:)`; a negative value traps.
func OS_dispatch_semaphoreInitValue(value int) *DispatchSemaphore { //nolint:revive // Objective-C name
	if value < 0 {
		Trap("DispatchSemaphore(value:) needs a value of zero or more")
	}
	return &DispatchSemaphore{value: value}
}

// OS_dispatch_semaphoreSignal is `DispatchSemaphore.signal()`: it adds one to the count, waking
// the longest waiter when there is one, and returns non-zero when it woke a waiter.
func OS_dispatch_semaphoreSignal(s *DispatchSemaphore) int { //nolint:revive // Objective-C name
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.waiters) == 0 {
		s.value++
		return 0
	}
	close(s.waiters[0])
	s.waiters = s.waiters[1:]
	return 1
}

// wait takes one from the count, blocking while it is zero for as long as limit allows; a negative
// limit is no limit.
func (s *DispatchSemaphore) wait(limit time.Duration) DispatchTimeoutResult {
	s.mu.Lock()
	if s.value > 0 {
		s.value--
		s.mu.Unlock()
		return DispatchTimeoutResultSuccess
	}
	if limit == 0 {
		s.mu.Unlock()
		return DispatchTimeoutResultTimedOut
	}
	woken := make(chan struct{})
	s.waiters = append(s.waiters, woken)
	s.mu.Unlock()
	if limit < 0 {
		<-woken
		return DispatchTimeoutResultSuccess
	}
	timer := time.NewTimer(limit)
	defer timer.Stop()
	select {
	case <-woken:
		return DispatchTimeoutResultSuccess
	case <-timer.C:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, w := range s.waiters {
		if w == woken {
			s.waiters = append(s.waiters[:i], s.waiters[i+1:]...)
			return DispatchTimeoutResultTimedOut
		}
	}
	// A signal reached this waiter between the timer firing and the lock.
	return DispatchTimeoutResultSuccess
}

// OS_dispatch_semaphoreWait is `DispatchSemaphore.wait()`: it takes one from the count, blocking
// until a signal while the count is zero.
func OS_dispatch_semaphoreWait(s *DispatchSemaphore) { s.wait(-1) } //nolint:revive // Objective-C name

// OS_dispatch_semaphoreWaitTimeout is `DispatchSemaphore.wait(timeout:)`: it waits as wait() does,
// but only until the time, and reports whether it took from the count or timed out.
func OS_dispatch_semaphoreWaitTimeout(s *DispatchSemaphore, deadline DispatchTime) DispatchTimeoutResult { //nolint:revive // Objective-C name
	if deadline == DispatchTimeDistantFuture {
		return s.wait(-1)
	}
	return s.wait(max(time.Duration(int64(deadline-DispatchTimeNow())), 0))
}
