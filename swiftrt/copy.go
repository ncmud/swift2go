package swiftrt

import (
	"reflect"
	"sync"
	"sync/atomic"
	"unsafe"
)

// SwiftClass marks Go types that came from Swift classes; their pointers are references, not optionals.
type SwiftClass interface{ SwiftClass() }

var swiftClassType = reflect.TypeFor[SwiftClass]()

// Copy gives v Swift value semantics: structs, arrays, dictionaries and optional structs are copied
// deeply, while class references keep their identity.
func Copy[T any](v T) T {
	p := planOf(reflect.TypeFor[T]())
	if p.fix == nil {
		// Go's assignment already copied every part Swift would.
		return v
	}
	return copyWithPlan(v, p)
}

// copyWithPlan runs p on v, a copy already. The fix only writes through the pointer while it runs,
// so v can stay on the stack although the fix is called indirectly.
func copyWithPlan[T any](v T, p *copyPlan) T {
	p.fix(noescape(unsafe.Pointer(&v))) //nolint:gosec // the plan writes through field offsets of T's own layout
	return v
}

// copyPlan unshares the value of one type in place. A nil fix means Go assignment alone copies the
// value as Swift would: it holds no slice, map, interface or non-class pointer.
type copyPlan struct {
	fix func(unsafe.Pointer)
}

var (
	planMu    sync.Mutex
	planCache sync.Map // reflect.Type -> *copyPlan
	// plansByWord holds the finished plans by type word, for planOf. It is replaced, never changed,
	// so readers need no lock.
	plansByWord atomic.Pointer[map[unsafe.Pointer]*copyPlan]
)

// planOf is planFor for a lookup on every copy: it finds the plan by t's type word.
func planOf(t reflect.Type) *copyPlan {
	w := typeWord(t)
	if m := plansByWord.Load(); m != nil {
		if p, ok := (*m)[w]; ok {
			return p
		}
	}
	p := planFor(t)
	planMu.Lock()
	defer planMu.Unlock()
	next := map[unsafe.Pointer]*copyPlan{w: p}
	if m := plansByWord.Load(); m != nil {
		for k, v := range *m {
			next[k] = v
		}
	}
	plansByWord.Store(&next)
	return p
}

// typeWord is the runtime type descriptor behind t: the type word of an interface holding a t.
func typeWord(t reflect.Type) unsafe.Pointer {
	return (*[2]unsafe.Pointer)(unsafe.Pointer(&t))[1] //nolint:gosec // an interface is two words, the second the *rtype
}

// valueAt is reflect.NewAt(t, p).Elem() for ptrWord, the type word of t's pointer type, without
// NewAt's lookup of that pointer type.
func valueAt(ptrWord, p unsafe.Pointer) reflect.Value {
	var i any
	w := (*[2]unsafe.Pointer)(unsafe.Pointer(&i)) //nolint:gosec // an empty interface is its type word and its data word
	w[0], w[1] = ptrWord, p
	return reflect.ValueOf(i).Elem()
}

// sliceHeader is the layout of a Go slice.
type sliceHeader struct {
	data     unsafe.Pointer
	len, cap int
}

// The runtime's allocation and typed copies, as package reflect calls them.

//go:linkname unsafeNew reflect.unsafe_New
func unsafeNew(typ unsafe.Pointer) unsafe.Pointer

//go:linkname unsafeNewArray reflect.unsafe_NewArray
func unsafeNewArray(typ unsafe.Pointer, n int) unsafe.Pointer

//go:linkname typedmemmove reflect.typedmemmove
func typedmemmove(typ, dst, src unsafe.Pointer)

//go:linkname typedslicecopy reflect.typedslicecopy
func typedslicecopy(elemType unsafe.Pointer, dst, src sliceHeader) int

// needsDeepCopy reports whether a value of type t shares storage that Swift would copy: a slice,
// map, interface or non-class pointer anywhere inside it.
func needsDeepCopy(t reflect.Type) bool {
	return planFor(t).fix != nil
}

func planFor(t reflect.Type) *copyPlan {
	if p, ok := planCache.Load(t); ok {
		return p.(*copyPlan)
	}
	planMu.Lock()
	defer planMu.Unlock()
	building := map[reflect.Type]*copyPlan{}
	p := buildPlan(t, building)
	for bt, bp := range building {
		planCache.Store(bt, bp)
	}
	return p
}

// buildPlan computes the plan for t. building holds the plans under construction, so recursive
// types resolve to the same plan; a type reached again while still being built is assumed deep.
func buildPlan(t reflect.Type, building map[reflect.Type]*copyPlan) *copyPlan {
	if p, ok := planCache.Load(t); ok {
		return p.(*copyPlan)
	}
	if p, ok := building[t]; ok {
		return p
	}
	p := &copyPlan{}
	building[t] = p
	switch t.Kind() {
	case reflect.Slice:
		p.fix = sliceFix(t, buildPlan(t.Elem(), building))
	case reflect.Map:
		p.fix = mapFix(t, buildPlan(t.Elem(), building))
	case reflect.Interface:
		p.fix = interfaceFix(t)
	case reflect.Pointer:
		if !t.Implements(swiftClassType) {
			p.fix = pointerFix(t, buildPlan(t.Elem(), building))
		}
	case reflect.Array:
		elem := buildPlan(t.Elem(), building)
		if isDeep(elem, t.Elem(), building) {
			p.fix = arrayFix(t, elem)
		}
	case reflect.Struct:
		p.fix = structFix(t, building)
	}
	return p
}

