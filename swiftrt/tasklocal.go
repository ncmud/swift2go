package swiftrt

// TaskLocal stands in for Swift's @TaskLocal. Generated Go is single-goroutine per game loop,
// so a scoped global with save/restore matches the observable behaviour.
type TaskLocal[T any] struct{ v T }

// TaskLocalInitWrappedValue is TaskLocal(wrappedValue:).
func TaskLocalInitWrappedValue[T any](v T) *TaskLocal[T] { return &TaskLocal[T]{v} }

// TaskLocalGet is TaskLocal.get().
func TaskLocalGet[T any](t *TaskLocal[T]) T { return t.v }

// TaskLocalWithValueOperation is TaskLocal.withValue(_:operation:).
func TaskLocalWithValueOperation[T, R any](t *TaskLocal[T], v T, op func() R) R {
	prev := t.v
	t.v = v
	defer func() { t.v = prev }()
	return op()
}
