package swiftrt

import (
	"fmt"
	"reflect"
	"sync"
	"time"
)

// cancelScope is the cancellation state of one Swift task or task group. Cancelling a scope cancels
// every child scope created under it, as cancelling a Swift task cancels its child tasks.
type cancelScope struct {
	mu        sync.Mutex
	cancelled bool
	done      chan struct{}
	parent    *cancelScope
	children  map[*cancelScope]struct{}
	handlers  map[int]func()
	nextID    int
}

func newCancelScope(parent *cancelScope) *cancelScope {
	s := &cancelScope{done: make(chan struct{}), parent: parent}
	if parent == nil {
		return s
	}
	parent.mu.Lock()
	inherited := parent.cancelled
	if !inherited {
		if parent.children == nil {
			parent.children = map[*cancelScope]struct{}{}
		}
		parent.children[s] = struct{}{}
	}
	parent.mu.Unlock()
	if inherited {
		s.cancel()
	}
	return s
}

func (s *cancelScope) cancel() {
	s.mu.Lock()
	if s.cancelled {
		s.mu.Unlock()
		return
	}
	s.cancelled = true
	close(s.done)
	handlers := s.handlers
	children := s.children
	s.handlers, s.children = nil, nil
	s.mu.Unlock()
	for _, h := range handlers {
		h()
	}
	for c := range children {
		c.cancel()
	}
}

func (s *cancelScope) isCancelled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cancelled
}

// detach drops s from its parent once the task or group it belongs to has finished.
func (s *cancelScope) detach() {
	if p := s.parent; p != nil {
		p.mu.Lock()
		delete(p.children, s)
		p.mu.Unlock()
	}
}

// onCancel registers h to run when s is cancelled, or runs it now if s already is; the returned
// function unregisters it.
func (s *cancelScope) onCancel(h func()) func() {
	s.mu.Lock()
	if s.cancelled {
		s.mu.Unlock()
		h()
		return func() {}
	}
	if s.handlers == nil {
		s.handlers = map[int]func(){}
	}
	id := s.nextID
	s.nextID++
	s.handlers[id] = h
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		delete(s.handlers, id)
		s.mu.Unlock()
	}
}

var goroutineScopes sync.Map

// runInScope runs f on the current goroutine as part of the task whose cancellation state is s.
func runInScope(s *cancelScope, locals *localFrame, f func()) {
	runWithLocals(locals, func() { runScoped(s, f) })
}

func runScoped(s *cancelScope, f func()) {
	id := goroutineID()
	prev, had := goroutineScopes.Load(id)
	goroutineScopes.Store(id, s)
	defer func() {
		if had {
			goroutineScopes.Store(id, prev)
		} else {
			goroutineScopes.Delete(id)
		}
	}()
	f()
}

// currentScope is the cancellation state of the task running on this goroutine, or nil outside any task.
func currentScope() *cancelScope {
	if s, ok := goroutineScopes.Load(goroutineID()); ok {
		return s.(*cancelScope)
	}
	return nil
}

// TaskIsCancelled is Swift `Task.isCancelled`: whether the current task has been cancelled, false outside
// any task. Given a Task value it is that task's `isCancelled`.
func TaskIsCancelled(task ...any) bool {
	if len(task) > 0 {
		// `task.isCancelled` on a Task value.
		return taskHandle(task[0]).scope.isCancelled()
	}
	s := currentScope()
	return s != nil && s.isCancelled()
}

// TaskCheckCancellation is Swift `Task.checkCancellation()`: it throws CancellationError when the
// current task has been cancelled.
func TaskCheckCancellation() {
	if TaskIsCancelled() {
		panic(Throw(CancellationError{}))
	}
}

func goDuration(d any) time.Duration {
	switch v := d.(type) {
	case Duration:
		return time.Duration(v)
	case time.Duration:
		return v
	}
	panic(fmt.Sprintf("swiftrt: %T is not a Swift Duration", d))
}

