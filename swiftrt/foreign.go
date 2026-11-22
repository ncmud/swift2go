package swiftrt

// This file holds the types that generated code passes to hand-written Go packages standing in for Swift
// modules that are not transpiled (swift2go -map).

// KV is one key-value pair of a dictionary literal passed to a hand-written package's
// `<Type>InitDictionaryLiteral`, in source order.
type KV[K, V any] struct {
	Key   K
	Value V
}

// Conformance describes a transpiled type's conformance to protocols of a hand-written package. Generated
// code passes one to the package's `RegisterConformance[T]` from an init function, once for each
// declaration (the type or an extension of it) that states conformances to the package's protocols; a
// package merges the registrations of one type.
type Conformance struct {
	// Type is the Swift name of the conforming type, without its module or enclosing types.
	Type string
	// Protocols names the package's protocols the declaration conforms to, implied ones included.
	Protocols []string
	// Witnesses maps a requirement, spelled as Swift does ("didInsert(_:)", "databaseTableName"), to the
	// type's implementation. A static property is a func() V; a static method or an initializer is its Go
	// function; an instance property or method is a function taking a pointer to the value first (the
	// class pointer itself for a class). Requirements the type leaves to the protocol's default are absent.
	Witnesses map[string]any
}
