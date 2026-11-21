package swiftrt

import (
	"fmt"
	"testing"
)

var currentT *testing.T

// BindT makes t the target of Expect/Require until the returned func runs.
func BindT(t *testing.T) func() {
	prev := currentT
	currentT = t
	return func() {
		currentT = prev
		if r := recover(); r != nil {
			t.Fatalf("swift test panicked: %v", r)
		}
	}
}

func report(fatal bool, format string, args ...any) {
	t := currentT
	if t == nil {
		panic(fmt.Sprintf(format, args...))
	}
	t.Helper()
	if fatal {
		t.Fatalf(format, args...)
	}
	t.Errorf(format, args...)
}

// Expect is swift-testing's #expect / #require on a Bool.
func Expect(ok bool, text string, fatal bool, msg string) {
	if !ok {
		currentT.Helper()
		report(fatal, "expectation failed %s", msg)
	}
}

// ExpectEqual is #expect(a == b), reporting both operands.
func ExpectEqual[T any](a, b T, fatal bool, msg string) {
	if !Equal(a, b) {
		currentT.Helper()
		report(fatal, "expected equal %s\n  lhs: %s\n  rhs: %s", msg, debugStr(a), debugStr(b))
	}
}

// ExpectNotEqual is #expect(a != b).
func ExpectNotEqual[T any](a, b T, fatal bool, msg string) {
	if Equal(a, b) {
		currentT.Helper()
		report(fatal, "expected not equal %s\n  both: %s", msg, debugStr(a))
	}
}

// RequireSome is #require(optional): it unwraps or stops the test.
func RequireSome[T any](p *T, msg string) T {
	if p == nil {
		currentT.Helper()
		report(true, "required value was nil %s", msg)
	}
	return *p
}

// Thrown carries a Swift error through a Go panic.
type Thrown struct{ Err error }

func (t Thrown) Error() string { return "swift error: " + t.Err.Error() }

// Throw wraps a Swift `throw`.
func Throw(err any) Thrown {
	if e, ok := err.(error); ok {
		return Thrown{e}
	}
	return Thrown{fmt.Errorf("%v", err)}
}

// DoCatch runs body; a thrown Swift error goes to handler, which reports whether a catch clause matched.
func DoCatch(body func(), handler func(error) bool) {
	defer func() {
		if r := recover(); r != nil {
			th, ok := r.(Thrown)
			if !ok || !handler(th.Err) {
				panic(r)
			}
		}
	}()
	body()
}

// TryOpt is Swift `try?`.
func TryOpt[T any](f func() T) (out *T) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(Thrown); !ok {
				panic(r)
			}
			out = nil
		}
	}()
	v := f()
	return &v
}

// TryVoid is Swift `try?` on a call returning Void; it reports whether the call completed.
func TryVoid(f func()) (ok *struct{}) {
	defer func() {
		if r := recover(); r != nil {
			if _, isThrown := r.(Thrown); !isThrown {
				panic(r)
			}
			ok = nil
		}
	}()
	f()
	return &struct{}{}
}

// TryFlat is Swift `try?` on an expression that is already optional; the results flatten.
func TryFlat[T any](f func() *T) (out *T) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(Thrown); !ok {
				panic(r)
			}
			out = nil
		}
	}()
	return f()
}

// Catch runs a Swift do body and returns the error it threw, or nil.
func Catch(body func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			th, ok := r.(Thrown)
			if !ok {
				panic(r)
			}
			err = th.Err
		}
	}()
	body()
	return nil
}
