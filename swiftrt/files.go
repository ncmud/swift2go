package swiftrt

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"syscall"
	"time"
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
type FileHandle struct {
	File *os.File
	null bool
}

// NSFileHandleNullDevice is Foundation `FileHandle.nullDevice`: reads from it are empty, writes
// to it are discarded, and closing it does nothing.
var NSFileHandleNullDevice = FileHandle{null: true}

var devNull = sync.OnceValue(func() *os.File {
	f, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		panic("swiftrt: cannot open " + os.DevNull + ": " + err.Error())
	}
	return f
})

// file is the open file behind the handle, or nil once it is closed.
func (h FileHandle) file() *os.File {
	if h.null {
		return devNull()
	}
	return h.File
}

// open is the handle's file, throwing the POSIX "bad file descriptor" error when it is closed.
func (h FileHandle) open() *os.File {
	f := h.file()
	if f == nil {
		panic(Throw(NSError{Domain: "NSPOSIXErrorDomain", Code: int(syscall.EBADF), Msg: "The operation could not be completed. Bad file descriptor"}))
	}
	return f
}

// NSFileHandleInitFileDescriptorCloseOnDealloc is Foundation `FileHandle(fileDescriptor:closeOnDealloc:)`.
// The descriptor is closed when the handle is closed or garbage-collected.
func NSFileHandleInitFileDescriptorCloseOnDealloc(fd int32, _ bool) FileHandle {
	return FileHandle{File: os.NewFile(uintptr(fd), "")}
}

// NSFileHandleStandardError is Foundation `FileHandle.standardError`.
var NSFileHandleStandardError = FileHandle{File: os.Stderr}

// NSFileHandleStandardOutput is Foundation `FileHandle.standardOutput`.
var NSFileHandleStandardOutput = FileHandle{File: os.Stdout}

// NSFileHandleWrite is Foundation `FileHandle.write(_:)`: it writes all of data and, like
// `try! write(contentsOf:)` on Linux, traps when the write fails.
func NSFileHandleWrite(h FileHandle, data []byte) {
	f := h.file()
	if f == nil {
		Trap("'try!' expression unexpectedly raised an error: the file handle is closed")
	}
	if _, err := f.Write(data); err != nil {
		Trap("'try!' expression unexpectedly raised an error: " + err.Error())
	}
}

// NSFileHandleClose is Foundation `FileHandle.close()`: it closes the descriptor, throwing when the
// close fails.
func NSFileHandleClose(h FileHandle) {
	if h.null || h.File == nil {
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

// cocoaWriteError is the NSError Foundation on Linux reports for a failed file-system write.
func cocoaWriteError(path string, err error) NSError {
	switch {
	case path == "" || errors.Is(err, fs.ErrNotExist):
		return NSError{Domain: "NSCocoaErrorDomain", Code: 4, Msg: "The operation could not be completed. The file doesn’t exist."}
	case errors.Is(err, fs.ErrPermission) || errors.Is(err, syscall.EACCES):
		return NSError{Domain: "NSCocoaErrorDomain", Code: 513, Msg: "The operation could not be completed. You don’t have permission."}
	case errors.Is(err, fs.ErrExist):
		return NSError{Domain: "NSCocoaErrorDomain", Code: 516, Msg: "The operation could not be completed. The file already exists."}
	}
	return NSError{Domain: "NSCocoaErrorDomain", Code: 512, Msg: "The operation could not be completed. (NSCocoaErrorDomain error 512.)"}
}

// NSFileHandleInitForWritingTo is Foundation `FileHandle(forWritingTo:)`: it opens the existing
// file at a file URL for writing from its start, without truncating it, and throws when the file
// is missing or cannot be opened.
func NSFileHandleInitForWritingTo(u URL) FileHandle {
	path := fileURLPath(u)
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		panic(Throw(cocoaWriteError(path, err)))
	}
	return FileHandle{File: f}
}

// NSFileHandleOptInitForWritingAtPath is Foundation `FileHandle(forWritingAtPath:)`: the existing
// file opened for writing from its start, or nil when it cannot be opened.
func NSFileHandleOptInitForWritingAtPath(path string) *FileHandle {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return nil
	}
	return &FileHandle{File: f}
}

// NSFileHandleOptInitForReadingAtPath is Foundation `FileHandle(forReadingAtPath:)`: the file
// opened for reading, or nil when it cannot be opened.
func NSFileHandleOptInitForReadingAtPath(path string) *FileHandle {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	return &FileHandle{File: f}
}

// NSFileHandleWriteContentsOf is Foundation `FileHandle.write(contentsOf:)`: it writes all of data
// at the file's current offset, throwing when the write fails or the handle is closed.
func NSFileHandleWriteContentsOf(h FileHandle, data []byte) {
	if _, err := h.open().Write(data); err != nil {
		panic(Throw(err))
	}
}

