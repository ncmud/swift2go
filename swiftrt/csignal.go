package swiftrt

import (
	"os"
	"os/signal"
	"reflect"
	"sync"
	"syscall"
)

// C `signal(3)` dispositions. CFSignal recognises each by identity, not by calling it.
var (
	CMacroSIG_DFL func(int32) = sigDfl //nolint:revive // C name
	CMacroSIG_IGN func(int32) = sigIgn //nolint:revive // C name
	CMacroSIG_ERR func(int32) = sigErr //nolint:revive // C name
)

func sigDfl(int32) { panic("swiftrt: SIG_DFL called as a handler") }
func sigIgn(int32) { panic("swiftrt: SIG_IGN called as a handler") }
func sigErr(int32) { panic("swiftrt: SIG_ERR called as a handler") }

func sameHandler(a, b func(int32)) bool {
	return a != nil && b != nil && reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
}

// signalHandlers holds the disposition CFSignal last set for each signal, and the delivery of
// each one set to a function.
var signalHandlers struct {
	sync.Mutex
	set      map[int32]func(int32)
	delivery map[int32]chan struct{}
}

// CFSignal is C `signal(_:_:)`: it sets how the process handles sig and returns the disposition it
// replaced, or SIG_ERR with errno set to EINVAL for a signal that cannot be caught.
//
// It works through os/signal. SIG_IGN is signal.Ignore, which ignores sig in the kernel as well, so
// the disposition outlives an execv as C's does. SIG_DFL is signal.Reset: the Go runtime's
// default, which for a signal such as SIGUSR1 is to discard it rather than end the process. A
// function is called on a goroutine of its own for each delivery, never in signal context. Each
// call replaces every os/signal subscription to sig, as one C disposition replaces another.
func CFSignal(sig int32, handler func(int32)) *func(int32) {
	s := syscall.Signal(sig)
	if sig <= 0 || s == syscall.SIGKILL || s == syscall.SIGSTOP || sameHandler(handler, CMacroSIG_ERR) {
		cErrno.Store(CMacroEINVAL)
		return &CMacroSIG_ERR
	}
	h := &signalHandlers
	h.Lock()
	defer h.Unlock()
	if h.set == nil {
		h.set = map[int32]func(int32){}
		h.delivery = map[int32]chan struct{}{}
	}
	prev, ok := h.set[sig]
	if !ok {
		prev = CMacroSIG_DFL
	}
	if done := h.delivery[sig]; done != nil {
		close(done)
		delete(h.delivery, sig)
	}
	switch {
	case handler == nil || sameHandler(handler, CMacroSIG_DFL):
		signal.Reset(s)
		handler = CMacroSIG_DFL
	case sameHandler(handler, CMacroSIG_IGN):
		signal.Ignore(s)
	default:
		signal.Reset(s)
		ch := make(chan os.Signal, 16)
		done := make(chan struct{})
		h.delivery[sig] = done
		signal.Notify(ch, s)
		go func() {
			defer signal.Stop(ch)
			for {
				select {
				case <-ch:
					handler(sig)
				case <-done:
					return
				}
			}
		}()
	}
	h.set[sig] = handler
	return &prev
}
