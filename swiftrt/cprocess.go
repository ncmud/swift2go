package swiftrt

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"syscall"

	"golang.org/x/sys/unix"
)

// More C error numbers, with the host platform's values.
const (
	CMacroEPERM   int32 = int32(syscall.EPERM)
	CMacroENOENT  int32 = int32(syscall.ENOENT)
	CMacroESRCH   int32 = int32(syscall.ESRCH)
	CMacroENOEXEC int32 = int32(syscall.ENOEXEC)
	CMacroECHILD  int32 = int32(syscall.ECHILD)
	CMacroENOMEM  int32 = int32(syscall.ENOMEM)
	CMacroEACCES  int32 = int32(syscall.EACCES)
	CMacroEEXIST  int32 = int32(syscall.EEXIST)
	CMacroENOTDIR int32 = int32(syscall.ENOTDIR)
	CMacroEISDIR  int32 = int32(syscall.EISDIR)
	CMacroEINVAL  int32 = int32(syscall.EINVAL)
)

// More C signal numbers, `waitpid(2)` options, `fcntl(2)` commands, `access(2)` modes and
// `flock(2)` operations, with the host platform's values.
const (
	CMacroSIGKILL int32 = int32(syscall.SIGKILL)
	CMacroSIGCHLD int32 = int32(syscall.SIGCHLD)
	CMacroSIGSTOP int32 = int32(syscall.SIGSTOP)
	CMacroSIGCONT int32 = int32(syscall.SIGCONT)

	CMacroWNOHANG   int32 = syscall.WNOHANG
	CMacroWUNTRACED int32 = syscall.WUNTRACED

	CMacroF_GETFD    int32 = unix.F_GETFD    //nolint:revive // C name
	CMacroF_SETFD    int32 = unix.F_SETFD    //nolint:revive // C name
	CMacroF_GETFL    int32 = unix.F_GETFL    //nolint:revive // C name
	CMacroF_SETFL    int32 = unix.F_SETFL    //nolint:revive // C name
	CMacroFD_CLOEXEC int32 = unix.FD_CLOEXEC //nolint:revive // C name

	CMacroF_OK int32 = unix.F_OK //nolint:revive // C name
	CMacroR_OK int32 = unix.R_OK //nolint:revive // C name
	CMacroW_OK int32 = unix.W_OK //nolint:revive // C name
	CMacroX_OK int32 = unix.X_OK //nolint:revive // C name

	CMacroLOCK_SH int32 = unix.LOCK_SH //nolint:revive // C name
	CMacroLOCK_EX int32 = unix.LOCK_EX //nolint:revive // C name
	CMacroLOCK_NB int32 = unix.LOCK_NB //nolint:revive // C name
	CMacroLOCK_UN int32 = unix.LOCK_UN //nolint:revive // C name
)

// CFStrdup is C `strdup(_:)`: a new C string holding a copy of s.
func CFStrdup(s *any) *any { return cStringResult(cStringArg(s)) }

// CFFree is C `free(_:)`. It does nothing: the garbage collector reclaims the memory.
func CFFree(*any) {}

// CFRealpath is C `realpath(_:_:)` with a nil buffer: a new C string holding the absolute path
// of the file with every symbolic link, `.` and `..` resolved, or nil with errno set when the
// file does not exist or cannot be reached. A caller-supplied buffer is not supported and panics.
func CFRealpath(path *any, resolved *any) *any {
	if resolved != nil {
		panic("swiftrt: realpath into a caller's buffer is not supported")
	}
	abs, err := filepath.Abs(cStringArg(path))
	if err == nil {
		abs, err = filepath.EvalSymlinks(abs)
	}
	if err != nil {
		var perr *os.PathError
		if errors.As(err, &perr) {
			err = perr.Err
		}
		failed[int32](err)
		return nil
	}
	return cStringResult(abs)
}

// CFAccess is C `access(_:_:)`: 0 when the caller may use the file in the ways mode asks, or -1
// with errno set.
func CFAccess(path *any, mode int32) int32 {
	if err := unix.Access(cStringArg(path), uint32(mode)); err != nil {
		return failed[int32](err)
	}
	return 0
}

// CFFlock is C `flock(_:_:)`: it applies or removes an advisory lock on the open file and
// returns 0, or -1 with errno set.
func CFFlock(fd, operation int32) int32 {
	if err := unix.Flock(int(fd), int(operation)); err != nil {
		return failed[int32](err)
	}
	return 0
}

