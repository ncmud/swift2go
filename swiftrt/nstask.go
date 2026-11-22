package swiftrt

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"sync"
	"syscall"
)

// Task is Foundation `Process`, under the Objective-C name NSTask the emitter calls it by: a child
// process to configure, run and wait for. Copies of a Task are the same process.
//
// A nil StandardInput, StandardOutput or StandardError leaves the child with this process's
// stream; a FileHandle gives it that file.
type Task struct {
	ExecutableURL       *URL
	Arguments           *[]string
	Environment         *map[string]string
	CurrentDirectoryURL *URL
	StandardInput       any
	StandardOutput      any
	StandardError       any

	state *taskState
}

type taskState struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	done   chan struct{}
	exited bool
	status int32
}

// NSTaskInit is Foundation `Process()`.
func NSTaskInit() Task { return Task{state: &taskState{}} }

// taskStream is the file a child's standard stream goes to: inherited when the property is nil.
func taskStream(v any, inherited *os.File) *os.File {
	if isNil(v) {
		return inherited
	}
	switch h := v.(type) {
	case FileHandle:
		return h.open()
	case *FileHandle:
		return h.open()
	}
	panic(fmt.Sprintf("swiftrt: Process cannot use a %T as a standard stream", v))
}

// NSTaskRun is Foundation `Process.run()`: it starts the executable with the arguments,
// environment (this process's when nil), working directory and standard streams set on the
// process. It throws NSCocoaErrorDomain 4 when there is no executable at the URL, and the POSIX
// error when the executable cannot be started.
func NSTaskRun(t Task) {
	s := t.state
	if s == nil {
		panic("swiftrt: Process was not made by Process()")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cmd != nil {
		Trap("Process.run() called on a process that has already been launched")
	}
	missing := NSError{Domain: "NSCocoaErrorDomain", Code: 4, Msg: "The operation could not be completed. The file doesn’t exist."}
	if t.ExecutableURL == nil || URLScheme(*t.ExecutableURL) == nil || *URLScheme(*t.ExecutableURL) != "file" {
		panic(Throw(missing))
	}
	path := URLPath(*t.ExecutableURL)
	cmd := &exec.Cmd{Path: path, Args: []string{path}}
	if t.Arguments != nil {
		cmd.Args = append(cmd.Args, *t.Arguments...)
	}
	cmd.Env = os.Environ()
	if t.Environment != nil {
		cmd.Env = make([]string, 0, len(*t.Environment))
		for k, v := range *t.Environment {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	if t.CurrentDirectoryURL != nil {
		cmd.Dir = URLPath(*t.CurrentDirectoryURL)
	}
	cmd.Stdin = taskStream(t.StandardInput, os.Stdin)
	cmd.Stdout = taskStream(t.StandardOutput, os.Stdout)
	cmd.Stderr = taskStream(t.StandardError, os.Stderr)
	if err := cmd.Start(); err != nil {
		var errno syscall.Errno
		switch {
		case errors.Is(err, fs.ErrNotExist):
			panic(Throw(missing))
		case errors.As(err, &errno):
			panic(Throw(NSError{Domain: "NSPOSIXErrorDomain", Code: int(errno), Msg: "The operation could not be completed. " + strerror(int32(errno))}))
		}
		panic(Throw(err))
	}
	s.cmd = cmd
	s.done = make(chan struct{})
	go func() {
		_ = cmd.Wait()
		s.mu.Lock()
		if ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			s.status = int32(ws.Signal())
		} else {
			s.status = int32(cmd.ProcessState.ExitCode())
		}
		s.exited = true
		s.mu.Unlock()
		close(s.done)
	}()
}

// NSTaskIsRunning is Foundation `Process.isRunning`: whether the process has been started and
// has not yet exited.
func NSTaskIsRunning(t Task) bool {
	t.state.mu.Lock()
	defer t.state.mu.Unlock()
	return t.state.cmd != nil && !t.state.exited
}

// NSTaskProcessIdentifier is Foundation `Process.processIdentifier`: the child's process ID, or 0
// before it is started.
func NSTaskProcessIdentifier(t Task) int32 {
	t.state.mu.Lock()
	defer t.state.mu.Unlock()
	if t.state.cmd == nil {
		return 0
	}
	return int32(t.state.cmd.Process.Pid)
}

// NSTaskWaitUntilExit is Foundation `Process.waitUntilExit()`: it blocks until the started process
// exits.
func NSTaskWaitUntilExit(t Task) {
	t.state.mu.Lock()
	done := t.state.done
	t.state.mu.Unlock()
	if done != nil {
		<-done
	}
}

// NSTaskTerminationStatus is Foundation `Process.terminationStatus`: the exit status of a process
// that exited, or the number of the signal that ended it. As on Linux it is 0 until the process
// exits.
func NSTaskTerminationStatus(t Task) int32 {
	t.state.mu.Lock()
	defer t.state.mu.Unlock()
	return t.state.status
}
