# Actor isolation

A Swift actor lets at most one task run on its isolated state at a time, and it is reentrant: while a
task is suspended at an `await` inside an actor method, other tasks may run on the actor. swift2go runs
every Swift task on a goroutine and models a suspension as the goroutine blocking (`<-ch` in
`WithCheckedContinuation`, `TaskValue`, `TaskSleepFor`, ...). There is no scheduler to hand a job to, so
isolation is a lock that a goroutine holds while it runs on the actor and lets go of while it blocks.

## Representation

- Every actor's struct gets a field `swiftActor swiftrt.Actor` (a mutex) and a method
  `SwiftActor() *swiftrt.Actor`.
- swiftrt records, per goroutine, the one actor the goroutine is on. A task is on at most one executor
  at a time, as in Swift.
- `swiftrt.ActorEnter(a)` puts the goroutine on `a` and returns the function that takes it back off:
  - already on `a`: nothing happens. A call from an actor's method to another of its methods does not
    hop, and the lock is reentrant for the goroutine that holds it.
  - on another actor `b`: it unlocks `b`, locks `a`, and on return unlocks `a` and locks `b` again. A
    call from one actor to another releases the caller, as Swift's hop does, so actors that call each
    other do not deadlock.
- `swiftrt.ActorSuspend()` takes the goroutine off its actor and returns the function that puts it
  back. Every runtime function that blocks for a Swift `await` calls it around the wait: continuations,
  `Task.value`, `Task.sleep`, `Task.yield`, task groups, `AsyncStream` iteration, WebSocket I/O and
  `URLSession`. A hand-written package (`-map`) that blocks in an async function calls it too.
- A blocking call that is synchronous in Swift (`DispatchSemaphore.wait`, a `Mutex`, `Thread.sleep`)
  keeps the actor, as it blocks the actor's thread in Swift.

When no goroutine is on an actor, `ActorSuspend` costs one atomic load, so code that never enters an
actor (the game loop) pays nothing measurable.

## What the emitter isolates

- Methods, and getters of computed properties and subscripts, of an `actor` start with
  `defer swiftrt.ActorEnter(&self.swiftActor)()`. These are the only members another task can call; a
  call from inside the actor is a reentrant no-op.
- Exempt: `nonisolated` members, `static` members, initializers, `deinit`, and `let` constants.
- An escaping closure isolated to the actor (`Task { self.n += 1 }` in an actor method, which the AST
  marks `actor_isolated`) enters the actor first, since it runs on another goroutine.
- A read of an actor's stored `var` through another reference (`await other.count`), or through `self`
  in code not isolated to it, is `swiftrt.ActorRead(other, func(self *A) T { return self.Count })`.

## Not modeled

- `isolated` parameters: the AST records them only in the function's mangled type. No code that
  drives swift2go uses them.
- Global actors, `@MainActor` included: the attribute is dropped, as before. Code that needs the main
  thread uses its own queue.
- Ordering. Swift's default executor runs jobs in priority order; Go's mutex wakes waiters in no
  promised order. Swift promises no order between tasks either.