// TaskSleepFor is Swift `Task.sleep(for:)`: it blocks for the duration, throwing CancellationError if
// the current task is cancelled before or while it sleeps.
func TaskSleepFor(d any) {
	dur := goDuration(d)
	s := currentScope()
	if s == nil {
		defer ActorSuspend()()
		time.Sleep(dur)
		return
	}
	if s.isCancelled() {
		panic(Throw(CancellationError{}))
	}
	timer := time.NewTimer(dur)
	defer timer.Stop()
	resume := ActorSuspend()
	select {
	case <-timer.C:
		resume()
	case <-s.done:
		resume()
		panic(Throw(CancellationError{}))
	}
}

// TaskInitNameExecutorPreferenceOperation is Swift `Task(name:executorPreference:operation:)`. The
// name and executor preference do not change what the operation does, so the task runs as
// TaskInitOperation would run it.
func TaskInitNameExecutorPreferenceOperation(name *string, executor *any, op any) *TaskHandle {
	return TaskInitOperation(op)
}

// WithTaskCancellationHandlerOperationOnCancel is Swift `withTaskCancellationHandler(operation:onCancel:)`:
// it runs op, calling onCancel once if the current task is cancelled while op runs, or immediately
// if the task is already cancelled.
// onCancel is a func() or, when the emitter passes every Void closure of the call as one returning
// struct{}, a func() struct{}.
func WithTaskCancellationHandlerOperationOnCancel[R any](op func() R, onCancel any) R {
	s := currentScope()
	if s == nil {
		return op()
	}
	cancel, ok := onCancel.(func())
	if !ok {
		unit := onCancel.(func() struct{})
		cancel = func() { unit() }
	}
	unregister := s.onCancel(cancel)
	defer unregister()
	return op()
}

// taskGroup is a Swift `TaskGroup` or `ThrowingTaskGroup`, held in the `*any` the group body receives.
type taskGroup struct {
	scope   *cancelScope
	mu      sync.Mutex
	cond    *sync.Cond
	running int
	results []taskGroupResult
}

type taskGroupResult struct {
	value any
	err   error
}

func callTaskOperation(op any) any {
	if f, ok := op.(func()); ok {
		f()
		return struct{}{}
	}
	out := reflect.ValueOf(op).Call(nil)
	if len(out) == 1 {
		return out[0].Interface()
	}
	return struct{}{}
}

func withTaskGroup(body any) any {
	parent := currentScope()
	g := &taskGroup{scope: newCancelScope(parent)}
	g.cond = sync.NewCond(&g.mu)
	defer g.scope.detach()
	var group any = g
	var result any = struct{}{}
	err := Catch(func() {
		out := reflect.ValueOf(body).Call([]reflect.Value{reflect.ValueOf(&group)})
		if len(out) == 1 {
			result = out[0].Interface()
		}
	})
	if err != nil {
		g.scope.cancel()
	}
	resume := ActorSuspend()
	g.mu.Lock()
	for g.running > 0 {
		g.cond.Wait()
	}
	g.mu.Unlock()
	resume()
	if err != nil {
		panic(Throw(err))
	}
	return result
}

func taskGroupAdd(group *any, op any) {
	g := (*group).(*taskGroup)
	child := newCancelScope(g.scope)
	g.mu.Lock()
	g.running++
	g.mu.Unlock()
	go runInScope(child, inheritLocals(), func() {
		defer child.detach()
		var value any
		err := Catch(func() { value = callTaskOperation(op) })
		g.mu.Lock()
		g.results = append(g.results, taskGroupResult{value, err})
		g.running--
		g.cond.Broadcast()
		g.mu.Unlock()
	})
}

func taskGroupNext(group *any) *any {
	g := (*group).(*taskGroup)
	resume := ActorSuspend()
	g.mu.Lock()
	for len(g.results) == 0 && g.running > 0 {
		g.cond.Wait()
	}
	g.mu.Unlock()
	resume()
	g.mu.Lock()
	if len(g.results) == 0 {
		g.mu.Unlock()
		return nil
	}
	r := g.results[0]
	g.results = g.results[1:]
	g.mu.Unlock()
	if r.err != nil {
		panic(Throw(r.err))
	}
	return &r.value
}

