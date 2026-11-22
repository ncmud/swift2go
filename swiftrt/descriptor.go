package swiftrt

import (
	"errors"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// DescriptorIO serves the C descriptor functions (read, write, close, poll) for descriptors that a
// Go package holds in place of the kernel, such as connections kept as net.Conn. A program
// installs one with SetDescriptorIO; the C functions then pass it the descriptors it owns and
// send the rest to the kernel.
//
// An error it returns sets errno: a syscall.Errno as itself, any other error as EIO.
type DescriptorIO interface {
	// Owns reports whether fd is one of this implementation's descriptors.
	Owns(fd int32) bool
	// Read moves up to len(p) available bytes into p. It returns syscall.EAGAIN when none are
	// available yet, and 0 with no error at the end of input.
	Read(fd int32, p []byte) (int, error)
	// Write takes up to len(p) bytes, or returns syscall.EAGAIN when it can take none.
	Write(fd int32, p []byte) (int, error)
	// Close releases fd.
	Close(fd int32) error
	// Poll sets Revents on every entry and returns how many are non-zero. When none is, it waits
	// up to timeout, or without limit when timeout is negative. An entry it does not own gets
	// POLLNVAL.
	Poll(fds []CPollfd, timeout time.Duration) int
}

// ExecHandoff is a DescriptorIO whose descriptors outlive execv, as kernel descriptors without
// FD_CLOEXEC do. CFExecv calls PrepareExec just before it replaces the process and AbortExec when
// the replacement fails.
type ExecHandoff interface {
	// PrepareExec readies every descriptor to pass to the replacement program and returns the
	// NAME=value entries to add to its environment so that it can take them back.
	PrepareExec() ([]string, error)
	// AbortExec returns the descriptors to service in this process after a failed execv.
	AbortExec()
}

var descriptorIO atomic.Pointer[DescriptorIO]

// SetDescriptorIO installs io for the descriptors it owns, or removes the current one when io is nil.
func SetDescriptorIO(io DescriptorIO) {
	if io == nil {
		descriptorIO.Store(nil)
		return
	}
	descriptorIO.Store(&io)
}

func ownerOf(fd int32) DescriptorIO {
	if p := descriptorIO.Load(); p != nil && (*p).Owns(fd) {
		return *p
	}
	return nil
}

// cErrno is C `errno`. It is one value for the process, where C keeps one per thread: transpiled
// code that reads it does so straight after the failing call.
var cErrno atomic.Int32

// CErrno is C `errno`: the error number the last failing C function left.
func CErrno() int32 { return cErrno.Load() }

// failed records err as errno and returns the C failure result.
func failed[R ~int | ~int32](err error) R {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		errno = syscall.EIO
	}
	cErrno.Store(int32(errno)) //nolint:gosec // error numbers are small
	return -1
}

// C error numbers, with the host platform's values.
const (
	CMacroEINTR       int32 = int32(syscall.EINTR)
	CMacroEAGAIN      int32 = int32(syscall.EAGAIN)
	CMacroEWOULDBLOCK int32 = int32(syscall.EWOULDBLOCK)
	CMacroEINPROGRESS int32 = int32(syscall.EINPROGRESS)
	CMacroEBADF       int32 = int32(syscall.EBADF)
	CMacroEPIPE       int32 = int32(syscall.EPIPE)
	CMacroECONNRESET  int32 = int32(syscall.ECONNRESET)
)

// CPollfd is the C `struct pollfd`.
type CPollfd struct {
	Fd      int32
	Events  int16
	Revents int16
}

// CPollfdInit is `pollfd()`, all zero.
func CPollfdInit() CPollfd { return CPollfd{} }

// CPollfdInitFdEventsRevents is `pollfd(fd:events:revents:)`.
func CPollfdInitFdEventsRevents(fd int32, events, revents int16) CPollfd {
	return CPollfd{Fd: fd, Events: events, Revents: revents}
}

// C `poll(2)` event bits.
const (
	CMacroPOLLIN   int32 = unix.POLLIN
	CMacroPOLLPRI  int32 = unix.POLLPRI
	CMacroPOLLOUT  int32 = unix.POLLOUT
	CMacroPOLLERR  int32 = unix.POLLERR
	CMacroPOLLHUP  int32 = unix.POLLHUP
	CMacroPOLLNVAL int32 = unix.POLLNVAL
)

