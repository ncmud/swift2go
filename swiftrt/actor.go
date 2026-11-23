package swiftrt

import (
	"sync"
	"sync/atomic"
)

// Actor is the isolation of one Swift actor instance: at most one goroutine runs on it at a time. A
// goroutine is on at most one actor, and it leaves the actor while it is blocked at an `await`, so other
// tasks may run on the actor meanwhile, as Swift's actors are reentrant at suspension points.
type Actor struct{ mu sync.Mutex }

// ActorIsolated is a translated actor: the isolation its instance owns.
type ActorIsolated interface{ SwiftActor() *Actor }

// goroutineActors maps a goroutine to the actor it is on.
var goroutineActors sync.Map

// actorsEntered counts the goroutines that are on an actor, so a goroutine that is on none skips the
// lookup while there are none.
var actorsEntered atomic.Int64

func noActorHop() {}

func currentActor(id uint64) *Actor {
	if actorsEntered.Load() == 0 {
		return nil
	}
	if a, ok := goroutineActors.Load(id); ok {
		return a.(*Actor)
	}
	return nil
}

// ActorEnter runs the rest of the calling function on actor a: the goroutine leaves the actor it is on,
// if another, and waits its turn on a. The returned function, which the caller defers, leaves a and
// returns to the actor the goroutine was on before. On a already, it does nothing.
func ActorEnter(a *Actor) func() {
	id := goroutineID()
	prev := currentActor(id)
	if prev == a {
		return noActorHop
	}
	if prev != nil {
		prev.mu.Unlock()
	} else {
		actorsEntered.Add(1)
	}
	a.mu.Lock()
	goroutineActors.Store(id, a)
	return func() {
		a.mu.Unlock()
		if prev != nil {
			prev.mu.Lock()
			goroutineActors.Store(id, prev)
			return
		}
		goroutineActors.Delete(id)
		actorsEntered.Add(-1)
	}
}

// ActorRead is a read of an actor's isolated stored property from outside its isolation, as
// `await actor.count`: get runs on the actor.
func ActorRead[A ActorIsolated, T any](a A, get func(A) T) T {
	defer ActorEnter(a.SwiftActor())()
	return get(a)
}

// ActorSuspend lets other tasks run on the actor the calling goroutine is on, for a suspension point:
// call it before blocking for a Swift `await`, and call the function it returns once the wait is over to
// return to the actor. Off any actor, both do nothing.
func ActorSuspend() func() {
	if actorsEntered.Load() == 0 {
		return noActorHop
	}
	id := goroutineID()
	a := currentActor(id)
	if a == nil {
		return noActorHop
	}
	goroutineActors.Delete(id)
	actorsEntered.Add(-1)
	a.mu.Unlock()
	return func() {
		a.mu.Lock()
		actorsEntered.Add(1)
		goroutineActors.Store(id, a)
	}
}
