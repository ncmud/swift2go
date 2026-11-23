//go:build wasip1

package swiftrt

import "syscall"

// WASI preview 1 has no processes, descriptor flags, advisory locks, access checks or poll over
// arbitrary descriptors, so those calls fail with ENOSYS. The constants are wasi-libc's, which a
// Swift package compiled for WASI sees. waitpid's options, which wasi-libc lacks, take musl's values,
// and POLLPRI, which it also lacks, is no bit at all: WASI has no priority data to report.
const (
	sysEWOULDBLOCK = syscall.EAGAIN

	sysWNOHANG   = 1
	sysWUNTRACED = 2

	sysF_GETFD    = 1 //nolint:revive // C name
	sysF_SETFD    = 2 //nolint:revive // C name
	sysF_GETFL    = 3 //nolint:revive // C name
	sysF_SETFL    = 4 //nolint:revive // C name
	sysFD_CLOEXEC = 1 //nolint:revive // C name

	sysF_OK = 0 //nolint:revive // C name
	sysR_OK = 4 //nolint:revive // C name
	sysW_OK = 2 //nolint:revive // C name
	sysX_OK = 1 //nolint:revive // C name

	sysLOCK_SH = 1 //nolint:revive // C name
	sysLOCK_EX = 2 //nolint:revive // C name
	sysLOCK_NB = 4 //nolint:revive // C name
	sysLOCK_UN = 8 //nolint:revive // C name

	sysPOLLIN   = 0x1
	sysPOLLPRI  = 0
	sysPOLLOUT  = 0x2
	sysPOLLERR  = 0x1000
	sysPOLLHUP  = 0x2000
	sysPOLLNVAL = 0x4000
)

func cStat(s *syscall.Stat_t) CStat {
	timespec := func(ns uint64) CTimespec {
		return CTimespec{Tv_sec: int64(ns / 1e9), Tv_nsec: int64(ns % 1e9)} //nolint:gosec // nanoseconds since 1970 fit
	}
	return CStat{
		St_dev:       s.Dev,
		St_ino:       s.Ino,
		St_mode:      uint32(s.Mode), //nolint:gosec // mode bits
		St_uid:       s.Uid,
		St_gid:       s.Gid,
		St_size:      int64(s.Size),               //nolint:gosec // file sizes fit
		St_blocks:    int64((s.Size + 511) / 512), //nolint:gosec // file sizes fit
		St_atimespec: timespec(s.Atime),
		St_mtimespec: timespec(s.Mtime),
		St_ctimespec: timespec(s.Ctime),
		St_atim:      timespec(s.Atime),
		St_mtim:      timespec(s.Mtime),
		St_ctim:      timespec(s.Ctime),
	}
}

func sysFstat(fd int, st *CStat) error {
	var s syscall.Stat_t
	if err := syscall.Fstat(fd, &s); err != nil {
		return err
	}
	*st = cStat(&s)
	return nil
}

func sysStat(path string, st *CStat) error {
	var s syscall.Stat_t
	if err := syscall.Stat(path, &s); err != nil {
		return err
	}
	*st = cStat(&s)
	return nil
}

func sysAccess(string, uint32) error { return syscall.ENOSYS }

func sysFlock(int, int) error { return syscall.ENOSYS }

func sysFcntlInt(uintptr, int, int) (int, error) { return 0, syscall.ENOSYS }

func sysPipe([]int) error { return syscall.ENOSYS }

func sysExec(string, []string, []string) error { return syscall.ENOSYS }

func sysForkExec(string, []string, *syscall.ProcAttr) (int, error) { return 0, syscall.ENOSYS }

func sysSetsockoptInt(int, int, int, int) error { return syscall.ENOSYS }

func sysPoll([]CPollfd, int) (int, error) { return 0, syscall.ENOSYS }
