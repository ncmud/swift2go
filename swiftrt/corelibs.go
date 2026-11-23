package swiftrt

import (
	"errors"
	"math"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

// The names below are the ones a package compiled for Linux calls where Darwin's Objective-C and C
// imports have names of their own: Foundation's option sets as Swift statics, C library functions
// that Darwin's overlay wraps, and the C library's types and macros.

// Foundation's option sets as Linux declares them.
const (
	NSStringCompareOptionsCaseInsensitive   = NSStringCompareOptionsCENSStringCompareOptionsNSCaseInsensitiveSearch
	NSStringCompareOptionsLiteral           = NSStringCompareOptionsCENSStringCompareOptionsNSLiteralSearch
	NSStringCompareOptionsBackwards         = NSStringCompareOptionsCENSStringCompareOptionsNSBackwardsSearch
	NSStringCompareOptionsAnchored          = NSStringCompareOptionsCENSStringCompareOptionsNSAnchoredSearch
	NSStringCompareOptionsRegularExpression = NSStringCompareOptionsCENSStringCompareOptionsNSRegularExpressionSearch

	NSISO8601DateFormatOptionsWithYear                     = NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithYear
	NSISO8601DateFormatOptionsWithMonth                    = NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithMonth
	NSISO8601DateFormatOptionsWithWeekOfYear               = NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithWeekOfYear
	NSISO8601DateFormatOptionsWithDay                      = NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithDay
	NSISO8601DateFormatOptionsWithTime                     = NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithTime
	NSISO8601DateFormatOptionsWithTimeZone                 = NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithTimeZone
	NSISO8601DateFormatOptionsWithSpaceBetweenDateAndTime  = NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithSpaceBetweenDateAndTime
	NSISO8601DateFormatOptionsWithDashSeparatorInDate      = NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithDashSeparatorInDate
	NSISO8601DateFormatOptionsWithColonSeparatorInTime     = NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithColonSeparatorInTime
	NSISO8601DateFormatOptionsWithColonSeparatorInTimeZone = NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithColonSeparatorInTimeZone
	NSISO8601DateFormatOptionsWithFractionalSeconds        = NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithFractionalSeconds
	NSISO8601DateFormatOptionsWithFullDate                 = NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithFullDate
	NSISO8601DateFormatOptionsWithFullTime                 = NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithFullTime
	NSISO8601DateFormatOptionsWithInternetDateTime         = NSISO8601DateFormatOptionsCENSISO8601DateFormatOptionsNSISO8601DateFormatWithInternetDateTime
)

// JSONSerialization.WritingOptions as Linux declares them.
var (
	NSJSONWritingOptionsPrettyPrinted          = NSJSONWritingOptionsCENSJSONWritingOptionsNSJSONWritingPrettyPrinted
	NSJSONWritingOptionsSortedKeys             = NSJSONWritingOptionsCENSJSONWritingOptionsNSJSONWritingSortedKeys
	NSJSONWritingOptionsFragmentsAllowed       = NSJSONWritingOptionsCENSJSONWritingOptionsNSJSONWritingFragmentsAllowed
	NSJSONWritingOptionsWithoutEscapingSlashes = NSJSONWritingOptionsCENSJSONWritingOptionsNSJSONWritingWithoutEscapingSlashes
)

// NSFileAttributeKeyPosixPermissions is `FileAttributeKey.posixPermissions` as Linux declares it.
var NSFileAttributeKeyPosixPermissions = NSFileAttributeKeyCNSFilePosixPermissions

// NSStringAppendingPathComponentString is `appendingPathComponent(_:)` on a String as Linux declares it.
func NSStringAppendingPathComponentString(path, component string) string {
	return NSStringAppendingPathComponent(path, component)
}

// NSNumberObjCType is `NSNumber.objCType`: a C string of one Objective-C type encoding, 'c' for a
// Bool, 'q' for a signed and 'Q' for an unsigned integer, 'd' for a Double.
func NSNumberObjCType(n NSNumber) []int8 {
	code := map[byte]int8{'b': 'c', 'i': 'q', 'u': 'Q', 'd': 'd'}[n.kind]
	return []int8{code, 0}
}

// CFLog is C `log(_:)`, which Linux calls where Darwin's overlay has its own `log`.
func CFLog(x float64) float64 { return math.Log(x) }

// CFLog2 is C `log2(_:)`.
func CFLog2(x float64) float64 { return math.Log2(x) }

// CX__sigset_t is the C library's `sigset_t` on Linux, a struct where Darwin's is an integer, named
// as the emitter names `__sigset_t`. The runtime's signal masks are integers either way.
type CX__sigset_t = uint64 //nolint:revive // C name

// CX__sigset_tInit is `sigset_t()`, the empty set.
func CX__sigset_tInit() CX__sigset_t { return 0 } //nolint:revive // C name

// CPosix_spawn_file_actions_t is the C `posix_spawn_file_actions_t` that Linux declares as a struct
// and Darwin as a pointer; the runtime keeps its file actions behind the pointer either way.
type CPosix_spawn_file_actions_t = *any //nolint:revive // C name

// CPosix_spawn_file_actions_tInit is `posix_spawn_file_actions_t()`, not yet initialized.
func CPosix_spawn_file_actions_tInit() CPosix_spawn_file_actions_t { return nil } //nolint:revive // C name

// C library macros as Linux defines them, where Darwin's differ or are missing.
const (
	CMacroCLOCK_MONOTONIC       = CEAClockid_t_CLOCK_MONOTONIC //nolint:revive // C name
	CMacroELIBBAD         int32 = 80                           //nolint:revive // C name
	CMacroIPPROTO_TCP     int32 = 6                            //nolint:revive // C name
	CMacroTCP_CORK        int32 = 3                            //nolint:revive // C name
)

// CEaIPPROTO_IPIPPROTO_TCP is glibc's `IPPROTO_TCP`, a case of an anonymous C enum that Swift imports as Int.
const CEaIPPROTO_IPIPPROTO_TCP = 6 //nolint:revive // C name

// CFSetsockopt is C `setsockopt(_:_:_:_:_:)` with an Int32 option value. A descriptor that a
// [DescriptorIO] owns is not a kernel socket of its own, so the option is accepted and dropped.
func CFSetsockopt(fd, level, name int32, value any, _ uint32) int32 { //nolint:revive // C name
	if ownerOf(fd) != nil {
		return 0
	}
	v, ok := int32Pointee(value)
	if !ok {
		return failed[int32](unix.EINVAL)
	}
	if err := unix.SetsockoptInt(int(fd), int(level), int(name), int(v)); err != nil {
		return failed[int32](err)
	}
	return 0
}

func int32Pointee(v any) (int32, bool) {
	switch p := v.(type) {
	case *int32:
		return *p, p != nil
	case *any:
		if p != nil {
			return int32Pointee(*p)
		}
	}
	return 0, false
}

// URLErrorBadServerResponse is `URLError.Code.badServerResponse`.
const URLErrorBadServerResponse = -1011

// URLErrorCodeBadServerResponse is `URLError.Code.badServerResponse` as a code value.
const URLErrorCodeBadServerResponse = URLErrorBadServerResponse

// URLErrorInitCode is `URLError(_:)`.
func URLErrorInitCode(code int) URLError { return URLError{Code: code} }

// URLSessionDataTask is Foundation `URLSessionDataTask` made with a completion handler.
type URLSessionDataTask struct{ *dataTask }

type dataTask struct {
	session URLSession
	request URLRequest
	handler func(*[]byte, *URLResponse, error)
	once    sync.Once
}

// NSURLSessionDataTaskWithCompletionHandler is `URLSession.dataTask(with:completionHandler:)`. The
// request starts on [NSURLSessionTaskResume].
func NSURLSessionDataTaskWithCompletionHandler(s URLSession, r URLRequest, handler func(*[]byte, *URLResponse, error)) URLSessionDataTask {
	return URLSessionDataTask{&dataTask{session: s, request: r, handler: handler}}
}

// NSURLSessionTaskResume is `URLSessionTask.resume()`: the request runs in the background and its
// completion handler hears how it ends, unless the task was cancelled first.
func NSURLSessionTaskResume(t URLSessionDataTask) {
	go func() {
		var result struct {
			F0 []byte
			F1 URLResponse
		}
		err := Catch(func() { result = NSURLSessionDataFor(t.session, t.request) })
		t.once.Do(func() {
			if err != nil {
				t.handler(nil, nil, err)
				return
			}
			t.handler(&result.F0, &result.F1, nil)
		})
	}()
}

// NSURLSessionTaskCancel is `URLSessionTask.cancel()`: the completion handler hears URLError.cancelled
// at once if the request has not completed; the request itself runs on, and its result is dropped.
func NSURLSessionTaskCancel(t URLSessionDataTask) {
	t.once.Do(func() {
		go t.handler(nil, nil, URLError{Code: URLErrorCancelled, Err: errors.New("cancelled")})
	})
}

// MemoryLayoutSize is `MemoryLayout<T>.size`, measured on T's Go form.
func MemoryLayoutSize[T any]() int { return int(unsafe.Sizeof(*new(T))) }

// MemoryLayoutStride is `MemoryLayout<T>.stride`.
func MemoryLayoutStride[T any]() int { return MemoryLayoutSize[T]() }

// MemoryLayoutAlignment is `MemoryLayout<T>.alignment`.
func MemoryLayoutAlignment[T any]() int { return int(unsafe.Alignof(*new(T))) }
