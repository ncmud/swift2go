package swiftrt

import (
	"os"
	"sync"
	"syscall"
	"testing"
	"time"
)

func pipe(t *testing.T) (r, w int32) {
	t.Helper()
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = pr.Close()
		_ = pw.Close()
	})
	return int32(pr.Fd()), int32(pw.Fd()) //nolint:gosec // descriptors are small
}

func TestCReadWritePollOnKernelDescriptors(t *testing.T) {
	r, w := pipe(t)
	pfd := CPollfdInitFdEventsRevents(r, int16(CMacroPOLLIN), 0)
	if n := CFPoll(&pfd, 1, 20); n != 0 || pfd.Revents != 0 {
		t.Fatalf("poll of an empty pipe = %d, revents %#x", n, pfd.Revents)
	}

	out := []byte("look\r\n")
	if n := CFWrite(w, Some[any](out), len(out)); n != len(out) {
		t.Fatalf("write = %d, errno %d", n, CErrno())
	}
	// The pointer `withUnsafeBufferPointer` passes on is the address of the first element.
	if n := CFWrite(w, Some[any](&out[0]), 4); n != 4 {
		t.Fatalf("write through an element pointer = %d", n)
	}

	fds := []CPollfd{{Fd: w, Events: int16(CMacroPOLLOUT)}, {Fd: r, Events: int16(CMacroPOLLIN)}}
	if n := CFPoll(&fds, len(fds), 1000); n != 2 || fds[0].Revents&int16(CMacroPOLLOUT) == 0 || fds[1].Revents&int16(CMacroPOLLIN) == 0 {
		t.Fatalf("poll = %d, revents %#x %#x", n, fds[0].Revents, fds[1].Revents)
	}

	// A buffer of C characters receives the bytes as characters.
	in := make([]int8, 16)
	if n := CFRead(r, Some[any](in), len(in)); n != 10 || StringInitCString(in) != "look\r\nlook" {
		t.Fatalf("read = %d %q", n, StringInitCString(in))
	}
}

func TestCFailuresSetErrno(t *testing.T) {
	r, w := pipe(t)
	if err := syscall.SetNonblock(int(r), true); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 8)
	if n := CFRead(r, Some[any](buf), len(buf)); n != -1 || CErrno() != CMacroEWOULDBLOCK {
		t.Fatalf("read of an empty non-blocking pipe = %d, errno %d", n, CErrno())
	}
	if rc := CFClose(w); rc != 0 {
		t.Fatalf("close = %d", rc)
	}
	if rc := CFClose(w); rc != -1 || CErrno() != CMacroEBADF {
		t.Fatalf("second close = %d, errno %d", rc, CErrno())
	}
	if n := CFRead(r, Some[any](buf), len(buf)); n != 0 {
		t.Fatalf("read at end of input = %d", n)
	}
}

// fakeDescriptors owns the descriptors at and above base and keeps their bytes in memory.
type fakeDescriptors struct {
	mu     sync.Mutex
	base   int32
	data   map[int32][]byte
	closed []int32
}

func (f *fakeDescriptors) Owns(fd int32) bool { return fd >= f.base }

func (f *fakeDescriptors) Read(fd int32, p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.data[fd]) == 0 {
		return 0, syscall.EAGAIN
	}
	n := copy(p, f.data[fd])
	f.data[fd] = f.data[fd][n:]
	return n, nil
}

func (f *fakeDescriptors) Write(fd int32, p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[fd] = append(f.data[fd], p...)
	return len(p), nil
}

func (f *fakeDescriptors) Close(fd int32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = append(f.closed, fd)
	return nil
}

func (f *fakeDescriptors) Poll(fds []CPollfd, timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	for {
		n := 0
		f.mu.Lock()
		for i := range fds {
			fds[i].Revents = 0
			if !f.Owns(fds[i].Fd) {
				fds[i].Revents = int16(CMacroPOLLNVAL)
			} else if len(f.data[fds[i].Fd]) > 0 {
				fds[i].Revents = fds[i].Events & int16(CMacroPOLLIN)
			}
			if fds[i].Revents != 0 {
				n++
			}
		}
		f.mu.Unlock()
		if n > 0 || !time.Now().Before(deadline) {
			return n
		}
		time.Sleep(time.Millisecond)
	}
}

func TestDescriptorIOServesItsOwnDescriptors(t *testing.T) {
	fake := &fakeDescriptors{base: 1 << 20, data: map[int32][]byte{}}
	SetDescriptorIO(fake)
	t.Cleanup(func() { SetDescriptorIO(nil) })
	const fd = 1<<20 + 3

	buf := make([]byte, 8)
	if n := CFRead(fd, Some[any](buf), len(buf)); n != -1 || CErrno() != CMacroEAGAIN {
		t.Fatalf("read with nothing buffered = %d, errno %d", n, CErrno())
	}
	msg := []byte("north")
	if n := CFWrite(fd, Some[any](msg), len(msg)); n != 5 {
		t.Fatalf("write = %d", n)
	}
	fds := []CPollfd{{Fd: fd, Events: int16(CMacroPOLLIN)}, {Fd: fd + 1, Events: int16(CMacroPOLLIN)}}
	if n := CFPoll(&fds, 2, 0); n != 1 || fds[0].Revents != int16(CMacroPOLLIN) || fds[1].Revents != 0 {
		t.Fatalf("poll = %d, revents %#x %#x", n, fds[0].Revents, fds[1].Revents)
	}
	if n := CFRead(fd, Some[any](buf), len(buf)); n != 5 || string(buf[:n]) != "north" {
		t.Fatalf("read = %d %q", n, buf[:n])
	}
	if rc := CFClose(fd); rc != 0 || len(fake.closed) != 1 || fake.closed[0] != fd {
		t.Fatalf("close = %d, closed %v", rc, fake.closed)
	}

	// Descriptors it does not own still reach the kernel.
	r, w := pipe(t)
	if n := CFWrite(w, Some[any](msg), len(msg)); n != 5 {
		t.Fatalf("kernel write = %d", n)
	}
	if n := CFRead(r, Some[any](buf), len(buf)); n != 5 || string(buf[:n]) != "north" {
		t.Fatalf("kernel read = %d %q", n, buf[:n])
	}
}