// cBuffer is the memory behind a C buffer argument: the bytes of the array, or of the C string,
// that the generated code passes as a pointer. The flush function copies bytes written into the
// returned slice back to a buffer of C characters.
func cBuffer(p any, n int) (buf []byte, flush func(int)) {
	switch v := p.(type) {
	case *any:
		if v == nil {
			Trap("unexpectedly found nil while unwrapping an Optional value")
		}
		return cBuffer(*v, n)
	case []byte:
		return v[:min(n, len(v))], func(int) {}
	case *[]byte:
		return cBuffer(*v, n)
	case *[]int8:
		return cBuffer(*v, n)
	case []int8:
		v = v[:min(n, len(v))]
		buf = make([]byte, len(v))
		for i, c := range v {
			buf[i] = byte(c) //nolint:gosec // reinterprets the C character's bits
		}
		return buf, func(got int) {
			for i := range got {
				v[i] = int8(buf[i]) //nolint:gosec // reinterprets the byte's bits
			}
		}
	case string:
		return []byte(v)[:min(n, len(v))], func(int) {}
	case *uint8:
		// A pointer to an array element, as `withUnsafeBufferPointer` hands out its base address.
		return unsafe.Slice(v, n), func(int) {} //nolint:gosec // the C caller vouches for n elements at v, as in C
	}
	panic("swiftrt: not a C buffer pointer")
}

// CFRead is C `read(_:_:_:)`: it moves up to n bytes from the descriptor into the buffer and
// returns how many, 0 at the end of input, or -1 with errno set.
func CFRead(fd int32, buf any, n int) int {
	p, flush := cBuffer(buf, n)
	var got int
	var err error
	if io := ownerOf(fd); io != nil {
		got, err = io.Read(fd, p)
	} else {
		got, err = syscall.Read(int(fd), p)
	}
	if err != nil {
		return failed[int](err)
	}
	flush(got)
	return got
}

// CFWrite is C `write(_:_:_:)`: it writes up to n bytes of the buffer to the descriptor and returns
// how many it took, or -1 with errno set.
func CFWrite(fd int32, buf any, n int) int {
	p, _ := cBuffer(buf, n)
	var put int
	var err error
	if io := ownerOf(fd); io != nil {
		put, err = io.Write(fd, p)
	} else {
		put, err = syscall.Write(int(fd), p)
	}
	if err != nil {
		return failed[int](err)
	}
	return put
}

// CFClose is C `close(_:)`: it releases the descriptor and returns 0, or -1 with errno set.
func CFClose(fd int32) int32 {
	var err error
	if io := ownerOf(fd); io != nil {
		err = io.Close(fd)
	} else {
		err = syscall.Close(int(fd))
	}
	if err != nil {
		return failed[int32](err)
	}
	return 0
}

// pollDescriptors is C `poll(2)` over fds with a timeout in milliseconds, negative for no limit.
// A set that holds any descriptor of the installed DescriptorIO is polled by it alone.
func pollDescriptors(fds []CPollfd, timeoutMs int32) int32 {
	if p := descriptorIO.Load(); p != nil {
		for _, f := range fds {
			if (*p).Owns(f.Fd) {
				timeout := time.Duration(timeoutMs) * time.Millisecond
				return int32((*p).Poll(fds, timeout)) //nolint:gosec // at most len(fds)
			}
		}
	}
	set := make([]unix.PollFd, len(fds))
	for i, f := range fds {
		set[i] = unix.PollFd{Fd: f.Fd, Events: f.Events}
	}
	n, err := unix.Poll(set, int(timeoutMs))
	if err != nil {
		return failed[int32](err)
	}
	for i := range fds {
		fds[i].Revents = set[i].Revents
	}
	return int32(n) //nolint:gosec // at most len(fds)
}

// CFPoll is C `poll(_:_:_:)`: it waits up to timeout milliseconds (without limit when negative) for
// any of the first n descriptors to become ready, sets their Revents, and returns how many are
// ready, or -1 with errno set. fds is `&pollfd` for one descriptor or `&array` for several.
func CFPoll[N Integer](fds any, n N, timeout int32) int32 {
	switch v := fds.(type) {
	case *CPollfd:
		if n == 0 {
			return pollDescriptors(nil, timeout)
		}
		one := []CPollfd{*v}
		r := pollDescriptors(one, timeout)
		*v = one[0]
		return r
	case *[]CPollfd:
		return pollDescriptors((*v)[:min(int(n), len(*v))], timeout)
	case []CPollfd:
		return pollDescriptors(v[:min(int(n), len(v))], timeout)
	}
	panic("swiftrt: not a pollfd pointer")
}
