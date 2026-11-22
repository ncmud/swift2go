// Package toy is a hand-written replacement for the Swift module Toy. Its greeting differs from the Swift
// one so the test can tell which ran.
package toy

import "github.com/ncmud/swift2go/cmd/swift2go/testdata/mapping/go/glue"

// Counter is Toy's `Counter`.
type Counter struct{ Count int }

// NewCounter is `Counter()`.
func NewCounter() Counter { return Counter{} }

// Bump is `Counter.bump(by:)`.
func (c *Counter) Bump(step int) { c.Count = glue.GlueAddIntInt(c.Count, step) }

// Greeting is Toy's `greeting(for:)`.
func Greeting(name string) string { return "hi, " + name }