// NSFileHandleSynchronize is Foundation `FileHandle.synchronize()`: it flushes the file's data to
// its device, throwing when that fails.
func NSFileHandleSynchronize(h FileHandle) {
	if h.null {
		return
	}
	if err := h.open().Sync(); err != nil {
		panic(Throw(err))
	}
}

// NSFileHandleSeekToOffset is Foundation `FileHandle.seek(toOffset:)`: it moves the file's offset
// to that many bytes from its start, throwing when the file cannot seek.
func NSFileHandleSeekToOffset(h FileHandle, offset uint64) {
	if offset > math.MaxInt64 {
		panic(Throw(NSError{Domain: "NSPOSIXErrorDomain", Code: int(syscall.EINVAL), Msg: "The operation could not be completed. Invalid argument"}))
	}
	if _, err := h.open().Seek(int64(offset), io.SeekStart); err != nil {
		panic(Throw(err))
	}
}

// NSFileHandleSeekToEnd is Foundation `FileHandle.seekToEnd()`: it moves the file's offset to its
// end and returns that offset, throwing when the file cannot seek.
func NSFileHandleSeekToEnd(h FileHandle) uint64 {
	end, err := h.open().Seek(0, io.SeekEnd)
	if err != nil {
		panic(Throw(err))
	}
	return uint64(end)
}

// NSFileHandleSeekToEndOfFile is Foundation `FileHandle.seekToEndOfFile()`: seekToEnd() that
// traps where it would throw.
func NSFileHandleSeekToEndOfFile(h FileHandle) uint64 {
	f := h.file()
	if f == nil {
		Trap("'try!' expression unexpectedly raised an error: the file handle is closed")
	}
	end, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		Trap("'try!' expression unexpectedly raised an error: " + err.Error())
	}
	return uint64(end)
}

// NSFileHandleReadDataToEndOfFile is Foundation `FileHandle.readDataToEndOfFile()`: the bytes from
// the file's offset to its end, trapping when the read fails.
func NSFileHandleReadDataToEndOfFile(h FileHandle) []byte {
	if h.null {
		return []byte{}
	}
	f := h.file()
	if f == nil {
		Trap("'try!' expression unexpectedly raised an error: the file handle is closed")
	}
	data, err := io.ReadAll(f)
	if err != nil {
		Trap("'try!' expression unexpectedly raised an error: " + err.Error())
	}
	return data
}

// NSFileHandleCloseFile is Foundation `FileHandle.closeFile()`: close() that ignores a failure.
func NSFileHandleCloseFile(h FileHandle) {
	if h.null || h.File == nil {
		return
	}
	_ = h.File.Close()
}

// NSFileManagerTemporaryDirectory is FileManager's `temporaryDirectory`: the directory of os.TempDir.
func NSFileManagerTemporaryDirectory(FileManager) URL { return directoryURL(os.TempDir()) }

// NSFileManagerCurrentDirectoryPath is Foundation `FileManager.currentDirectoryPath`.
func NSFileManagerCurrentDirectoryPath(FileManager) string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	return dir
}

// NSFileManagerCreateFileAtPathContents is Foundation `FileManager.createFile(atPath:contents:)`:
// it writes the contents, or nothing for nil, to the file, replacing one already there, and
// reports whether it could.
func NSFileManagerCreateFileAtPathContents(_ FileManager, path string, contents *[]byte) bool {
	var data []byte
	if contents != nil {
		data = *contents
	}
	return os.WriteFile(path, data, 0o644) == nil
}

// NSFileManagerRemoveItemAtPath is Foundation `FileManager.removeItem(atPath:)`: it removes the
// file, or the directory and everything in it, throwing when nothing exists at the path.
func NSFileManagerRemoveItemAtPath(_ FileManager, path string) {
	if _, err := os.Lstat(path); err != nil {
		panic(Throw(cocoaWriteError(path, err)))
	}
	if err := os.RemoveAll(path); err != nil {
		panic(Throw(cocoaWriteError(path, err)))
	}
}

// NSFileManagerMoveItemAtPathToPath is Foundation `FileManager.moveItem(atPath:toPath:)`, which
// moves as NSFileManagerMoveItemAtTo does.
func NSFileManagerMoveItemAtPathToPath(_ FileManager, src, dst string) { moveItem(src, dst) }

// NSFileManagerMoveItemAtTo is Foundation `FileManager.moveItem(at:to:)`: it renames the item,
// copying it across file systems when it must. It throws NSCocoaErrorDomain 516 when the
// destination exists and 4 when the source does not.
func NSFileManagerMoveItemAtTo(_ FileManager, src, dst URL) {
	moveItem(fileURLPath(src), fileURLPath(dst))
}

