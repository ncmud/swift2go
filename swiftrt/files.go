package swiftrt

import (
	"io"
	"os"
	"path/filepath"
)

// FileManager is Foundation `FileManager`.
type FileManager struct{}

// NSFileManagerCObjc is `FileManager.default`.
var NSFileManagerCObjc = FileManager{}

// NSFileManagerCObjcAt is `FileManager.removeItem(at:)`: it removes the file, or the directory and
// everything in it, throwing when nothing exists at the URL.
func NSFileManagerCObjcAt(_ FileManager, u URL) {
	path := fileURLPath(u)
	if _, err := os.Lstat(path); err != nil {
		panic(Throw(err))
	}
	if err := os.RemoveAll(path); err != nil {
		panic(Throw(err))
	}
}

// NSFileManagerCObjcAtPathWithIntermediateDirectories is Foundation
// `FileManager.createDirectory(atPath:withIntermediateDirectories:)`. With intermediates it creates
// missing parents and accepts an existing directory; without, it throws when the parent is missing or
// the path exists.
func NSFileManagerCObjcAtPathWithIntermediateDirectories(_ FileManager, path string, intermediates bool) {
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

// NSFileManagerCObjcAtTo is Foundation `FileManager.copyItem(at:to:)`: it copies a file, a symbolic
// link, or a directory and everything in it. It throws NSCocoaErrorDomain 516 when the destination
// exists and 260 when the source does not.
func NSFileManagerCObjcAtTo(_ FileManager, src, dst URL) {
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
		out.Close()
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
