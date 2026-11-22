package swiftrt

import (
	"crypto/hmac"
	"crypto/sha256"
	"math/bits"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// StringRemovingPercentEncoding is Foundation `String.removingPercentEncoding`: every %XX escape
// decoded, or nil when an escape is malformed or the bytes are not UTF-8.
func StringRemovingPercentEncoding(s string) *string {
	if !strings.Contains(s, "%") {
		return &s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			b.WriteByte(s[i])
			continue
		}
		if i+2 >= len(s) || !isHex(s[i+1]) || !isHex(s[i+2]) {
			return nil
		}
		v, _ := strconv.ParseUint(s[i+1:i+3], 16, 8)
		b.WriteByte(byte(v))
		i += 2
	}
	out := b.String()
	if !utf8.ValidString(out) {
		return nil
	}
	return &out
}

// CharacterSetInverted is Foundation `CharacterSet.inverted`.
func CharacterSetInverted(cs CharacterSet) CharacterSet { return func(r rune) bool { return !cs(r) } }

// StringRangeOfCharacterFrom is Foundation `String.rangeOfCharacter(from:)`: the byte range of the
// first scalar in the set, or nil.
func StringRangeOfCharacterFrom(s string, cs CharacterSet) *Range[int] {
	for i, r := range s {
		if cs(r) {
			return &Range[int]{Lo: i, Hi: i + utf8.RuneLen(r)}
		}
	}
	return nil
}

// NSNumberStringValue is Foundation `NSNumber.stringValue`: Booleans as 1 or 0, integers in
// decimal and doubles as their shortest round-trip form.
func NSNumberStringValue(n NSNumber) string {
	if n.kind == 'd' {
		return strconv.FormatFloat(n.f, 'g', -1, 64)
	}
	return n.Description()
}

// URLComponentsQueryItems is Foundation `URLComponents.queryItems`.
func URLComponentsQueryItems(c URLComponents) *[]URLQueryItem { return c.QueryItems }

// URLComponentsFragment is Foundation `URLComponents.fragment`, percent-decoded.
func URLComponentsFragment(c URLComponents) *string {
	if c.fragment == nil {
		return nil
	}
	f := percentDecodeLenient(*c.fragment)
	return &f
}

// URLQueryItemName is Foundation `URLQueryItem.name`.
func URLQueryItemName(q URLQueryItem) string { return q.Name }

// URLQueryItemValue is Foundation `URLQueryItem.value`.
func URLQueryItemValue(q URLQueryItem) *string { return q.Value }

// HashedAuthenticationCode is CryptoKit `HashedAuthenticationCode`: the MAC's bytes.
type HashedAuthenticationCode []byte

// HMACAuthenticationCodeForUsing is CryptoKit `HMAC<SHA256>.authenticationCode(for:using:)`. The
// generated call does not carry the hash function; it is SHA-256, the one ncmud uses.
func HMACAuthenticationCodeForUsing(data []byte, key SymmetricKey) HashedAuthenticationCode {
	mac := hmac.New(sha256.New, key.bytes)
	mac.Write(data)
	return mac.Sum(nil)
}

// DataInitHashedAuthenticationCode is `Data(_:)` over a MAC's bytes.
func DataInitHashedAuthenticationCode(mac HashedAuthenticationCode) []byte { return slices.Clone(mac) }

// DataMap is `Data.map(_:)`.
func DataMap[T any](d []byte, f func(uint8) T) []T {
	out := make([]T, len(d))
	for i, b := range d {
		out[i] = f(b)
	}
	return out
}

// DataWithUnsafeBytes is Foundation `Data.withUnsafeBytes(_:)`: body receives the bytes as an
// UnsafeRawBufferPointer.
func DataWithUnsafeBytes[R any](d []byte, body func(any) R) R { return body(d) }

// UnsafeRawBufferPointerCount is `UnsafeRawBufferPointer.count`.
func UnsafeRawBufferPointerCount(p any) int { return len(p.([]byte)) }

// UnsafeRawBufferPointerBaseAddress is `UnsafeRawBufferPointer.baseAddress`: the bytes themselves,
// which C calls taking a pointer accept; nil when there are none.
func UnsafeRawBufferPointerBaseAddress(p any) any {
	if b := p.([]byte); len(b) > 0 {
		return b
	}
	return nil
}

// SliceInitData is `Array(_: Data)`: a copy of the bytes.
func SliceInitData[S ~[]uint8](d []byte) S { return S(slices.Clone(d)) }

// SetInitAny is `Set(_:)` over a sequence the runtime holds as a slice.
func SetInitAny[S ~map[T]struct{}, T comparable](xs []T) S { return SetInitSlice[S](xs) }

