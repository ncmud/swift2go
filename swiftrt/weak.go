package swiftrt

import (
	"reflect"
	"runtime"
	"unsafe"
	"weak"
)

// Weak is a Swift `weak` stored property holding a class reference of type T: a pointer, or an
// interface holding one. It does not keep the object alive, and reads nil once the garbage
// collector has freed it.
type Weak[T any] struct {
	ptr weak.Pointer[byte]
	typ reflect.Type // the dynamic pointer type that was stored
	// strong holds an object weak.Make cannot track, one of zero size.
	strong T
	set    bool
}

// Get is the property's value.
func (w *Weak[T]) Get() T {
	var zero T
	if w == nil || !w.set {
		return zero
	}
	if w.typ == nil {
		return w.strong
	}
	p := w.ptr.Value()
	if p == nil {
		return zero
	}
	return reflect.NewAt(w.typ.Elem(), unsafe.Pointer(p)).Interface().(T) //nolint:gosec // p is the stored pointer, typed back as it was stored
}

// Set stores v without keeping it alive.
func (w *Weak[T]) Set(v T) {
	*w = Weak[T]{}
	if isNil(v) {
		return
	}
	w.set = true
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.Type().Elem().Size() == 0 {
		w.strong = v
		return
	}
	w.typ = rv.Type()
	w.ptr = weak.Make((*byte)(rv.UnsafePointer()))
}

// KeepAlive keeps x reachable until this call, as a Swift local stays alive to the end of its scope
// while a weak reference to it can still be read there.
func KeepAlive(x any) { runtime.KeepAlive(x) }
