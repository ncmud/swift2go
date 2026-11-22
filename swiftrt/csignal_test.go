package swiftrt

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestCSignalCallsAFunctionAndReturnsThePreviousDisposition(t *testing.T) {
	t.Cleanup(func() { CFSignal(CMacroSIGUSR2, CMacroSIG_DFL) })
	got := make(chan int32, 4)
	if prev := Flat(CFSignal(CMacroSIGUSR2, func(sig int32) { got <- sig })); !sameHandler(prev, CMacroSIG_DFL) {
		t.Fatal("the first signal() did not return SIG_DFL")
	}
	if err := syscall.Kill(os.Getpid(), syscall.SIGUSR2); err != nil {
		t.Fatal(err)
	}
	select {
	case sig := <-got:
		if sig != CMacroSIGUSR2 {
			t.Fatalf("handler got signal %d", sig)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the handler never ran")
	}

	if prev := Flat(CFSignal(CMacroSIGUSR2, CMacroSIG_IGN)); prev == nil || sameHandler(prev, CMacroSIG_DFL) || sameHandler(prev, CMacroSIG_IGN) {
		t.Fatal("signal() did not return the function it replaced")
	}
	if err := syscall.Kill(os.Getpid(), syscall.SIGUSR2); err != nil {
		t.Fatal(err)
	}
	select {
	case <-got:
		t.Fatal("an ignored signal reached the replaced handler")
	case <-time.After(100 * time.Millisecond):
	}
	if prev := Flat(CFSignal(CMacroSIGUSR2, CMacroSIG_DFL)); !sameHandler(prev, CMacroSIG_IGN) {
		t.Fatal("signal() did not return SIG_IGN")
	}
}

func TestCSignalRefusesSignalsThatCannotBeCaught(t *testing.T) {
	if prev := Flat(CFSignal(int32(syscall.SIGKILL), CMacroSIG_IGN)); !sameHandler(prev, CMacroSIG_ERR) || CErrno() != CMacroEINVAL {
		t.Fatalf("signal(SIGKILL) did not fail with EINVAL; errno %d", CErrno())
	}
}

// TestCSignalIgnoreOutlivesExecv re-runs the test binary as a child that ignores SIGUSR1 and execs a
// shell that sends itself SIGUSR1. An ignored disposition survives execv, so the shell lives to
// print; a handled one would be reset to the default, which ends it.
func TestCSignalIgnoreOutlivesExecv(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("no /bin/sh")
	}
	script := `kill -USR1 $$; printf alive`
	if os.Getenv("SWIFTRT_SIGNAL_EXEC_CHILD") == "ignore" {
		CFSignal(CMacroSIGUSR1, CMacroSIG_IGN)
		CFExecv(CStringOf("/bin/sh"), []*any{CStringOf("sh"), CStringOf("-c"), CStringOf(script), nil})
		t.Fatalf("execv failed: errno %d", CErrno())
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestCSignalIgnoreOutlivesExecv$")
	cmd.Env = append(os.Environ(), "SWIFTRT_SIGNAL_EXEC_CHILD=ignore")
	out, err := cmd.Output()
	if err != nil || string(out) != "alive" {
		t.Fatalf("child printed %q, err %v", out, err)
	}

	// The control: a shell that was not told to ignore SIGUSR1 dies of it.
	out, err = exec.Command("/bin/sh", "-c", script).Output()
	if err == nil || string(out) == "alive" {
		t.Fatalf("a shell outlived its own SIGUSR1: %q", out)
	}
}
