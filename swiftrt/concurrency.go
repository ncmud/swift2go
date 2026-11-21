package swiftrt

import (
	"reflect"
	"sync/atomic"
)

// Task is a Swift `Task` handle: an operation running on its own goroutine.
type Task struct {
	done  chan struct{}
	scope *cancelScope
	value any
	err   error
}

// TaskInitOperation is Swift `Task { ... }`, run on a goroutine. op is a `func()` or a `func() T`; an
// error the operation throws is kept for TaskValue, and any other panic crashes the process as a
// Swift trap would.
func TaskInitOperation(op any) *Task {
	t := &Task{done: make(chan struct{}), scope: newCancelScope(nil)}
	go runInScope(t.scope, func() {
		defer close(t.done)
		t.err = Catch(func() {
			if f, ok := op.(func()); ok {
				f()
				t.value = struct{}{}
				return
			}
			out := reflect.ValueOf(op).Call(nil)
			if len(out) == 1 {
				t.value = out[0].Interface()
			} else {
				t.value = struct{}{}
			}
		})
	})
	return t
}

// TaskCancel is Swift `Task.cancel()`: it marks the task and its child tasks cancelled; the operation
// keeps running and observes the cancellation through TaskIsCancelled, TaskCheckCancellation and TaskSleepFor.
func TaskCancel(t any) { t.(*Task).scope.cancel() }

// TaskValue is Swift `await task.value`: it waits for the operation and returns its result,
// rethrowing the error a throwing task ended with.
func TaskValue(t any) any {
	task := t.(*Task)
	<-task.done
	if task.err != nil {
		panic(Throw(task.err))
	}
	return task.value
}

// CancellationError is Swift `CancellationError`.
type CancellationError struct{}

func (CancellationError) Error() string { return "CancellationError()" }

// CancellationErrorInit is Swift `CancellationError()`.
func CancellationErrorInit() CancellationError { return CancellationError{} }

// CheckedContinuation is Swift `CheckedContinuation`.
type CheckedContinuation struct {
	resumed atomic.Bool
	ch      chan struct{}
	value   any
	err     error
}

func (c *CheckedContinuation) resume(value any, err error) {
	if c.resumed.Swap(true) {
		Trap("SWIFT TASK CONTINUATION MISUSE: tried to resume its continuation more than once")
	}
	c.value, c.err = value, err
	close(c.ch)
}

// WithCheckedThrowingContinuation is Swift `withCheckedThrowingContinuation(_:)`: it runs body with a
// continuation and blocks until the continuation is resumed, returning its value or rethrowing its error.
func WithCheckedThrowingContinuation(body func(any)) any {
	c := &CheckedContinuation{ch: make(chan struct{})}
	body(c)
	<-c.ch
	if c.err != nil {
		panic(Throw(c.err))
	}
	return c.value
}

// CheckedContinuationResumeReturning is `CheckedContinuation.resume(returning:)`; a second resume traps.
func CheckedContinuationResumeReturning(c any, value any) {
	c.(*CheckedContinuation).resume(value, nil)
}

// CheckedContinuationResumeThrowing is `CheckedContinuation.resume(throwing:)`; a second resume traps.
func CheckedContinuationResumeThrowing(c any, err any) {
	c.(*CheckedContinuation).resume(nil, Throw(err).Err)
}
