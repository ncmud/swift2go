package swiftrt

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
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

// SourceLocationHere is swift-testing's `#_sourceLocation`: the Go file and line of the call that evaluates it.
func SourceLocationHere() any {
	_, file, line, _ := runtime.Caller(1)
	return file + ":" + strconv.Itoa(line)
}

// RequireRef is #require(optional) on an Optional class or existential, which is the reference itself.
func RequireRef[T any](v T, msg string) T {
	if isNil(v) {
		currentT.Helper()
		report(true, "required value was nil %s", msg)
	}
	return v
}

// Thrown carries a Swift error through a Go panic.
type Thrown struct{ Err error }

func (t Thrown) Error() string { return "swift error: " + t.Err.Error() }

// Throw wraps a Swift `throw`. A thrown value whose Go type is not an error, such as an enum
// emitted as an interface, is kept whole, so `catch let e as E`, `as?` and `#expect(throws:)`
// still see it.
func Throw(err any) Thrown {
	if e, ok := err.(error); ok {
		return Thrown{e}
	}
	return Thrown{thrownValue{err}}
}

// ErrorOf is a Swift value stored as `any Error`. A value whose Go type is not an error is carried
// whole, as Throw carries it, so casts and catch patterns still see it.
func ErrorOf(v any) error {
	return Throw(v).Err
}

// thrownValue is the error carrying a thrown Swift value that does not implement Go's error.
type thrownValue struct{ value any }

func (t thrownValue) Error() string { return Str(t.value) }

// SwiftDynamic exposes the thrown value to Is, As and MustAs.
func (t thrownValue) SwiftDynamic() any { return t.value }

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

// TryNil is Swift `try?` on an optional whose Go form is nil-able itself (an interface, closure or
// map): a thrown error gives the zero value, nil.
func TryNil[T any](f func() T) (out T) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(Thrown); !ok {
				panic(r)
			}
			var zero T
			out = zero
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

// IssueRecord is swift-testing `Issue.record(_:)`: a non-fatal test failure.
func IssueRecord(args ...any) {
	parts := make([]string, 0, len(args))
	for _, a := range args {
		if c, ok := a.(*string); ok {
			// A Comment? argument prints its text, not Optional("...").
			if c == nil {
				continue
			}
			a = *c
		}
		if s := Str(a); s != "" {
			parts = append(parts, s)
		}
	}
	report(false, "issue recorded: %s", strings.Join(parts, " "))
}

// CommentInitRawValue is swift-testing's `Comment(rawValue:)`; a Comment is its text.
func CommentInitRawValue(text string) string { return text }

// CommentRawValue is swift-testing's `Comment.rawValue`.
func CommentRawValue(c string) string { return c }

// SourceLocationInitFileIDFilePathLineColumn is swift-testing's `SourceLocation(fileID:filePath:line:column:)`,
// in the `path:line:column` form SourceLocationHere's callers print.
func SourceLocationInitFileIDFilePathLineColumn(fileID, filePath string, line, column int) any {
	return filePath + ":" + strconv.Itoa(line) + ":" + strconv.Itoa(column)
}

// IssueRecordSourceLocation is swift-testing's `Issue.record(_:sourceLocation:)`: a non-fatal test failure
// reported at the given location.
func IssueRecordSourceLocation(comment *string, location any) {
	text := "issue recorded"
	if comment != nil {
		text = *comment
	}
	report(false, "%s: %s", Str(location), text)
}

// Test is swift-testing's `Test`: the test a trait scopes.
type Test struct{ Name string }

// TestCase is swift-testing's `Test.Case`: one run of a test, which a trait scopes when it is not nil.
type TestCase struct{}

// TestScoping is swift-testing's `TestScoping`: a trait that runs each test inside a scope of its own.
type TestScoping interface {
	ProvideScope(test Test, testCase *TestCase, function func())
}

// WithTraits runs the test body inside the scope of each trait that provides one, the first outermost.
func WithTraits(name string, body func(), traits ...any) {
	run := body
	for i := len(traits) - 1; i >= 0; i-- {
		if s, ok := traits[i].(TestScoping); ok {
			inner := run
			run = func() { s.ProvideScope(Test{Name: name}, &TestCase{}, inner) }
		}
	}
	run()
}
