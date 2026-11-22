package swiftrt

import (
	"sync"
	"testing"
)

func TestRecursiveLockReentersOnItsGoroutineOnly(t *testing.T) {
	l := NSRecursiveLockInit()
	total := 0
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 100 {
				NSRecursiveLockWithLock(l, func() struct{} {
					NSRecursiveLockWithLock(l, func() struct{} {
						total++
						return struct{}{}
					})
					return struct{}{}
				})
			}
		})
	}
	wg.Wait()
	if total != 800 {
		t.Fatalf("total = %d, want 800", total)
	}
}
