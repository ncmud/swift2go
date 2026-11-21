package swiftrt

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// FileManager is Foundation `FileManager`.
type FileManager struct{}

// NSFileManagerDefault is `FileManager.default`.
var NSFileManagerDefault = FileManager{}

// NSFileManagerRemoveItemAt is `FileManager.removeItem(at:)`: it removes the file, or the directory and
// everything in it, throwing when nothing exists at the URL.
func NSFileManagerRemoveItemAt(_ FileManager, u URL) {
	path := fileURLPath(u)
	if _, err := os.Lstat(path); err != nil {
		panic(Throw(err))
	}
	if err := os.RemoveAll(path); err != nil {
		panic(Throw(err))
	}
}

// NSFileManagerCreateDirectoryAtPathWithIntermediateDirectories is Foundation
// `FileManager.createDirectory(atPath:withIntermediateDirectories:)`. With intermediates it creates
// missing parents and accepts an existing directory; without, it throws when the parent is missing or
// the path exists.
func NSFileManagerCreateDirectoryAtPathWithIntermediateDirectories(_ FileManager, path string, intermediates bool) {
	var err error
	if intermediates {
		err = os.MkdirAll(path, 0o777)
	} else {
		err = os.Mkdir(path, 0o777)
	}
	if err != nil {
		panic(Throw(err))
	}
}

// NSFileManagerCopyItemAtTo is Foundation `FileManager.copyItem(at:to:)`: it copies a file, a symbolic
// link, or a directory and everything in it. It throws NSCocoaErrorDomain 516 when the destination
// exists and 260 when the source does not.
func NSFileManagerCopyItemAtTo(_ FileManager, src, dst URL) {
	from, to := fileURLPath(src), fileURLPath(dst)
	if _, err := os.Lstat(from); err != nil {
		panic(Throw(NSError{Domain: "NSCocoaErrorDomain", Code: 260, Msg: "The file couldn't be opened because there is no such file."}))
	}
	if _, err := os.Lstat(to); err == nil {
		panic(Throw(NSError{Domain: "NSCocoaErrorDomain", Code: 516, Msg: "The file couldn't be copied because an item with the same name already exists."}))
	}
	if err := copyTree(from, to); err != nil {
		panic(Throw(err))
	}
}

func copyTree(from, to string) error {
	info, err := os.Lstat(from)
	if err != nil {
		return err
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(from)
		if err != nil {
			return err
		}
		return os.Symlink(target, to)
	case info.IsDir():
		if err := os.Mkdir(to, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(from)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := copyTree(filepath.Join(from, e.Name()), filepath.Join(to, e.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// FileHandle is Foundation `FileHandle`.
type FileHandle struct{ File *os.File }

// NSFileHandleInitFileDescriptorCloseOnDealloc is Foundation `FileHandle(fileDescriptor:closeOnDealloc:)`.
// The descriptor is closed when the handle is closed or garbage-collected.
func NSFileHandleInitFileDescriptorCloseOnDealloc(fd int32, _ bool) FileHandle {
	return FileHandle{os.NewFile(uintptr(fd), "")}
}

// NSFileHandleStandardError is Foundation `FileHandle.standardError`.
var NSFileHandleStandardError = FileHandle{os.Stderr}

// NSFileHandleWrite is Foundation `FileHandle.write(_:)`: it writes all of data and, like
// `try! write(contentsOf:)` on Linux, traps when the write fails.
func NSFileHandleWrite(h FileHandle, data []byte) {
	if h.File == nil {
		Trap("'try!' expression unexpectedly raised an error: the file handle is closed")
	}
	if _, err := h.File.Write(data); err != nil {
		Trap("'try!' expression unexpectedly raised an error: " + err.Error())
	}
}

// NSFileHandleClose is Foundation `FileHandle.close()`: it closes the descriptor, throwing when the
// close fails.
func NSFileHandleClose(h FileHandle) {
	if h.File == nil {
		return
	}
	if err := h.File.Close(); err != nil {
		panic(Throw(err))
	}
}

// NSFileManagerFileExistsAtPath is Foundation `FileManager.fileExists(atPath:)`: whether path names
// an existing item, following a final symbolic link, so a dangling link does not exist.
func NSFileManagerFileExistsAtPath(_ FileManager, path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// NSFileManagerContentsAtPath is Foundation `FileManager.contents(atPath:)`: the file's bytes, or nil
// when it cannot be read, including when path is a directory.
func NSFileManagerContentsAtPath(_ FileManager, path string) *[]byte {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return &b
}

// NSFileManagerContentsOfDirectoryAtPath is Foundation `FileManager.contentsOfDirectory(atPath:)`:
// the directory's entry names, hidden ones included and `.` and `..` excluded, in the order the
// file system lists them (unsorted, as on Linux). It throws NSCocoaErrorDomain 260 when the path
// does not exist, 257 when permission is denied, and 256 otherwise.
func NSFileManagerContentsOfDirectoryAtPath(_ FileManager, path string) []string {
	f, err := os.Open(path)
	var names []string
	if err == nil {
		names, err = f.Readdirnames(-1)
		_ = f.Close()
	}
	if err != nil {
		panic(Throw(cocoaFileError(path, err)))
	}
	if names == nil {
		names = []string{}
	}
	return names
}

// cocoaFileError is the NSError Foundation on Linux reports for a failed file-system read.
func cocoaFileError(path string, err error) NSError {
	switch {
	case path == "" || errors.Is(err, fs.ErrNotExist):
		return NSError{Domain: "NSCocoaErrorDomain", Code: 260, Msg: "The operation could not be completed. The file doesn’t exist."}
	case errors.Is(err, fs.ErrPermission) || errors.Is(err, syscall.EACCES):
		return NSError{Domain: "NSCocoaErrorDomain", Code: 257, Msg: "The operation could not be completed. You don’t have permission."}
	}
	return NSError{Domain: "NSCocoaErrorDomain", Code: 256, Msg: "The operation could not be completed. (NSCocoaErrorDomain error 256.)"}
}