// isDeep reports whether a plan copies anything. A plan still under construction counts as deep:
// only a container type can lead back to itself, and every container is deep.
func isDeep(p *copyPlan, t reflect.Type, building map[reflect.Type]*copyPlan) bool {
	if p.fix != nil {
		return true
	}
	_, inProgress := building[t]
	if !inProgress {
		return false
	}
	switch t.Kind() {
	case reflect.Slice, reflect.Map, reflect.Interface:
		return true
	case reflect.Pointer:
		return !t.Implements(swiftClassType)
	}
	return false
}

type fieldFix struct {
	offset uintptr
	plan   *copyPlan
}

func structFix(t reflect.Type, building map[reflect.Type]*copyPlan) func(unsafe.Pointer) {
	var fields []fieldFix
	deep := false
	for f := range t.Fields() {
		fp := buildPlan(f.Type, building)
		if !isDeep(fp, f.Type, building) {
			continue
		}
		deep = true
		// A Swift stored property is copied whatever its Go name (`_items` is unexported); other
		// unexported fields are the runtime's own and stay shared.
		if _, swiftProperty := f.Tag.Lookup("swift"); f.IsExported() || swiftProperty {
			fields = append(fields, fieldFix{f.Offset, fp})
		}
	}
	if !deep {
		return nil
	}
	return func(ptr unsafe.Pointer) {
		for _, f := range fields {
			if f.plan.fix != nil {
				f.plan.fix(unsafe.Add(ptr, f.offset))
			}
		}
	}
}

func arrayFix(t reflect.Type, elem *copyPlan) func(unsafe.Pointer) {
	n, size := t.Len(), t.Elem().Size()
	return func(ptr unsafe.Pointer) {
		if elem.fix == nil {
			return
		}
		for i := range n {
			elem.fix(unsafe.Add(ptr, uintptr(i)*size))
		}
	}
}

func sliceFix(t reflect.Type, elem *copyPlan) func(unsafe.Pointer) {
	size := t.Elem().Size()
	elemType := typeWord(t.Elem())
	return func(ptr unsafe.Pointer) {
		h := (*sliceHeader)(ptr)
		if h.data == nil {
			return
		}
		n := h.len
		data := unsafeNewArray(elemType, n)
		typedslicecopy(elemType, sliceHeader{data, n, n}, sliceHeader{h.data, n, n})
		if elem.fix != nil {
			for i := range n {
				elem.fix(unsafe.Add(data, uintptr(i)*size))
			}
		}
		*h = sliceHeader{data, n, n}
	}
}

func mapFix(t reflect.Type, elem *copyPlan) func(unsafe.Pointer) {
	ptrWord := typeWord(reflect.PointerTo(t))
	return func(ptr unsafe.Pointer) {
		if *(*unsafe.Pointer)(ptr) == nil {
			return
		}
		v := valueAt(ptrWord, ptr)
		if v.IsNil() {
			return
		}
		// Iterate a Value holding the map itself: the heap-allocated MapIter must not address ptr.
		src := reflect.ValueOf(v.Interface())
		out := reflect.MakeMapWithSize(t, src.Len())
		key := reflect.New(t.Key()).Elem()
		val := reflect.New(t.Elem()).Elem()
		it := src.MapRange()
		for it.Next() {
			key.SetIterKey(it)
			val.SetIterValue(it)
			if elem.fix != nil {
				elem.fix(val.Addr().UnsafePointer())
			}
			out.SetMapIndex(key, val)
		}
		v.Set(out)
	}
}

func pointerFix(t reflect.Type, elem *copyPlan) func(unsafe.Pointer) {
	elemType := typeWord(t.Elem())
	return func(ptr unsafe.Pointer) {
		old := *(*unsafe.Pointer)(ptr)
		if old == nil {
			return
		}
		np := unsafeNew(elemType)
		typedmemmove(elemType, np, old)
		if elem.fix != nil {
			elem.fix(np)
		}
		*(*unsafe.Pointer)(ptr) = np
	}
}

func interfaceFix(t reflect.Type) func(unsafe.Pointer) {
	ptrWord := typeWord(reflect.PointerTo(t))
	return func(ptr unsafe.Pointer) {
		v := valueAt(ptrWord, ptr)
		if v.IsNil() {
			return
		}
		inner := v.Elem()
		dyn := planOf(inner.Type())
		if dyn.fix == nil {
			return
		}
		nv := reflect.New(inner.Type())
		nv.Elem().Set(inner)
		dyn.fix(nv.UnsafePointer())
		v.Set(nv.Elem())
	}
}
