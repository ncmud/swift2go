package emit

import (
	"path/filepath"
	"testing"
)

// TestRefinedProtocolDefaults checks that a struct conforming to a protocol with a settable property gets a
// setter, and that a default from a refining protocol's extension wins over the refined protocol's.
func TestRefinedProtocolDefaults(t *testing.T) {
	out := transpileFixture(t, "protodefaults", "ProtoDefaults", "ProtoDefaultsTests")
	goTest(t, filepath.Join(out, "protodefaults"))
}
