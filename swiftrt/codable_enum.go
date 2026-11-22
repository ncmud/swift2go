package swiftrt

import (
	"reflect"
	"strings"
	"sync"
)

// PayloadCase describes one case of a Codable enum with associated values: its name, the coding
// keys of its associated values ("_0", "_1", … for unlabeled ones) and the Go struct emitted for it.
type PayloadCase struct {
	Name   string
	Labels []string
	Type   reflect.Type
}

// CaseOf describes case C of a payload enum.
func CaseOf[C any](name string, labels ...string) PayloadCase {
	return PayloadCase{Name: name, Labels: labels, Type: reflect.TypeFor[C]()}
}

var (
	payloadMu     sync.RWMutex
	payloadEnums  = map[reflect.Type][]PayloadCase{}
	payloadCaseOf = map[reflect.Type]PayloadCase{}
)

// RegisterPayloadEnum records the cases of E, an enum with associated values whose Codable
// conformance Swift synthesizes. A value encodes as {"case": {"label": value, …}}.
func RegisterPayloadEnum[E any](cases ...PayloadCase) {
	payloadMu.Lock()
	defer payloadMu.Unlock()
	payloadEnums[reflect.TypeFor[E]()] = cases
	for _, c := range cases {
		payloadCaseOf[c.Type] = c
	}
}

var caseEnums = map[reflect.Type][]string{}

// RegisterCaseEnum records the case names of E, an enum without associated values or a raw type
// whose Codable conformance Swift synthesizes. A value encodes as {"case": {}}.
func RegisterCaseEnum[E any](names ...string) {
	payloadMu.Lock()
	defer payloadMu.Unlock()
	caseEnums[reflect.TypeFor[E]()] = names
}

func caseNamesFor(t reflect.Type) ([]string, bool) {
	payloadMu.RLock()
	defer payloadMu.RUnlock()
	names, ok := caseEnums[t]
	return names, ok
}

func payloadCaseFor(t reflect.Type) (PayloadCase, bool) {
	payloadMu.RLock()
	defer payloadMu.RUnlock()
	c, ok := payloadCaseOf[t]
	return c, ok
}

func payloadCasesFor(t reflect.Type) ([]PayloadCase, bool) {
	payloadMu.RLock()
	defer payloadMu.RUnlock()
	cs, ok := payloadEnums[t]
	return cs, ok
}

func encodePayloadCase(c PayloadCase, rv reflect.Value) any {
	inner := newEncodedObject(len(c.Labels))
	for i, label := range c.Labels {
		fv := rv.Field(i)
		if fv.Kind() == reflect.Pointer && fv.IsNil() {
			continue
		}
		inner.set(label, EncodeValue(fv.Interface()))
	}
	outer := newEncodedObject(1)
	outer.set(c.Name, inner)
	return outer
}

func decodePayloadEnum(dst reflect.Value, cases []PayloadCase, raw any, path []string) {
	obj, ok := treeObject(raw)
	if !ok || len(obj) != 1 {
		codingFail("typeMismatch: expected one case key at %s", strings.Join(path, "."))
	}
	for _, c := range cases {
		inner, ok := obj[c.Name]
		if !ok {
			continue
		}
		fields, _ := treeObject(inner)
		v := reflect.New(c.Type).Elem()
		for i, label := range c.Labels {
			fv, present := fields[label]
			if !present {
				if v.Field(i).Kind() == reflect.Pointer {
					continue
				}
				codingFail("keyNotFound: %s at %s", label, strings.Join(append(path, c.Name), "."))
			}
			decodeInto(v.Field(i), fv, append(append(path, c.Name), label))
		}
		dst.Set(v)
		return
	}
	codingFail("dataCorrupted: no case of %s at %s", dst.Type(), strings.Join(path, "."))
}
