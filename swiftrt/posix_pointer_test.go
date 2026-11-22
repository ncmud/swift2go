package swiftrt

import "testing"

func TestPointerAdvancedSharesMemoryWithItsBase(t *testing.T) {
	buf := UnsafeMutablePointerAllocateCapacity(8)
	UnsafeMutablePointerInitializeRepeatingCount(buf, 'x', 3)
	if got := CFStrlen(buf); got != 3 {
		t.Fatalf("strlen = %d, want 3", got)
	}
	tail := PointerAdvanced(buf, 2)
	UnsafeMutablePointerSetSubscriptInt(tail, 1, 'y')
	if got := StringInitCString(buf); got != "xxxy" {
		t.Errorf("after a write through the advanced pointer the buffer is %q, want xxxy", got)
	}
}
