package swiftrt

// Result is Swift `Result<Success, Failure>`: Ok selects Value, otherwise Err holds the failure.
type Result[T any] struct {
	Value T
	Err   any
	Ok    bool
}

// ResultGet is `Result.get()`: the success value, or the failure thrown.
func ResultGet[T any](r Result[T]) T {
	if !r.Ok {
		panic(Throw(r.Err))
	}
	return r.Value
}

// ResultMap is `Result.map`.
func ResultMap[T, U any](r Result[T], f func(T) U) Result[U] {
	if !r.Ok {
		return Result[U]{Err: r.Err}
	}
	return Result[U]{Value: f(r.Value), Ok: true}
}

// ResultFlatMap is `Result.flatMap`.
func ResultFlatMap[T, U any](r Result[T], f func(T) Result[U]) Result[U] {
	if !r.Ok {
		return Result[U]{Err: r.Err}
	}
	return f(r.Value)
}

// ResultMapError is `Result.mapError`.
func ResultMapError[T, E, F any](r Result[T], f func(E) F) Result[T] {
	if r.Ok {
		return r
	}
	return Result[T]{Err: f(r.Err.(E))}
}

// ResultInitCatching is `Result(catching:)`.
func ResultInitCatching[T any](body func() T) (out Result[T]) {
	defer func() {
		if rec := recover(); rec != nil {
			th, ok := rec.(Thrown)
			if !ok {
				panic(rec)
			}
			out = Result[T]{Err: th.Err}
		}
	}()
	return Result[T]{Value: body(), Ok: true}
}
