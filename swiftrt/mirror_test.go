package swiftrt

import (
	"errors"
	"testing"
)

type mirrorPoint struct {
	X int `swift:"x"`
	Y int `swift:"y"`
}

type mirrorCase struct{ F0 error }

func TestMirrorChildren(t *testing.T) {
	kids := MirrorChildren(MirrorInitReflecting(mirrorPoint{1, 2}))
	if len(kids) != 2 || *kids[0].F0 != "x" || kids[1].F1 != 2 {
		t.Errorf("struct children = %v", kids)
	}
	inner := errors.New("inner")
	kids = MirrorChildren(MirrorInitReflecting(ErrorOf(mirrorCase{inner})))
	if len(kids) != 1 || kids[0].F1 != inner {
		t.Errorf("payload children = %v", kids)
	}
	if len(MirrorChildren(MirrorInitReflecting(3))) != 0 {
		t.Error("an Int has children")
	}
}
