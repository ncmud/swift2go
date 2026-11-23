package swiftrt

import "sync"

// NSThread is Foundation `Thread`. Go runs Swift's threads as goroutines, so a Thread stands for the
// goroutine that asked for it.
type NSThread struct{ id uint64 }

var (
	threads       sync.Map // goroutine ID → *NSThread
	mainGoroutine = goroutineID()
)

// NSThreadCurrent is Foundation `Thread.current`: the same object every time the same goroutine asks.
func NSThreadCurrent() *NSThread {
	id := goroutineID()
	if t, ok := threads.Load(id); ok {
		return t.(*NSThread) //nolint:forcetypeassert // only *NSThread is stored
	}
	t, _ := threads.LoadOrStore(id, &NSThread{id: id})
	return t.(*NSThread) //nolint:forcetypeassert // only *NSThread is stored
}

// NSThreadIsMainThread is Foundation `Thread.isMainThread`: whether the caller is the goroutine that
// initialized the program.
func NSThreadIsMainThread() bool { return goroutineID() == mainGoroutine }

// OS_dispatch_queueConcurrentPerformIterationsExecute is Dispatch `DispatchQueue.concurrentPerform(iterations:execute:)`:
// it runs work for each index in 0..<iterations at once and returns when every call has.
func OS_dispatch_queueConcurrentPerformIterationsExecute(iterations int, work func(int)) {
	var wg sync.WaitGroup
	for i := range iterations {
		wg.Go(func() { work(i) })
	}
	wg.Wait()
}
