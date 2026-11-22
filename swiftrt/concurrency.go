package swiftrt

import (
	"reflect"
	"sync"
	"sync/atomic"
)

// TaskHandle is a Swift `Task` handle: an operation running on its own goroutine. Generated code
// holds it as `any`; the name Task belongs to Foundation's `Process`, which the emitter calls by
// its Objective-C name NSTask.
type TaskHandle struct {
	done  chan struct{}
	scope *cancelScope
	value any
	err   error
}

// TaskInitOperation is Swift `Task { ... }`, run on a goroutine. op is a `func()` or a `func() T`; an
// error the operation throws is kept for TaskValue, and any other panic crashes the process as a
// Swift trap would.
func TaskInitOperation(op any) *TaskHandle {
	t := &TaskHandle{done: make(chan struct{}), scope: newCancelScope(nil)}
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
func TaskCancel(t any) { taskHandle(t).scope.cancel() }

// taskHandle is the task behind a `Task` value, which the generated code may hold boxed as an
// optional.
func taskHandle(t any) *TaskHandle {
	if p, ok := t.(*any); ok && p != nil {
		t = *p
	}
	return t.(*TaskHandle)
}

// TaskInitExecutorPreferenceOperation is Swift `Task(executorPreference:operation:)`. The executor
// preference does not change what the operation does, so the task runs as TaskInitOperation would
// run it.
func TaskInitExecutorPreferenceOperation(_ *any, op any) *TaskHandle { return TaskInitOperation(op) }

// TaskDetachedOperation is Swift `Task.detached(operation:)`: a task that inherits nothing from the
// one that starts it. Tasks here take no cancellation or priority from their creator either way.
func TaskDetachedOperation(op any) *TaskHandle { return TaskInitOperation(op) }

// TaskValue is Swift `await task.value`: it waits for the operation and returns its result,
// rethrowing the error a throwing task ended with.
func TaskValue(t any) any {
	task := taskHandle(t)
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

// WithCheckedContinuation is Swift `withCheckedContinuation(_:)`: it runs body with a continuation
// and blocks until the continuation is resumed, returning its value.
func WithCheckedContinuation(body func(any)) any {
	c := &CheckedContinuation{ch: make(chan struct{})}
	body(c)
	<-c.ch
	return c.value
}

// CheckedContinuationResume is `CheckedContinuation.resume()` on a continuation of Void; a second
// resume traps.
func CheckedContinuationResume(c any) { c.(*CheckedContinuation).resume(struct{}{}, nil) }

// CheckedContinuationResumeReturning is `CheckedContinuation.resume(returning:)`; a second resume traps.
func CheckedContinuationResumeReturning(c any, value any) {
	c.(*CheckedContinuation).resume(value, nil)
}

// CheckedContinuationResumeThrowing is `CheckedContinuation.resume(throwing:)`; a second resume traps.
func CheckedContinuationResumeThrowing(c any, err any) {
	c.(*CheckedContinuation).resume(nil, Throw(err).Err)
}

// asyncStreamStorage is the shared state of an AsyncStream and its continuation: an unbounded buffer
// of yielded elements and whether the stream has finished. changed is closed and replaced whenever
// either changes.
type asyncStreamStorage[T any] struct {
	mu       sync.Mutex
	buf      []T
	finished bool
	changed  chan struct{}
}

func (s *asyncStreamStorage[T]) signal() {
	close(s.changed)
	s.changed = make(chan struct{})
}

func (s *asyncStreamStorage[T]) finish() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.finished {
		s.finished = true
		s.signal()
	}
}

// AsyncStream is Swift `AsyncStream` with the default unbounded buffering policy.
type AsyncStream[T any] struct{ s *asyncStreamStorage[T] }

// AsyncStreamContinuation is Swift `AsyncStream.Continuation`.
type AsyncStreamContinuation[T any] struct{ s *asyncStreamStorage[T] }

// AsyncStreamIterator is Swift `AsyncStream.Iterator`.
type AsyncStreamIterator[T any] struct{ s *asyncStreamStorage[T] }

// AsyncStreamMakeStreamOf is Swift `AsyncStream.makeStream(of:)`: a stream and the continuation that
// feeds it, buffering every element until it is consumed.
func AsyncStreamMakeStreamOf[T any](_ TypeT[T]) struct {
	F0 AsyncStream[T]
	F1 AsyncStreamContinuation[T]
} {
	s := &asyncStreamStorage[T]{changed: make(chan struct{})}
	return struct {
		F0 AsyncStream[T]
		F1 AsyncStreamContinuation[T]
	}{AsyncStream[T]{s}, AsyncStreamContinuation[T]{s}}
}

// ContinuationYield is Swift `AsyncStream.Continuation.yield(_:)`: it buffers v for the consumer,
// or drops it once the stream has finished.
func ContinuationYield[T any](c AsyncStreamContinuation[T], v T) {
	c.s.mu.Lock()
	defer c.s.mu.Unlock()
	if c.s.finished {
		return
	}
	c.s.buf = append(c.s.buf, v)
	c.s.signal()
}

// ContinuationFinish is Swift `AsyncStream.Continuation.finish()`: the consumer receives the
// elements already buffered and then nil. Later yields are dropped; finishing again does nothing.
func ContinuationFinish[T any](c AsyncStreamContinuation[T]) { c.s.finish() }

// AsyncStreamMakeAsyncIterator is Swift `AsyncStream.makeAsyncIterator()`.
func AsyncStreamMakeAsyncIterator[T any](s AsyncStream[T]) AsyncStreamIterator[T] {
	return AsyncStreamIterator[T](s)
}

// AsyncStreamIteratorNext is Swift `AsyncStream.Iterator.next()`: it waits for the next element and
// returns nil once the stream has finished and its buffer is drained. Cancelling the waiting task
// finishes the stream, as AsyncStream's cancellation handler does.
func AsyncStreamIteratorNext[T any](it *AsyncStreamIterator[T]) *T {
	s := it.s
	var cancelled <-chan struct{}
	if scope := currentScope(); scope != nil {
		if scope.isCancelled() {
			s.finish()
		}
		cancelled = scope.done
	}
	for {
		s.mu.Lock()
		if len(s.buf) > 0 {
			v := s.buf[0]
			var zero T
			s.buf[0] = zero
			s.buf = s.buf[1:]
			s.mu.Unlock()
			return &v
		}
		if s.finished {
			s.mu.Unlock()
			return nil
		}
		changed := s.changed
		s.mu.Unlock()
		select {
		case <-changed:
		case <-cancelled:
			s.finish()
			cancelled = nil
		}
	}
}

// ExecutorJob is Swift `ExecutorJob`: a unit of task work handed to an executor's `enqueue(_:)`.
type ExecutorJob struct{ run func() }

// UnownedJob is Swift `UnownedJob`, an unmanaged reference to an ExecutorJob.
type UnownedJob struct{ run func() }

// UnownedJobInitExecutorJob is Swift `UnownedJob(_:)` from an ExecutorJob.
func UnownedJobInitExecutorJob(job ExecutorJob) UnownedJob { return UnownedJob(job) }

// UnownedTaskExecutor is Swift `UnownedTaskExecutor`: an unmanaged reference to a TaskExecutor.
type UnownedTaskExecutor struct{ Executor any }

// UnownedSerialExecutor is Swift `UnownedSerialExecutor`: an unmanaged reference to a SerialExecutor.
type UnownedSerialExecutor struct{ Executor any }

// TaskExecutorAsUnownedTaskExecutor is Swift `TaskExecutor.asUnownedTaskExecutor()`.
func TaskExecutorAsUnownedTaskExecutor(executor any) UnownedTaskExecutor {
	return UnownedTaskExecutor{executor}
}

// SerialExecutorAsUnownedSerialExecutor is Swift `SerialExecutor.asUnownedSerialExecutor()`.
func SerialExecutorAsUnownedSerialExecutor(executor any) UnownedSerialExecutor {
	return UnownedSerialExecutor{executor}
}

// UnownedJobRunSynchronouslyOn is Swift `UnownedJob.runSynchronously(on:)`: it runs the job on the
// calling goroutine. job is the UnownedJob; the executor it runs on does not change what it does.
func UnownedJobRunSynchronouslyOn(job any, _ any) {
	if j := job.(UnownedJob); j.run != nil {
		j.run()
	}
}
