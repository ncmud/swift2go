//go:build !wasip1

package swiftrt

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// The host C library's constants and the system calls that the C functions wrap, on the platforms
// golang.org/x/sys/unix serves.

const (
	sysEWOULDBLOCK = syscall.EWOULDBLOCK

	sysWNOHANG   = syscall.WNOHANG
	sysWUNTRACED = syscall.WUNTRACED

	sysF_GETFD    = unix.F_GETFD    //nolint:revive // C name
	sysF_SETFD    = unix.F_SETFD    //nolint:revive // C name
	sysF_GETFL    = unix.F_GETFL    //nolint:revive // C name
	sysF_SETFL    = unix.F_SETFL    //nolint:revive // C name
	sysFD_CLOEXEC = unix.FD_CLOEXEC //nolint:revive // C name

	sysF_OK = unix.F_OK //nolint:revive // C name
	sysR_OK = unix.R_OK //nolint:revive // C name
	sysW_OK = unix.W_OK //nolint:revive // C name
	sysX_OK = unix.X_OK //nolint:revive // C name

	sysLOCK_SH = unix.LOCK_SH //nolint:revive // C name
	sysLOCK_EX = unix.LOCK_EX //nolint:revive // C name
	sysLOCK_NB = unix.LOCK_NB //nolint:revive // C name
	sysLOCK_UN = unix.LOCK_UN //nolint:revive // C name

	sysPOLLIN   = unix.POLLIN
	sysPOLLPRI  = unix.POLLPRI
	sysPOLLOUT  = unix.POLLOUT
	sysPOLLERR  = unix.POLLERR
	sysPOLLHUP  = unix.POLLHUP
	sysPOLLNVAL = unix.POLLNVAL
)

func cStat(s *unix.Stat_t) CStat {
	timespec := func(t unix.Timespec) CTimespec {
		return CTimespec{Tv_sec: int64(t.Sec), Tv_nsec: int64(t.Nsec)} //nolint:unconvert // the fields are narrower on 32-bit platforms
	}
	return CStat{
		St_dev:       uint64(s.Dev), //nolint:unconvert // signed and narrower on macOS
		St_ino:       s.Ino,
		St_mode:      uint32(s.Mode), //nolint:unconvert // narrower on macOS
		St_uid:       s.Uid,
		St_gid:       s.Gid,
		St_size:      s.Size,
		St_blocks:    s.Blocks,
		St_atimespec: timespec(s.Atim),
		St_mtimespec: timespec(s.Mtim),
		St_ctimespec: timespec(s.Ctim),
		St_atim:      timespec(s.Atim),
		St_mtim:      timespec(s.Mtim),
		St_ctim:      timespec(s.Ctim),
	}
}

func sysFstat(fd int, st *CStat) error {
	var s unix.Stat_t
	if err := unix.Fstat(fd, &s); err != nil {
		return err
	}
	*st = cStat(&s)
	return nil
}

func sysStat(path string, st *CStat) error {
	var s unix.Stat_t
	if err := unix.Stat(path, &s); err != nil {
		return err
	}
	*st = cStat(&s)
	return nil
}

func sysAccess(path string, mode uint32) error { return unix.Access(path, mode) }

func sysFlock(fd, operation int) error { return unix.Flock(fd, operation) }

func sysFcntlInt(fd uintptr, cmd, arg int) (int, error) { return unix.FcntlInt(fd, cmd, arg) }

func sysPipe(p []int) error { return unix.Pipe(p) }

func sysExec(path string, argv, env []string) error { return syscall.Exec(path, argv, env) }

func sysForkExec(path string, argv []string, attr *syscall.ProcAttr) (int, error) {
	return syscall.ForkExec(path, argv, attr)
}

func sysSetsockoptInt(fd, level, name, value int) error {
	return unix.SetsockoptInt(fd, level, name, value)
}

func sysPoll(fds []CPollfd, timeoutMs int) (int, error) {
	set := make([]unix.PollFd, len(fds))
	for i, f := range fds {
		set[i] = unix.PollFd{Fd: f.Fd, Events: f.Events}
	}
	n, err := unix.Poll(set, timeoutMs)
	if err != nil {
		return 0, err
	}
	for i := range fds {
		fds[i].Revents = set[i].Revents
	}
	return n, nil
}
