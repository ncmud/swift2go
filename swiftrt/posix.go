package swiftrt

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"syscall"
	"unicode"
	"unicode/utf8"
	"unsafe"
)

// cStringArg reads a C string argument: the optional pointer the generated code passes for a Swift
// String bridged to `UnsafePointer<CChar>`, holding the string or its bytes.
func cStringArg(p *any) string {
	if p == nil {
		Trap("unexpectedly found nil while unwrapping an Optional value")
	}
	return StringInitCString(*p)
}

// cStringResult is a C string result: the NUL-terminated bytes `String(cString:)` reads.
func cStringResult(s string) *any {
	var v any = append([]byte(s), 0)
	return &v
}

// CStringOf is a String passed where C expects a `const char *`.
func CStringOf(s string) *any { return cStringResult(s) }

// CFGetenv is C `getenv(_:)`: the variable's value as a C string, or nil when it is unset.
func CFGetenv(name *any) *any {
	v, ok := os.LookupEnv(cStringArg(name))
	if !ok {
		return nil
	}
	return cStringResult(v)
}

// CFSetenv is C `setenv(_:_:_:)`: it sets the variable, leaving an existing value in place when
// overwrite is 0, and returns 0, or -1 when the name is empty or contains `=`.
func CFSetenv(name, value *any, overwrite int32) int32 {
	n := cStringArg(name)
	if n == "" || strings.Contains(n, "=") {
		return -1
	}
	if _, ok := os.LookupEnv(n); ok && overwrite == 0 {
		return 0
	}
	if os.Setenv(n, cStringArg(value)) != nil {
		return -1
	}
	return 0
}

// CFUnlink is C `unlink(_:)`: it removes the directory entry and returns 0, or -1 on failure.
func CFUnlink(path *any) int32 {
	if syscall.Unlink(cStringArg(path)) != nil {
		return -1
	}
	return 0
}

// CFRename is C `rename(_:_:)`: it renames the file, replacing any file at the destination, and
// returns 0, or -1 on failure.
func CFRename(from, to *any) int32 {
	if syscall.Rename(cStringArg(from), cStringArg(to)) != nil {
		return -1
	}
	return 0
}

// CFStrerror is C `strerror(_:)`: the platform C library's message for the error number, as a C
// string.
func CFStrerror(errnum int32) *any {
	return cStringResult(strerror(errnum))
}

func strerror(errnum int32) string {
	if errnum == 0 {
		if runtime.GOOS == "darwin" {
			return "Undefined error: 0"
		}
		return "Success"
	}
	if runtime.GOOS == "darwin" && errnum == 107 {
		return "Capabilities insufficient"
	}
	msg := syscall.Errno(errnum).Error()
	if errnum < 0 || strings.HasPrefix(msg, "errno ") {
		if runtime.GOOS == "darwin" {
			return fmt.Sprintf("Unknown error: %d", errnum)
		}
		return fmt.Sprintf("Unknown error %d", errnum)
	}
	r, n := utf8.DecodeRuneInString(msg)
	return string(unicode.ToUpper(r)) + msg[n:]
}

// C `open(2)` flags, with the host platform's values.
const (
	CMacroO_RDONLY  int32 = syscall.O_RDONLY
	CMacroO_WRONLY  int32 = syscall.O_WRONLY
	CMacroO_RDWR    int32 = syscall.O_RDWR
	CMacroO_APPEND  int32 = syscall.O_APPEND
	CMacroO_CREAT   int32 = syscall.O_CREAT
	CMacroO_TRUNC   int32 = syscall.O_TRUNC
	CMacroO_EXCL    int32 = syscall.O_EXCL
	CMacroO_CLOEXEC int32 = syscall.O_CLOEXEC
)

// OpenUnsafePointerIntInt is C `open(_:_:_:)`: it opens path, a C string, with the flags and
// creation mode and returns the descriptor, or -1 on failure.
func OpenUnsafePointerIntInt[M Integer](path any, flags int32, mode M) int32 {
	fd, err := syscall.Open(StringInitCString(path), int(flags), uint32(mode))
	if err != nil {
		return -1
	}
	return int32(fd)
}

// cCharAt reads element i of a C character pointer: the bytes of a C string the runtime hands out
// as `UnsafePointer<CChar>`, or a Swift String passed where C expects one.
func cCharAt(p any, i int) int8 {
	switch v := p.(type) {
	case []int8:
		return v[i]
	case []byte:
		return int8(v[i])
	case string:
		if i == len(v) {
			return 0
		}
		return int8(v[i])
	case *any:
		if v == nil {
			Trap("unexpectedly found nil while unwrapping an Optional value")
		}
		return cCharAt(*v, i)
	case *uint8:
		return int8(*(*uint8)(unsafe.Add(unsafe.Pointer(v), i))) //nolint:gosec // the caller vouches for i elements at v, as in C
	case *int8:
		return *(*int8)(unsafe.Add(unsafe.Pointer(v), i)) //nolint:gosec // the caller vouches for i elements at v, as in C
	}
	panic(fmt.Sprintf("swiftrt: %T is not a C character pointer", p))
}

// UnsafePointerSubscriptInt is Swift `UnsafePointer<CChar>[i]`: the C character at offset i.
func UnsafePointerSubscriptInt(p any, i int) int8 { return cCharAt(p, i) }

// UnsafePointerPointee is Swift `UnsafePointer<CChar>.pointee`: the first C character.
func UnsafePointerPointee(p any) int8 { return cCharAt(p, 0) }

// StringWithCString is Swift `String.withCString(_:)`: body receives a pointer to the string's
// NUL-terminated UTF-8 bytes, valid for the call.
func StringWithCString[R any](s string, body func(*any) R) R {
	return body(cStringResult(s))
}

// UnsafeMutablePointerSubscriptInt is Swift `UnsafeMutablePointer<CChar>[i]`: the C character at
// offset i.
func UnsafeMutablePointerSubscriptInt(p any, i int) int8 { return cCharAt(p, i) }

// UnsafeMutablePointerPointee is Swift `UnsafeMutablePointer<CChar>.pointee`: the first C character.
func UnsafeMutablePointerPointee(p any) int8 { return cCharAt(p, 0) }

// CFStrlen is C `strlen(_:)`: the number of C characters before the terminating NUL.
func CFStrlen(p any) int {
	n := 0
	for cCharAt(p, n) != 0 {
		n++
	}
	return n
}
