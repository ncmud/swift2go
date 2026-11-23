package swiftrt

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCSignalSetsAndTheMaskThatDoesNothing(t *testing.T) {
	set := IntInit[uint32]()
	if set != 0 {
		t.Fatalf("a new set holds %#x", set)
	}
	set = 0xffff
	if CFSigemptyset(&set) != 0 || set != 0 {
		t.Fatalf("sigemptyset left %#x", set)
	}
	CFSigaddset(&set, CMacroSIGHUP)
	CFSigaddset(&set, CMacroSIGTERM)
	if want := uint32(1)<<(CMacroSIGHUP-1) | uint32(1)<<(CMacroSIGTERM-1); set != want {
		t.Fatalf("set = %#x, want %#x", set, want)
	}
	if CFSigprocmask(CMacroSIG_SETMASK, &set, nil) != 0 {
		t.Fatal("sigprocmask failed")
	}
}

func TestCProcessCalls(t *testing.T) {
	if n := CFGetdtablesize(); n < 3 {
		t.Fatalf("getdtablesize = %d", n)
	}
	start := time.Now()
	if CFUsleep(20000) != 0 || time.Since(start) < 20*time.Millisecond {
		t.Fatalf("usleep(20000) returned after %v", time.Since(start))
	}

	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	sub := filepath.Join(dir, "lib")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if CFChdir(CStringOf("lib")) != 0 {
		t.Fatalf("chdir failed, errno %d", CErrno())
	}
	if wd, _ := os.Getwd(); wd != sub {
		t.Fatalf("working directory = %s, want %s", wd, sub)
	}
	if CFChdir(CStringOf("absent")) != -1 || StringInitCString(*CFStrerror(CErrno())) != "No such file or directory" {
		t.Fatalf("chdir to a missing directory: errno %d", CErrno())
	}
}

func TestCAbortPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("abort returned")
		}
	}()
	CFAbort()
}

func TestAtomicUpdates(t *testing.T) {
	a := AtomicInit[uint32](0b0101)
	if got := AtomicBitwiseOrOrdering(a, 0b0010, AtomicUpdateOrderingRelaxed); got.F0 != 0b0101 || got.F1 != 0b0111 {
		t.Fatalf("bitwiseOr = %+v", got)
	}
	if old := AtomicExchangeOrdering(a, 0, AtomicUpdateOrderingRelaxed); old != 0b0111 || AtomicLoadOrdering(a, AtomicLoadOrderingRelaxed) != 0 {
		t.Fatalf("exchange returned %#b", old)
	}
	b := AtomicInit[int8](127)
	if got := AtomicWrappingAddOrdering(b, 1, AtomicUpdateOrderingRelaxed); got.F1 != -128 {
		t.Fatalf("wrappingAdd(127, 1) = %d", got.F1)
	}
}

func TestCCharacterPointerReads(t *testing.T) {
	buf := []int8{'n', 'o', 0, 'x'}
	var p any = buf
	if CFStrlen(&p) != 2 || UnsafeMutablePointerPointee(&p) != 'n' || UnsafeMutablePointerSubscriptInt(&p, 1) != 'o' {
		t.Fatal("C character pointer reads disagree with the buffer")
	}
	if CFStrlen(CStringOf("north")) != 5 {
		t.Fatal("strlen of a bridged String")
	}
}