// FcntlIntIntInt is C `fcntl(_:_:_:)` with an integer argument: the command's result, or -1 with
// errno set.
func FcntlIntIntInt(fd, cmd, arg int32) int32 {
	r, err := unix.FcntlInt(uintptr(fd), int(cmd), int(arg))
	if err != nil {
		return failed[int32](err)
	}
	return int32(r)
}

// CFPipe is C `pipe(_:)`: it puts the read and write descriptors of a new pipe in the first two
// elements of fds and returns 0, or -1 with errno set.
func CFPipe(fds *[]int32) int32 {
	if len(*fds) < 2 {
		Trap("pipe needs an array of two descriptors")
	}
	var p [2]int
	if err := unix.Pipe(p[:]); err != nil {
		return failed[int32](err)
	}
	(*fds)[0], (*fds)[1] = int32(p[0]), int32(p[1])
	return 0
}

// CFKill is C `kill(_:_:)`: it sends the signal to the process and returns 0, or -1 with errno set.
func CFKill(pid, sig int32) int32 {
	if err := syscall.Kill(int(pid), syscall.Signal(sig)); err != nil {
		return failed[int32](err)
	}
	return 0
}

// CFWaitpid is C `waitpid(_:_:_:)`: it waits for a child to change state, stores its wait status,
// and returns its process ID, 0 when WNOHANG found none ready, or -1 with errno set.
func CFWaitpid(pid int32, status *int32, options int32) int32 {
	var ws syscall.WaitStatus
	got, err := syscall.Wait4(int(pid), &ws, int(options), nil)
	if err != nil {
		return failed[int32](err)
	}
	if got != 0 && status != nil {
		*status = int32(ws)
	}
	return int32(got)
}

// argvSlice is the array behind a C `char **`: a `**any` or `*any` leading to an `any` that holds
// the `[]*any` of C strings, or that slice itself.
func argvSlice(p any) []*any {
	for {
		switch v := p.(type) {
		case []*any:
			return v
		case *[]*any:
			return *v
		case **any:
			if v == nil || *v == nil {
				Trap("unexpectedly found nil while unwrapping an Optional value")
			}
			p = **v
		case *any:
			if v == nil {
				Trap("unexpectedly found nil while unwrapping an Optional value")
			}
			p = *v
		default:
			panic(fmt.Sprintf("swiftrt: %T is not a pointer to C strings", p))
		}
	}
}

// cStringList reads a C array of strings up to its NULL terminator, or its end.
func cStringList(p any) []string {
	var out []string
	for _, s := range argvSlice(p) {
		if s == nil {
			break
		}
		out = append(out, StringInitCString(s))
	}
	return out
}

// UnsafeMutablePointerPointerSubscriptInt is Swift `p[i]` on an
// `UnsafeMutablePointer<UnsafeMutablePointer<CChar>?>` such as argv: the C string at index i, or
// nil for a NULL element.
func UnsafeMutablePointerPointerSubscriptInt(p any, i int) *any { return argvSlice(p)[i] }

// UnsafeMutablePointerPointerSetSubscriptInt is Swift `p[i] = v` on an
// `UnsafeMutablePointer<UnsafeMutablePointer<CChar>?>`.
func UnsafeMutablePointerPointerSetSubscriptInt(p any, i int, v *any) { argvSlice(p)[i] = v }

func commandLineArgv() **any {
	list := make([]*any, 0, len(os.Args)+1)
	for _, arg := range os.Args {
		list = append(list, cStringResult(arg))
	}
	var held any = append(list, nil)
	inner := &held
	return &inner
}

// CommandLineUnsafeArgv is Swift `CommandLine.unsafeArgv`: the program's arguments as a
// NULL-terminated array of C strings. Writes through it persist.
var CommandLineUnsafeArgv = commandLineArgv()

// CommandLineArgc is Swift `CommandLine.argc`.
var CommandLineArgc = int32(len(os.Args))

// CFExecv is C `execv(_:_:)`: it replaces this process with the program at path, given argv and
// this process's environment. It returns only on failure: -1 with errno set.
func CFExecv(path *any, argv any) int32 {
	return failed[int32](syscall.Exec(cStringArg(path), cStringList(argv), os.Environ()))
}

