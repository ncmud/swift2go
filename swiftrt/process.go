package swiftrt

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

// CFExit is C `exit(_:)`: it ends the process with the status code.
func CFExit(code int32) { os.Exit(int(code)) }

// CFFflush is C `fflush(_:)`. Print writes straight to the descriptor and buffers nothing, so there is
// nothing to flush: it returns 0.
func CFFflush(any) int32 { return 0 }

// CFAbort is C `abort()`: it ends the process abnormally. In Go that is an unrecovered panic, which
// prints every goroutine's stack and, under GOTRACEBACK=crash, raises SIGABRT for a core.
func CFAbort() { panic("swiftrt: abort() called") }

// CFPerror is C `perror(_:)`: it prints the message and the text of errno to standard error.
func CFPerror(msg *any) {
	text := strerror(CErrno())
	if msg != nil {
		if s := cStringArg(msg); s != "" {
			text = s + ": " + text
		}
	}
	fmt.Fprintln(os.Stderr, text)
}

// CFUsleep is C `usleep(_:)`: it suspends the calling goroutine for the given microseconds.
func CFUsleep(usec uint32) int32 {
	time.Sleep(time.Duration(usec) * time.Microsecond)
	return 0
}

// CFGetdtablesize is C `getdtablesize()`: the most descriptors the process may hold open, its soft
// RLIMIT_NOFILE.
func CFGetdtablesize() int32 {
	var lim syscall.Rlimit
	if syscall.Getrlimit(syscall.RLIMIT_NOFILE, &lim) != nil {
		return 256
	}
	return int32(min(uint64(lim.Cur), 1<<31-1)) //nolint:gosec,unconvert // clamped; the field is signed on some platforms
}

// CFChdir is C `chdir(_:)`: it changes the working directory and returns 0, or -1 with errno set.
func CFChdir(path *any) int32 {
	if err := syscall.Chdir(cStringArg(path)); err != nil {
		return failed[int32](err)
	}
	return 0
}

// C signal numbers, with the host platform's values.
const (
	CMacroSIGHUP  int32 = int32(syscall.SIGHUP)
	CMacroSIGINT  int32 = int32(syscall.SIGINT)
	CMacroSIGQUIT int32 = int32(syscall.SIGQUIT)
	CMacroSIGPIPE int32 = int32(syscall.SIGPIPE)
	CMacroSIGALRM int32 = int32(syscall.SIGALRM)
	CMacroSIGTERM int32 = int32(syscall.SIGTERM)
	CMacroSIGURG  int32 = int32(syscall.SIGURG)
	CMacroSIGXCPU int32 = int32(syscall.SIGXCPU)
	CMacroSIGUSR1 int32 = int32(syscall.SIGUSR1)
	CMacroSIGUSR2 int32 = int32(syscall.SIGUSR2)
)

// C `sigprocmask(2)` operations. The values are placeholders: see CFSigprocmask.
const (
	CMacroSIG_BLOCK   int32 = 1 //nolint:revive // C name
	CMacroSIG_UNBLOCK int32 = 2 //nolint:revive // C name
	CMacroSIG_SETMASK int32 = 3 //nolint:revive // C name
)

// CFSigemptyset is C `sigemptyset(_:)`: it clears a signal set.
func CFSigemptyset[S Integer](set *S) int32 {
	*set = 0
	return 0
}

// CFSigaddset is C `sigaddset(_:_:)`: it adds a signal to a set.
func CFSigaddset[S Integer](set *S, sig int32) int32 {
	*set |= 1 << (sig - 1)
	return 0
}

// CFSigprocmask is C `sigprocmask(_:_:_:)`. It does nothing and returns 0. The Go runtime owns the
// thread signal masks, and a Go program receives signals through os/signal on a goroutine of its
// own, never in the middle of the caller's work, so there is nothing for a mask to hold back.
func CFSigprocmask[S Integer](_ int32, _ *S, _ *S) int32 { return 0 }

// CommandLineArguments is `CommandLine.arguments`: the program's name and its arguments.
var CommandLineArguments = os.Args

// IntInit is the zero-argument initializer of an integer type, such as `Int32()` or a C typedef of
// one like `sigset_t()`: zero.
func IntInit[T Integer]() T { return 0 }
