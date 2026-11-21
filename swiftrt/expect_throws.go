package swiftrt

import "reflect"

// runThrowing calls fn, a Go func with no parameters, and returns what it threw.
func runThrowing(fn any) error {
	return Catch(func() { reflect.ValueOf(fn).Call(nil) })
}

// ExpectThrows is `#expect(throws: (any Error).self) { ... }`: fn must throw something.
func ExpectThrows(fn any, fatal bool, msg string) {
	if runThrowing(fn) == nil {
		report(fatal, "expected an error to be thrown %s", msg)
	}
}

// ExpectThrowsType is `#expect(throws: E.self) { ... }`: fn must throw an E.
func ExpectThrowsType[E any](fn any, fatal bool, msg string) {
	err := runThrowing(fn)
	if err == nil {
		report(fatal, "expected an error to be thrown %s", msg)
		return
	}
	if !Is[E](err) {
		report(fatal, "expected %T to be thrown, got %s %s", *new(E), debugStr(err), msg)
	}
}

// ExpectThrowsEqual is `#expect(throws: value) { ... }`: fn must throw an error equal to want.
func ExpectThrowsEqual[E any](want E, fn any, fatal bool, msg string) {
	err := runThrowing(fn)
	if err == nil {
		report(fatal, "expected %s to be thrown %s", debugStr(want), msg)
		return
	}
	if !Is[E](err) || !Equal(MustAs[E](err), want) {
		report(fatal, "expected %s to be thrown, got %s %s", debugStr(want), debugStr(err), msg)
	}
}

// ExpectNoThrow is `#expect(throws: Never.self) { ... }`.
func ExpectNoThrow(fn any, fatal bool, msg string) {
	if err := runThrowing(fn); err != nil {
		report(fatal, "expected no error, got %s %s", debugStr(err), msg)
	}
}