// spawnFileActions is a C `posix_spawn_file_actions_t`: descriptor changes to make in the child,
// in order. A dup2 has a target; a close has target -1.
type spawnFileActions struct{ actions [][2]int }

func fileActionsOf(fa **any) *spawnFileActions {
	if fa == nil || *fa == nil {
		return nil
	}
	a, _ := (**fa).(*spawnFileActions)
	return a
}

// CFPosix_spawn_file_actions_init is C `posix_spawn_file_actions_init(_:)`.
func CFPosix_spawn_file_actions_init(fa **any) int32 { //nolint:revive // C name
	var held any = &spawnFileActions{}
	*fa = &held
	return 0
}

// CFPosix_spawn_file_actions_destroy is C `posix_spawn_file_actions_destroy(_:)`.
func CFPosix_spawn_file_actions_destroy(fa **any) int32 { //nolint:revive // C name
	if fileActionsOf(fa) == nil {
		return CMacroEINVAL
	}
	*fa = nil
	return 0
}

// CFPosix_spawn_file_actions_adddup2 is C `posix_spawn_file_actions_adddup2(_:_:_:)`: the child
// gets fd duplicated onto newfd. It returns 0, or an error number.
func CFPosix_spawn_file_actions_adddup2(fa **any, fd, newfd int32) int32 { //nolint:revive // C name
	a := fileActionsOf(fa)
	if a == nil || fd < 0 || newfd < 0 {
		return CMacroEBADF
	}
	a.actions = append(a.actions, [2]int{int(fd), int(newfd)})
	return 0
}

// CFPosix_spawn_file_actions_addclose is C `posix_spawn_file_actions_addclose(_:_:)`: the child
// gets fd closed. It returns 0, or an error number.
func CFPosix_spawn_file_actions_addclose(fa **any, fd int32) int32 { //nolint:revive // C name
	a := fileActionsOf(fa)
	if a == nil || fd < 0 {
		return CMacroEBADF
	}
	a.actions = append(a.actions, [2]int{int(fd), -1})
	return 0
}

const closedInChild = ^uintptr(0)

// childFiles is the child's descriptor table after the file actions, as syscall.ProcAttr.Files
// wants it: entry i is the parent descriptor the child has as i. A descriptor no action touched
// is inherited as posix_spawn inherits it: kept when open and not close-on-exec.
func (a *spawnFileActions) childFiles() []uintptr {
	table := map[int]uintptr{}
	source := func(fd int) uintptr {
		if src, ok := table[fd]; ok {
			return src
		}
		return uintptr(fd)
	}
	top := 2
	for _, action := range a.actions {
		fd, target := action[0], action[1]
		if target < 0 {
			table[fd] = closedInChild
			top = max(top, fd)
			continue
		}
		table[target] = source(fd)
		top = max(top, target)
	}
	files := make([]uintptr, top+1)
	for fd := range files {
		if src, ok := table[fd]; ok {
			files[fd] = src
			continue
		}
		flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
		if err != nil || flags&unix.FD_CLOEXEC != 0 {
			files[fd] = closedInChild
		} else {
			files[fd] = uintptr(fd)
		}
	}
	return files
}

// CFPosix_spawn is C `posix_spawn(_:_:_:_:_:_:)`: it starts the program at path as a child with
// argv, the environment envp (a C array of `NAME=value` strings; none when nil) and the
// descriptor changes of the file actions, stores the child's process ID, and returns 0, or the
// error number when the child could not be started. Spawn attributes are not supported and panic.
func CFPosix_spawn(pid *int32, path *any, fileActions **any, attributes any, argv any, envp any) int32 { //nolint:revive // C name
	if !isNil(attributes) {
		panic("swiftrt: posix_spawn attributes are not supported")
	}
	attr := &syscall.ProcAttr{Env: []string{}}
	if !isNil(envp) && !(reflect.TypeOf(envp) == reflect.TypeFor[**any]() && *envp.(**any) == nil) {
		attr.Env = cStringList(envp)
	}
	actions := fileActionsOf(fileActions)
	if actions == nil {
		actions = &spawnFileActions{}
	}
	attr.Files = actions.childFiles()
	child, err := syscall.ForkExec(cStringArg(path), cStringList(argv), attr)
	if err != nil {
		var errno syscall.Errno
		if errors.As(err, &errno) {
			return int32(errno)
		}
		return int32(syscall.EIO)
	}
	if pid != nil {
		*pid = int32(child)
	}
	return 0
}
