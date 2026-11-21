package swiftrt

import "sync"

// Lazy is a Swift static or global stored property: initialized once, on first access.
type Lazy[T any] struct {
	Init func() T
	once sync.Once
	v    T
}

// Ptr initializes the value if needed and returns its address, for reads and in-place writes.
func (l *Lazy[T]) Ptr() *T {
	l.once.Do(func() { l.v = l.Init() })
	return &l.v
}