// SetInitLazyFilterSequence is `Set(_:)` over a lazy filter.
func SetInitLazyFilterSequence[S ~map[T]struct{}, T comparable](xs []T) S { return SetInitSlice[S](xs) }

// SetForEach is `Set.forEach(_:)`, in an unspecified order as Swift's.
func SetForEach[T comparable](s map[T]struct{}, f func(T)) {
	for v := range s {
		f(v)
	}
}

// StringReduce is `String.reduce(_:_:)` over the string's Characters.
func StringReduce[A any](s string, initial A, f func(A, Character) A) A {
	acc := initial
	for _, c := range StringChars(s) {
		acc = f(acc, c)
	}
	return acc
}

// StringInitAny is `String(_:)` over a value that is LosslessStringConvertible: its description.
func StringInitAny(v any) string { return Str(v) }

// AnyTrailingZeroBitCount is `BinaryInteger.trailingZeroBitCount` on an integer of a type parameter.
func AnyTrailingZeroBitCount(v any) int {
	var u uint64
	width := 64
	switch n := v.(type) {
	case int:
		u = uint64(n)
	case int8:
		u, width = uint64(uint8(n)), 8
	case int16:
		u, width = uint64(uint16(n)), 16
	case int32:
		u, width = uint64(uint32(n)), 32
	case int64:
		u = uint64(n)
	case uint:
		u = uint64(n)
	case uint8:
		u, width = uint64(n), 8
	case uint16:
		u, width = uint64(n), 16
	case uint32:
		u, width = uint64(n), 32
	case uint64:
		u = n
	default:
		panic("swiftrt: trailingZeroBitCount of " + Str(v))
	}
	if u == 0 {
		return width
	}
	return bits.TrailingZeros64(u)
}

// TaskPriorityUserInitiated and the other priorities are Swift's `TaskPriority` values. Tasks here
// run without priorities, so they only stand for the value.
var (
	TaskPriorityHigh          any = TaskPriority(25)
	TaskPriorityUserInitiated any = TaskPriority(25)
	TaskPriorityMedium        any = TaskPriority(21)
	TaskPriorityLow           any = TaskPriority(17)
	TaskPriorityUtility       any = TaskPriority(17)
	TaskPriorityBackground    any = TaskPriority(9)
)

// TaskPriority is Swift's `TaskPriority`.
type TaskPriority uint8

// TaskDetachedPriorityOperation is `Task.detached(priority:operation:)`.
func TaskDetachedPriorityOperation(_ *any, op any) *TaskHandle { return startTask(nil, op) }

// TaskInitPriorityOperation is `Task(priority:operation:)`.
func TaskInitPriorityOperation(_ *any, op any) *TaskHandle { return TaskInitOperation(op) }

// TaskSleepNanoseconds is `Task.sleep(nanoseconds:)`: it throws CancellationError if the current
// task is cancelled before or while it sleeps.
func TaskSleepNanoseconds(ns uint64) { TaskSleepFor(time.Duration(min(ns, uint64(1<<63-1)))) }

// NSFileHandleInitForReadingFrom is Foundation `FileHandle(forReadingFrom:)`: it throws when the
// file cannot be opened.
func NSFileHandleInitForReadingFrom(u URL) FileHandle {
	path := fileURLPath(u)
	f, err := os.Open(path)
	if err != nil {
		panic(Throw(cocoaFileError(path, err)))
	}
	return FileHandle{File: f}
}

// The `Substring.UTF8View` functions. The runtime holds the view as its bytes, in an any, and
// indexes it from 0.

// UTF8ViewLast is `UTF8View.last`.
func UTF8ViewLast(v any) *uint8 {
	b := v.([]byte)
	if len(b) == 0 {
		return nil
	}
	return &b[len(b)-1]
}

// UTF8ViewLastIndexOf is `UTF8View.lastIndex(of:)`.
func UTF8ViewLastIndexOf(v any, x uint8) *int {
	b := v.([]byte)
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] == x {
			return &i
		}
	}
	return nil
}

// UTF8ViewSubscriptPartialRangeUpTo is `view[..<i]`.
func UTF8ViewSubscriptPartialRangeUpTo(v any, r PartialRangeUpTo[int]) any { return v.([]byte)[:r.Hi] }

// UTF8ViewSuffixInt is `UTF8View.suffix(_:)`.
func UTF8ViewSuffixInt(v any, n int) any {
	b := v.([]byte)
	if n < 0 {
		Trap("Can't take a suffix of negative length from a collection")
	}
	return b[len(b)-min(n, len(b)):]
}

// UTF8ViewElementsEqual is `UTF8View.elementsEqual(_:)`.
func UTF8ViewElementsEqual(v any, other []uint8) bool { return slices.Equal(v.([]byte), other) }