func moveItem(from, to string) {
	if _, err := os.Lstat(from); err != nil {
		panic(Throw(cocoaWriteError(from, err)))
	}
	if _, err := os.Lstat(to); err == nil {
		panic(Throw(cocoaWriteError(to, fs.ErrExist)))
	}
	err := os.Rename(from, to)
	if errors.Is(err, syscall.EXDEV) {
		if err = copyTree(from, to); err == nil {
			err = os.RemoveAll(from)
		}
	}
	if err != nil {
		panic(Throw(cocoaWriteError(to, err)))
	}
}

// FileAttributeKey is Foundation `FileAttributeKey`.
type FileAttributeKey string

// NSFileAttributeKeyCNSFilePosixPermissions is `FileAttributeKey.posixPermissions`: the item's
// permission bits, as an integer.
const NSFileAttributeKeyCNSFilePosixPermissions FileAttributeKey = "NSFilePosixPermissions"

// NSFileAttributeKeyCNSFileModificationDate is `FileAttributeKey.modificationDate`, a Date.
const NSFileAttributeKeyCNSFileModificationDate FileAttributeKey = "NSFileModificationDate"

// NSFileAttributeKeyCNSFileSize is `FileAttributeKey.size`, the item's size in bytes.
const NSFileAttributeKeyCNSFileSize FileAttributeKey = "NSFileSize"

// NSFileManagerAttributesOfItemAtPath is Foundation `FileManager.attributesOfItem(atPath:)`: the item's
// permission bits, modification date and size, throwing when it is missing.
func NSFileManagerAttributesOfItemAtPath(_ FileManager, path string) map[FileAttributeKey]any {
	fi, err := os.Stat(path)
	if err != nil {
		panic(Throw(cocoaFileError(path, err)))
	}
	return map[FileAttributeKey]any{
		NSFileAttributeKeyCNSFilePosixPermissions: int(fi.Mode().Perm()),
		NSFileAttributeKeyCNSFileModificationDate: dateOf(fi.ModTime()),
		NSFileAttributeKeyCNSFileSize:             fi.Size(),
	}
}

// NSFileManagerSetAttributesOfItemAtPath is Foundation `FileManager.setAttributes(_:ofItemAtPath:)`
// for `.posixPermissions` and `.modificationDate`, throwing when the item is missing or cannot be changed. Any other
// attribute is not supported and panics.
func NSFileManagerSetAttributesOfItemAtPath(_ FileManager, attributes map[FileAttributeKey]any, path string) {
	for key, value := range attributes {
		if key == NSFileAttributeKeyCNSFileModificationDate {
			d, ok := value.(Date)
			if !ok {
				panic(fmt.Sprintf("swiftrt: file attribute %s needs a Date, not %T", key, value))
			}
			if err := os.Chtimes(path, time.Time{}, d.Time()); err != nil {
				panic(Throw(cocoaWriteError(path, err)))
			}
			continue
		}
		if key != NSFileAttributeKeyCNSFilePosixPermissions {
			panic(fmt.Sprintf("swiftrt: setting file attribute %s is not supported", key))
		}
		var mode uint32
		switch rv := reflect.ValueOf(value); {
		case rv.CanInt():
			mode = uint32(rv.Int())
		case rv.CanUint():
			mode = uint32(rv.Uint())
		default:
			panic(fmt.Sprintf("swiftrt: file attribute %s needs an integer, not %T", key, value))
		}
		if err := syscall.Chmod(path, mode&0o7777); err != nil {
			panic(Throw(cocoaWriteError(path, err)))
		}
	}
}

// NSFileManagerCreateDirectoryAtWithIntermediateDirectoriesAttributes is Foundation
// `FileManager.createDirectory(at:withIntermediateDirectories:attributes:)`. With intermediates it
// creates missing parents and accepts an existing directory; without, it throws when the parent is
// missing or the path exists. The attributes are set on the directory the URL names.
func NSFileManagerCreateDirectoryAtWithIntermediateDirectoriesAttributes(fm FileManager, u URL, intermediates bool, attributes *map[FileAttributeKey]any) {
	path := fileURLPath(u)
	var err error
	if intermediates {
		err = os.MkdirAll(path, 0o777)
	} else {
		err = os.Mkdir(path, 0o777)
	}
	if err != nil {
		panic(Throw(cocoaWriteError(path, err)))
	}
	if attributes != nil {
		NSFileManagerSetAttributesOfItemAtPath(fm, *attributes, path)
	}
}

// NSFileManagerCreateDirectoryAtWithIntermediateDirectories is Foundation
// `FileManager.createDirectory(at:withIntermediateDirectories:)`.
func NSFileManagerCreateDirectoryAtWithIntermediateDirectories(fm FileManager, u URL, intermediates bool) {
	NSFileManagerCreateDirectoryAtWithIntermediateDirectoriesAttributes(fm, u, intermediates, nil)
}
