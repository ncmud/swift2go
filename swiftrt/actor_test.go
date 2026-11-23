package swiftrt

import (
	"sync"
	"testing"
	"time"
)

type testActor struct {
	isolation Actor
	count     int
}

func (a *testActor) SwiftActor() *Actor { return &a.isolation }

func (a *testActor) increment() {
	defer ActorEnter(a.SwiftActor())()
	a.count++
}

func TestActorSerializesMethods(t *testing.T) {
	a := &testActor{}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 1000 {
				a.increment()
			}
		})
	}
	wg.Wait()
	if got := ActorRead(a, func(a *testActor) int { return a.count }); got != 8000 {
		t.Fatalf("count = %d, want 8000", got)
	}
}

func TestActorReentersOnItsOwnGoroutine(t *testing.T) {
	a := &testActor{}
	defer ActorEnter(a.SwiftActor())()
	a.increment()
	if a.count != 1 {
		t.Fatalf("count = %d, want 1", a.count)
	}
}

func TestActorSuspensionLetsOthersRun(t *testing.T) {
	a := &testActor{}
	var c any
	stored := make(chan struct{})
	waited := make(chan struct{})
	go func() {
		defer close(waited)
		defer ActorEnter(a.SwiftActor())()
		WithCheckedContinuation(func(cont any) {
			c = cont
			close(stored)
		})
		a.count++
	}()
	<-stored
	entered := make(chan struct{})
	go func() {
		a.increment()
		close(entered)
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("a goroutine suspended in a continuation kept its actor")
	}
	ActorRead(a, func(*testActor) struct{} {
		CheckedContinuationResume(c)
		return struct{}{}
	})
	<-waited
	if got := ActorRead(a, func(a *testActor) int { return a.count }); got != 2 {
		t.Fatalf("count = %d, want 2", got)
	}
}

func TestActorHopReleasesTheCaller(t *testing.T) {
	a, b := &testActor{}, &testActor{}
	var wg sync.WaitGroup
	call := func(from, to *testActor) {
		defer ActorEnter(from.SwiftActor())()
		from.count++
		to.increment()
		from.count++
	}
	for range 200 {
		wg.Go(func() { call(a, b) })
		wg.Go(func() { call(b, a) })
	}
	wg.Wait()
	if a.count != 600 || b.count != 600 {
		t.Fatalf("counts = %d, %d, want 600, 600", a.count, b.count)
	}
}
