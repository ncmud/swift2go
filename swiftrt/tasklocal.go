package swiftrt

import (
	"sync"
	"sync/atomic"
)

// TaskLocal stands in for Swift's @TaskLocal. A value bound with withValue is seen only by the
// goroutine that bound it, for the length of the operation, and by the tasks that goroutine starts
// meanwhile with Task { } or a task group's addTask, which copy the bindings in force when they
// start. Task.detached starts with none. Every other goroutine reads the default value.
type TaskLocal[T any] struct {
	def T
	// bound counts the bindings of this value in force on any goroutine, so a read can skip
	// looking up its goroutine's bindings while there are none.
	bound atomic.Int64
}

// TaskLocalInitWrappedValue is TaskLocal(wrappedValue:).
func TaskLocalInitWrappedValue[T any](v T) *TaskLocal[T] { return &TaskLocal[T]{def: v} }

// TaskLocalGet is TaskLocal.get(): the value bound innermost on this goroutine, or the default.
func TaskLocalGet[T any](t *TaskLocal[T]) T {
	if t.bound.Load() == 0 {
		return t.def
	}
	for f := localsOf(goroutineID()); f != nil; f = f.next {
		if f.key == taskLocalKey(t) {
			v, _ := f.value.(T)
			return v
		}
	}
	return t.def
}

// TaskLocalWithValueOperation is TaskLocal.withValue(_:operation:): op runs with v bound on this
// goroutine, and the previous bindings return when it finishes or throws.
func TaskLocalWithValueOperation[T, R any](t *TaskLocal[T], v T, op func() R) R {
	anyTaskLocalBound.Store(true)
	id := goroutineID()
	prev := localsOf(id)
	t.bound.Add(1)
	goroutineLocals.Store(id, &localFrame{key: t, value: v, next: prev})
	defer func() {
		setLocals(id, prev)
		t.bound.Add(-1)
	}()
	return op()
}

func (t *TaskLocal[T]) addBound(n int64) { t.bound.Add(n) }

type taskLocalKey interface{ addBound(n int64) }

// localFrame is one binding of a goroutine's task-local values; the list runs innermost first.
// Frames are never modified once made, so a task can share its creator's list.
type localFrame struct {
	key   taskLocalKey
	value any
	next  *localFrame
}

var goroutineLocals sync.Map

func localsOf(id uint64) *localFrame {
	if f, ok := goroutineLocals.Load(id); ok {
		return f.(*localFrame)
	}
	return nil
}

func setLocals(id uint64, f *localFrame) {
	if f == nil {
		goroutineLocals.Delete(id)
	} else {
		goroutineLocals.Store(id, f)
	}
}

// inheritLocals is the bindings in force on this goroutine, which a task it starts copies.
func inheritLocals() *localFrame {
	if !anyTaskLocalBound.Load() {
		return nil
	}
	return localsOf(goroutineID())
}

// anyTaskLocalBound is set once any value is bound, so that starting a task skips the goroutine
// lookup until then.
var anyTaskLocalBound atomic.Bool

// runWithLocals runs f on this goroutine, a new task's, with the bindings it inherited.
func runWithLocals(f *localFrame, run func()) {
	if f == nil {
		run()
		return
	}
	for b := f; b != nil; b = b.next {
		b.key.addBound(1)
	}
	id := goroutineID()
	goroutineLocals.Store(id, f)
	defer func() {
		goroutineLocals.Delete(id)
		for b := f; b != nil; b = b.next {
			b.key.addBound(-1)
		}
	}()
	run()
}
