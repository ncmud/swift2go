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

// FileHandle is Foundation `FileHandle`.
type FileHandle struct{ File *os.File }

// NSFileHandleInitFileDescriptorCloseOnDealloc is Foundation `FileHandle(fileDescriptor:closeOnDealloc:)`.
// The descriptor is closed when the handle is closed or garbage-collected.
func NSFileHandleInitFileDescriptorCloseOnDealloc(fd int32, _ bool) FileHandle {
	return FileHandle{os.NewFile(uintptr(fd), "")}
}
