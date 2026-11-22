package swiftrt

import (
	"reflect"
	"sync"
	"unsafe"
)

// SwiftClass marks Go types that came from Swift classes; their pointers are references, not optionals.
type SwiftClass interface{ SwiftClass() }

var swiftClassType = reflect.TypeFor[SwiftClass]()

// Copy gives v Swift value semantics: structs, arrays, dictionaries and optional structs are copied
// deeply, while class references keep their identity.
func Copy[T any](v T) T {
	p := planFor(reflect.TypeFor[T]())
	if p.fix == nil {
		// Go's assignment already copied every part Swift would.
		return v
	}
	return copyWithPlan(v, p)
}

// copyWithPlan runs p on a copy of v. It is separate from Copy so that only values needing a fix
// move to the heap.
func copyWithPlan[T any](v T, p *copyPlan) T {
	p.fix(unsafe.Pointer(&v)) //nolint:gosec // the plan writes through field offsets of T's own layout
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
)

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
		if f.IsExported() {
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
	return func(ptr unsafe.Pointer) {
		v := reflect.NewAt(t, ptr).Elem()
		if v.IsNil() {
			return
		}
		n := v.Len()
		out := reflect.MakeSlice(t, n, n)
		reflect.Copy(out, v)
		if elem.fix != nil && n > 0 {
			base := out.UnsafePointer()
			for i := range n {
				elem.fix(unsafe.Add(base, uintptr(i)*size))
			}
		}
		v.Set(out)
	}
}

func mapFix(t reflect.Type, elem *copyPlan) func(unsafe.Pointer) {
	return func(ptr unsafe.Pointer) {
		v := reflect.NewAt(t, ptr).Elem()
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
	return func(ptr unsafe.Pointer) {
		v := reflect.NewAt(t, ptr).Elem()
		if v.IsNil() {
			return
		}
		np := reflect.New(t.Elem())
		np.Elem().Set(v.Elem())
		if elem.fix != nil {
			elem.fix(np.UnsafePointer())
		}
		v.Set(np)
	}
}

func interfaceFix(t reflect.Type) func(unsafe.Pointer) {
	return func(ptr unsafe.Pointer) {
		v := reflect.NewAt(t, ptr).Elem()
		if v.IsNil() {
			return
		}
		inner := v.Elem()
		dyn := planFor(inner.Type())
		if dyn.fix == nil {
			return
		}
		nv := reflect.New(inner.Type())
		nv.Elem().Set(inner)
		dyn.fix(nv.UnsafePointer())
		v.Set(nv.Elem())
	}
}
