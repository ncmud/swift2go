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

// runtimePkg is the import path of this package.
var runtimePkg = reflect.TypeFor[copyPlan]().PkgPath()

// Copy gives v Swift value semantics: the result and v can each be changed without the other seeing
// it. Class references keep their identity. An array whose storage nothing can write in place (see
// [Mut]) stays shared, as Swift shares an array's buffer between copies; the first write through
// either copy makes that copy's own storage. Other storage is copied now: dictionaries, sets,
// optionals of value types, protocol values, and arrays that some copy owns and writes in place.
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

// copyPlan unshares the value of one type in place.
type copyPlan struct {
	// fix makes the value at the pointer, a copy already, independent of the value it was copied
	// from. A nil fix means Go assignment alone copies the value as Swift would: it holds no slice,
	// map, interface or non-class pointer.
	fix func(unsafe.Pointer)
	// frozen reports whether every array in the value at the pointer may be shared as it is: each
	// one's length equals its capacity, all the way down. Only a shareable plan has one; nil means
	// the type holds no array.
	frozen func(unsafe.Pointer) bool
	// shareable says every in-place write to the type's storage goes through Mut (an array) or
	// MutPtr (an Optional), and that no one writes an interface's value in place. A map, or a field
	// that a runtime type of its own writes in place, makes a type unshareable, and its storage is
	// always copied.
	shareable bool
	// kids are the plans of the parts, for working out shareable over recursive types.
	kids []*copyPlan
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

// shareableElems reports whether an array of t may share its storage with copies of it (see
// [copyPlan.shareable]).
func shareableElems(t reflect.Type) bool {
	return planOf(t).shareable
}

func planFor(t reflect.Type) *copyPlan {
	if p, ok := planCache.Load(t); ok {
		return p.(*copyPlan)
	}
	planMu.Lock()
	defer planMu.Unlock()
	building := map[reflect.Type]*copyPlan{}
	p := buildPlan(t, building)
	settleShareable(building)
	for bt, bp := range building {
		planCache.Store(bt, bp)
	}
	return p
}

// settleShareable finishes the plans just built: a plan is shareable only if every plan it reaches
// is, which a recursive type can only tell once all of its plans exist. Every new plan starts
// shareable unless its own kind rules that out.
func settleShareable(building map[reflect.Type]*copyPlan) {
	for changed := true; changed; {
		changed = false
		for _, p := range building {
			if !p.shareable {
				continue
			}
			for _, k := range p.kids {
				if !k.shareable {
					p.shareable, changed = false, true
					break
				}
			}
		}
	}
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
	p := &copyPlan{shareable: true}
	building[t] = p
	switch t.Kind() {
	case reflect.Slice:
		elem := buildPlan(t.Elem(), building)
		p.kids = []*copyPlan{elem}
		p.fix = sliceFix(t, elem)
		p.frozen = sliceFrozen(t, elem)
	case reflect.Map:
		p.shareable = false
		p.fix = mapFix(t, buildPlan(t.Elem(), building))
	case reflect.Interface:
		p.fix = interfaceFix(t)
		p.frozen = interfaceFrozen(t)
	case reflect.Pointer:
		if !t.Implements(swiftClassType) {
			elem := buildPlan(t.Elem(), building)
			p.kids = []*copyPlan{elem}
			p.fix = pointerFix(t, p, elem)
			p.frozen = func(ptr unsafe.Pointer) bool {
				target := *(*unsafe.Pointer)(ptr)
				return target == nil || elem.frozen == nil || elem.frozen(target)
			}
		}
	case reflect.Array:
		elem := buildPlan(t.Elem(), building)
		if isDeep(elem, t.Elem(), building) {
			p.kids = []*copyPlan{elem}
			p.fix, p.frozen = arrayFix(t, elem)
		}
	case reflect.Struct:
		structPlan(t, p, building)
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

func structPlan(t reflect.Type, p *copyPlan, building map[reflect.Type]*copyPlan) {
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
		_, swiftProperty := f.Tag.Lookup("swift")
		if !f.IsExported() && !swiftProperty {
			continue
		}
		if !swiftProperty && t.PkgPath() == runtimePkg {
			// A field of the runtime's own types (OrderedSet.Items) is written in place by Go code
			// that does not go through Mut: it is never shared.
			p.shareable = false
			fp = ownedPlan(f.Type, fp)
		}
		fields = append(fields, fieldFix{f.Offset, fp})
		p.kids = append(p.kids, fp)
	}
	if !deep {
		return
	}
	p.fix = func(ptr unsafe.Pointer) {
		for _, f := range fields {
			if f.plan.fix != nil {
				f.plan.fix(unsafe.Add(ptr, f.offset))
			}
		}
	}
	p.frozen = func(ptr unsafe.Pointer) bool {
		for _, f := range fields {
			if f.plan.frozen != nil && !f.plan.frozen(unsafe.Add(ptr, f.offset)) {
				return false
			}
		}
		return true
	}
}

// ownedPlan is a plan for a field of type t, whose usual plan is p, that the runtime writes in place
// without Mut: a slice is always copied, into storage with a spare element so that Mut leaves it be.
func ownedPlan(t reflect.Type, p *copyPlan) *copyPlan {
	if t.Kind() != reflect.Slice || p.fix == nil {
		return p
	}
	elem := p.kids[0]
	elemType := typeWord(t.Elem())
	size := t.Elem().Size()
	return &copyPlan{kids: p.kids, fix: func(ptr unsafe.Pointer) {
		h := (*sliceHeader)(ptr)
		if h.data == nil {
			return
		}
		n := h.len
		data := unsafeNewArray(elemType, n+1)
		typedslicecopy(elemType, sliceHeader{data, n, n}, sliceHeader{h.data, n, n})
		if elem.fix != nil {
			for i := range n {
				elem.fix(unsafe.Add(data, uintptr(i)*size))
			}
		}
		*h = sliceHeader{data, n, n + 1}
	}}
}

func arrayFix(t reflect.Type, elem *copyPlan) (fix func(unsafe.Pointer), frozen func(unsafe.Pointer) bool) {
	n, size := t.Len(), t.Elem().Size()
	fix = func(ptr unsafe.Pointer) {
		if elem.fix == nil {
			return
		}
		for i := range n {
			elem.fix(unsafe.Add(ptr, uintptr(i)*size))
		}
	}
	frozen = func(ptr unsafe.Pointer) bool {
		if elem.frozen == nil {
			return true
		}
		for i := range n {
			if !elem.frozen(unsafe.Add(ptr, uintptr(i)*size)) {
				return false
			}
		}
		return true
	}
	return fix, frozen
}

// sliceFrozen is the frozen test of a slice type: the slice shares its storage, and every element
// can be shared as it is too.
func sliceFrozen(t reflect.Type, elem *copyPlan) func(unsafe.Pointer) bool {
	size := t.Elem().Size()
	return func(ptr unsafe.Pointer) bool {
		h := (*sliceHeader)(ptr)
		if h.len != h.cap {
			return false
		}
		if elem.frozen == nil {
			return true
		}
		for i := range h.len {
			if !elem.frozen(unsafe.Add(h.data, uintptr(i)*size)) {
				return false
			}
		}
		return true
	}
}

// sliceFix keeps a frozen slice of shareable elements, which no copy can write in place without Mut
// copying it first, and copies any other. A copy of shareable elements is frozen itself, so that
// later copies of it share it; any other copy gets a spare element of capacity, which marks it as its
// holder's own to write in place (Mut).
func sliceFix(t reflect.Type, elem *copyPlan) func(unsafe.Pointer) {
	size := t.Elem().Size()
	elemType := typeWord(t.Elem())
	frozen := sliceFrozen(t, elem)
	return func(ptr unsafe.Pointer) {
		h := (*sliceHeader)(ptr)
		if h.data == nil {
			return
		}
		if elem.shareable && frozen(ptr) {
			return
		}
		n := h.len
		if n == 0 {
			*h = sliceHeader{unsafeNewArray(elemType, 0), 0, 0}
			return
		}
		c := n
		if !elem.shareable {
			c = n + 1
		}
		data := unsafeNewArray(elemType, c)
		typedslicecopy(elemType, sliceHeader{data, n, n}, sliceHeader{h.data, n, n})
		if elem.fix != nil {
			for i := range n {
				elem.fix(unsafe.Add(data, uintptr(i)*size))
			}
		}
		*h = sliceHeader{data, n, c}
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

// pointerFix keeps an Optional whose value can be shared as it is, since no one writes through it
// in place (MutPtr), and copies any other.
func pointerFix(t reflect.Type, self, elem *copyPlan) func(unsafe.Pointer) {
	elemType := typeWord(t.Elem())
	return func(ptr unsafe.Pointer) {
		old := *(*unsafe.Pointer)(ptr)
		if old == nil {
			return
		}
		if self.shareable && (elem.frozen == nil || elem.frozen(old)) {
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
		dt := inner.Type()
		if dt.Kind() == reflect.Pointer && !dt.Implements(swiftClassType) {
			// A struct held by pointer, which its mutating methods write through: copy what it
			// points at.
			nv := reflect.New(dt.Elem())
			nv.Elem().Set(inner.Elem())
			if elem := planOf(dt.Elem()); elem.fix != nil {
				elem.fix(nv.UnsafePointer())
			}
			v.Set(nv)
			return
		}
		dyn := planOf(dt)
		if dyn.fix == nil || ifaceShareable(dt, ptr) {
			return
		}
		nv := reflect.New(inner.Type())
		nv.Elem().Set(inner)
		dyn.fix(nv.UnsafePointer())
		v.Set(nv.Elem())
	}
}

// interfaceFrozen is the frozen test of an interface type: the value an interface holds can be
// shared as it is.
func interfaceFrozen(t reflect.Type) func(unsafe.Pointer) bool {
	ptrWord := typeWord(reflect.PointerTo(t))
	return func(ptr unsafe.Pointer) bool {
		if (*[2]unsafe.Pointer)(ptr)[0] == nil {
			return true
		}
		dt := valueAt(ptrWord, ptr).Elem().Type()
		return ifaceShareable(dt, ptr)
	}
}

// ifaceShareable reports whether the interface at ptr, holding a value of type dt, can be shared as it
// is. Go never writes an interface's value in place, except that a struct whose protocol has mutating
// requirements is held by pointer (swiftrt.Ref), and its methods write through that pointer.
func ifaceShareable(dt reflect.Type, ptr unsafe.Pointer) bool {
	if dt.Kind() == reflect.Pointer && !dt.Implements(swiftClassType) {
		return false
	}
	dyn := planOf(dt)
	return dyn.fix == nil || dyn.shareable && (dyn.frozen == nil || dyn.frozen(ifaceValue(ptr, dt)))
}

// ifaceValue points at the value an interface at ptr holds, of dynamic type t: the data word itself
// for a pointer-shaped type, which the interface stores directly, and what the data word points at
// otherwise.
func ifaceValue(ptr unsafe.Pointer, t reflect.Type) unsafe.Pointer {
	w := (*[2]unsafe.Pointer)(ptr)
	if directIface(t) {
		return unsafe.Pointer(&w[1]) //nolint:gosec // the data word holds the value
	}
	return w[1]
}

// directIface reports whether Go stores a value of type t in an interface's data word itself.
func directIface(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Pointer, reflect.Chan, reflect.Map, reflect.Func, reflect.UnsafePointer:
		return true
	case reflect.Struct:
		return t.NumField() == 1 && directIface(t.Field(0).Type)
	case reflect.Array:
		return t.Len() == 1 && directIface(t.Elem())
	}
	return false
}
