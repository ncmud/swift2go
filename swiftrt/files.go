package swiftrt

import "os"

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

// FileHandle is Foundation `FileHandle`.
type FileHandle struct{ File *os.File }

// NSFileHandleInitFileDescriptorCloseOnDealloc is Foundation `FileHandle(fileDescriptor:closeOnDealloc:)`.
// The descriptor is closed when the handle is closed or garbage-collected.
func NSFileHandleInitFileDescriptorCloseOnDealloc(fd int32, _ bool) FileHandle {
	return FileHandle{os.NewFile(uintptr(fd), "")}
}