// WithThrowingTaskGroupOfBody is Swift `withThrowingTaskGroup(of:body:)`. body is a `func(*any)` or
// `func(*any) R` receiving the group; the call returns body's result after every child task has
// finished. If body throws, the remaining child tasks are cancelled and awaited before the error is
// rethrown; child errors body never reads with ThrowingTaskGroupNext are discarded.
func WithThrowingTaskGroupOfBody(_ Type, body any) any { return withTaskGroup(body) }

// ThrowingTaskGroupAddTaskOperation is `ThrowingTaskGroup.addTask(operation:)`: op runs on its own
// goroutine as a child task of the group.
func ThrowingTaskGroupAddTaskOperation(group *any, op any) { taskGroupAdd(group, op) }

// ThrowingTaskGroupNext is `ThrowingTaskGroup.next()`: it waits for the next child task to finish and
// returns its result, rethrowing the error it threw, or returns nil when no child tasks remain.
func ThrowingTaskGroupNext(group *any) *any { return taskGroupNext(group) }

// ThrowingTaskGroupCancelAll is `ThrowingTaskGroup.cancelAll()`.
func ThrowingTaskGroupCancelAll(group any) { group.(*taskGroup).scope.cancel() }

// WithTaskGroupOfBody is Swift `withTaskGroup(of:body:)`; see WithThrowingTaskGroupOfBody.
func WithTaskGroupOfBody(_ Type, body any) any { return withTaskGroup(body) }

// TaskGroupAddTaskOperation is `TaskGroup.addTask(operation:)`.
func TaskGroupAddTaskOperation(group *any, op any) { taskGroupAdd(group, op) }

// TaskGroupNext is `TaskGroup.next()`; see ThrowingTaskGroupNext.
func TaskGroupNext(group *any) *any { return taskGroupNext(group) }

// TaskGroupCancelAll is `TaskGroup.cancelAll()`.
func TaskGroupCancelAll(group any) { group.(*taskGroup).scope.cancel() }

// OptAnyAs converts a task group's child result, as ThrowingTaskGroupNext and TaskGroupNext return
// it, to the child task's result type T: nil when no child tasks remain.
func OptAnyAs[T any](p *any) *T {
	if p == nil {
		return nil
	}
	v := MustAs[T](*p)
	return &v
}

// OptAnyAsRef is OptAnyAs for a child result type whose Optional is T itself (a reference, or an
// Optional that the runtime does not nest): nil when no child tasks remain or the child returned nil.
func OptAnyAsRef[T any](p *any) T {
	var zero T
	if p == nil || *p == nil {
		return zero
	}
	return MustAs[T](*p)
}

// UnsafeCurrentTask is Swift `UnsafeCurrentTask`: the task running on the goroutine that asked for it.
type UnsafeCurrentTask struct{ scope *cancelScope }

// WithUnsafeCurrentTaskBody is Swift `withUnsafeCurrentTask(body:)`: body gets the current task, or nil
// outside any task.
func WithUnsafeCurrentTaskBody[T any](body func(*any) T) T {
	s := currentScope()
	if s == nil {
		return body(nil)
	}
	var task any = UnsafeCurrentTask{s}
	return body(&task)
}

// UnsafeCurrentTaskCancel is `UnsafeCurrentTask.cancel()`.
func UnsafeCurrentTaskCancel(task any) { task.(UnsafeCurrentTask).scope.cancel() } //nolint:forcetypeassert // generated code passes the task withUnsafeCurrentTask gave

// UnsafeCurrentTaskIsCancelled is `UnsafeCurrentTask.isCancelled`.
func UnsafeCurrentTaskIsCancelled(task any) bool {
	return task.(UnsafeCurrentTask).scope.isCancelled() //nolint:forcetypeassert // generated code passes the task withUnsafeCurrentTask gave
}
