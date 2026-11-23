package swiftrt

import (
	"fmt"
	"reflect"
	"slices"
	"time"
	"unsafe"
)

// An unsafe buffer pointer is carried as the Go slice of its elements, which shares their memory.
// A pointer to bytes is a `*any` holding such a slice or, for a base address, a `*uint8` at its
// first element.

// pointee follows the `*any` boxes around a pointer's memory.
func pointee(p any) any {
	for {
		box, ok := p.(*any)
		if !ok {
			return p
		}
		if box == nil {
			Trap("unexpectedly found nil while unwrapping an Optional value")
		}
		p = *box
	}
}

// OpaquePointerInitUnsafeRawPointer is Swift `OpaquePointer(_: UnsafeRawPointer)`: the same address.
func OpaquePointerInitUnsafeRawPointer(p any) any { return p }

// OpaquePointerInitUnsafeMutablePointer is Swift `OpaquePointer(_: UnsafeMutablePointer<T>)`.
func OpaquePointerInitUnsafeMutablePointer(p any) any { return p }

// UnsafePointerInitOpaquePointer is Swift `UnsafePointer<T>(_: OpaquePointer)`.
func UnsafePointerInitOpaquePointer(p any) any { return p }

// UnsafeRawPointerInitUnsafeRawPointer is Swift `UnsafeRawPointer(_:)` on a typed or raw pointer.
func UnsafeRawPointerInitUnsafeRawPointer(p any) any { return p }

// UnsafeRawPointerAssumingMemoryBoundTo is Swift `UnsafeRawPointer.assumingMemoryBound(to:)`: a
// pointer to the first element of the memory, read as T. The memory must hold T, or bytes when T
// is a one-byte integer.
func UnsafeRawPointerAssumingMemoryBoundTo[T any](p any, _ TypeT[T]) *T {
	switch v := pointee(p).(type) {
	case *T:
		return v
	case []T:
		return unsafe.SliceData(v) //nolint:gosec // a pointer to the slice's own first element
	case *[]T:
		return unsafe.SliceData(*v) //nolint:gosec // a pointer to the slice's own first element
	}
	if reflect.TypeFor[T]().Size() == 1 {
		switch v := pointee(p).(type) {
		case []byte:
			return (*T)(unsafe.Pointer(unsafe.SliceData(v))) //nolint:gosec // reinterprets bytes as a one-byte type
		case []int8:
			return (*T)(unsafe.Pointer(unsafe.SliceData(v))) //nolint:gosec // reinterprets bytes as a one-byte type
		case string:
			return (*T)(unsafe.Pointer(unsafe.StringData(v))) //nolint:gosec // reinterprets bytes as a one-byte type
		case *uint8:
			return (*T)(unsafe.Pointer(v)) //nolint:gosec // reinterprets bytes as a one-byte type
		case *int8:
			return (*T)(unsafe.Pointer(v)) //nolint:gosec // reinterprets bytes as a one-byte type
		}
	}
	panic(fmt.Sprintf("swiftrt: cannot bind %T memory to %v", pointee(p), reflect.TypeFor[T]()))
}

// UnsafeBufferPointerInitStartCount is Swift `UnsafeBufferPointer(start:count:)`: the first count
// elements at a pointer. It traps when the memory behind the pointer is shorter.
func UnsafeBufferPointerInitStartCount(start any, count int) any {
	if count < 0 {
		Trap("UnsafeBufferPointer with negative count")
	}
	switch v := pointee(start).(type) {
	case string:
		if count > len(v) {
			Trap("UnsafeBufferPointer reaches past the end of its memory")
		}
		return []byte(v[:count])
	case *uint8:
		return unsafe.Slice(v, count) //nolint:gosec // the caller vouches for count elements at v, as in C
	case *int8:
		return unsafe.Slice(v, count) //nolint:gosec // the caller vouches for count elements at v, as in C
	default:
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Pointer && !rv.IsNil() && rv.Elem().Kind() == reflect.Slice {
			rv = rv.Elem()
		}
		if rv.Kind() != reflect.Slice {
			panic(fmt.Sprintf("swiftrt: %T is not a pointer to elements", v))
		}
		if count > rv.Len() {
			Trap("UnsafeBufferPointer reaches past the end of its memory")
		}
		return rv.Slice(0, count).Interface()
	}
}

func bufferValue(buf any) reflect.Value {
	rv := reflect.ValueOf(pointee(buf))
	if rv.Kind() != reflect.Slice {
		panic(fmt.Sprintf("swiftrt: %T is not an unsafe buffer pointer", buf))
	}
	return rv
}

// UnsafeBufferPointerCount is Swift `UnsafeBufferPointer.count`.
func UnsafeBufferPointerCount(buf any) int { return bufferValue(buf).Len() }

// UnsafeMutableRawBufferPointerCount is Swift `UnsafeMutableRawBufferPointer.count`.
func UnsafeMutableRawBufferPointerCount(buf any) int { return bufferValue(buf).Len() }

// UnsafeBufferPointerBaseAddress is Swift `UnsafeBufferPointer.baseAddress` on a buffer of
// one-byte integers: a pointer to the first, or nil for an empty buffer. Buffers of other
// elements are not supported and panic.
func UnsafeBufferPointerBaseAddress(buf any) **uint8 {
	var base *uint8
	switch v := pointee(buf).(type) {
	case []byte:
		base = unsafe.SliceData(v) //nolint:gosec // a pointer to the slice's own first element
	case []int8:
		base = (*uint8)(unsafe.Pointer(unsafe.SliceData(v))) //nolint:gosec // reinterprets C characters as bytes
	default:
		panic(fmt.Sprintf("swiftrt: baseAddress of a %T buffer is not supported", v))
	}
	if base == nil || bufferValue(buf).Len() == 0 {
		return nil
	}
	return &base
}

// UnsafeMutableRawBufferPointerBaseAddress is Swift `UnsafeMutableRawBufferPointer.baseAddress`:
// the buffer's bytes, as the C functions take a pointer to them.
func UnsafeMutableRawBufferPointerBaseAddress(buf any) any { return pointee(buf) }

// UnsafeMutableBufferPointerBaseAddress is Swift `UnsafeMutableBufferPointer.baseAddress`: a
// pointer whose pointee holds the buffer's elements, as the C functions take one.
func UnsafeMutableBufferPointerBaseAddress(buf any) *any {
	held := bufferValue(buf).Interface()
	return &held
}

// UnsafeBufferPointerPrefixWhile is Swift `UnsafeBufferPointer.prefix(while:)`.
func UnsafeBufferPointerPrefixWhile[T any](buf any, f func(T) bool) []T {
	return SlicePrefixWhile(pointee(buf).([]T), f)
}

// SliceInitUnsafeBufferPointer is Swift `Array(_: UnsafeBufferPointer)`: a copy of the buffer's
// elements. A buffer of one-byte integers converts to an array of another one-byte integer type,
// as rebinding its memory would.
func SliceInitUnsafeBufferPointer[S ~[]E, E any](buf any) S {
	if xs, ok := pointee(buf).([]E); ok {
		return S(slices.Clone(xs))
	}
	rv := bufferValue(buf)
	elem := reflect.TypeFor[E]()
	from := rv.Type().Elem()
	if elem.Size() != 1 || from.Size() != 1 || !from.ConvertibleTo(elem) {
		panic(fmt.Sprintf("swiftrt: cannot read a %v buffer as %v", rv.Type(), elem))
	}
	out := make(S, rv.Len())
	for i := range out {
		out[i] = rv.Index(i).Convert(elem).Interface().(E)
	}
	return out
}

// SliceWithUnsafeBufferPointer is Swift `Array.withUnsafeBufferPointer(_:)`: body receives a
// buffer over the array's elements.
func SliceWithUnsafeBufferPointer[T, R any](xs []T, body func(any) R) R { return body(xs) }

// SliceWithUnsafeMutableBufferPointer is Swift `Array.withUnsafeMutableBufferPointer(_:)`: body
// receives a buffer over the array's elements, and what it writes through the buffer is written
// to the array.
func SliceWithUnsafeMutableBufferPointer[T, R any](xs *[]T, body func(*any) R) R {
	var buf any = Mut(xs)
	return body(&buf)
}

// SliceWithUnsafeMutableBytes is Swift `Array.withUnsafeMutableBytes(_:)`: body receives the
// array's memory as bytes, and what it writes there is written to the array. The elements must be
// numbers; other element types are not supported and panic.
func SliceWithUnsafeMutableBytes[T, R any](xs *[]T, body func(any) R) R {
	Mut(xs)
	if b, ok := any(*xs).([]byte); ok {
		return body(b)
	}
	elem := reflect.TypeFor[T]()
	switch elem.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
	default:
		panic(fmt.Sprintf("swiftrt: withUnsafeMutableBytes over %v is not supported", elem))
	}
	return body(unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(*xs))), len(*xs)*int(elem.Size()))) //nolint:gosec // views numbers as their bytes
}

// CTm is the C `struct tm`.
type CTm struct {
	Tm_sec    int32 //nolint:revive // C field name
	Tm_min    int32 //nolint:revive // C field name
	Tm_hour   int32 //nolint:revive // C field name
	Tm_mday   int32 //nolint:revive // C field name
	Tm_mon    int32 //nolint:revive // C field name
	Tm_year   int32 //nolint:revive // C field name
	Tm_wday   int32 //nolint:revive // C field name
	Tm_yday   int32 //nolint:revive // C field name
	Tm_isdst  int32 //nolint:revive // C field name
	Tm_gmtoff int   //nolint:revive // C field name
	Tm_zone   *any  //nolint:revive // C field name
}

// CTmInit is `tm()`, all zero.
func CTmInit() CTm { return CTm{} }

// CFLocaltime is C `localtime(_:)`: the calendar time of a `time_t` in the local time zone, in
// storage of its own.
func CFLocaltime(clock *int) **CTm {
	t := time.Unix(int64(*clock), 0).In(time.Local)
	name, offset := t.Zone()
	tm := &CTm{
		Tm_sec: int32(t.Second()), Tm_min: int32(t.Minute()), Tm_hour: int32(t.Hour()),
		Tm_mday: int32(t.Day()), Tm_mon: int32(t.Month()) - 1, Tm_year: int32(t.Year() - 1900),
		Tm_wday: int32(t.Weekday()), Tm_yday: int32(t.YearDay() - 1),
		Tm_gmtoff: offset, Tm_zone: cStringResult(name),
	}
	if t.IsDST() {
		tm.Tm_isdst = 1
	}
	return &tm
}

var (
	cWeekdays = [...]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	cMonths   = [...]string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
)

// isoWeek is the ISO 8601 week number and week-based year of a `struct tm`.
func isoWeek(tm *CTm) (week, year int) {
	year = int(tm.Tm_year) + 1900
	monday := (int(tm.Tm_wday) + 6) % 7
	week = (int(tm.Tm_yday) - monday + 10) / 7
	leap := func(y int) bool { return y%4 == 0 && (y%100 != 0 || y%400 == 0) }
	weeksIn := func(y int) int {
		// A year has 53 weeks when 1 January is a Thursday, or a Wednesday in a leap year.
		jan1 := time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC).Weekday()
		if jan1 == time.Thursday || jan1 == time.Wednesday && leap(y) {
			return 53
		}
		return 52
	}
	switch {
	case week < 1:
		year--
		week = weeksIn(year)
	case week > weeksIn(year):
		year++
		week = 1
	}
	return week, year
}

// strftime expands a C `strftime` format in the C locale. A conversion it does not know is
// copied as written.
func strftime(format string, tm *CTm) string {
	var out []byte
	pad := func(v, width int, fill byte) {
		s := fmt.Sprintf("%d", v)
		for n := len(s); n < width; n++ {
			out = append(out, fill)
		}
		out = append(out, s...)
	}
	name := func(names []string, i int32, short bool) {
		if i < 0 || int(i) >= len(names) {
			out = append(out, '?')
			return
		}
		if short {
			out = append(out, names[i][:3]...)
			return
		}
		out = append(out, names[i]...)
	}
	hour12 := func() int {
		if h := int(tm.Tm_hour) % 12; h != 0 {
			return h
		}
		return 12
	}
	year := int(tm.Tm_year) + 1900
	for i := 0; i < len(format); i++ {
		if format[i] != '%' || i+1 == len(format) {
			out = append(out, format[i])
			continue
		}
		start := i
		i++
		if (format[i] == 'E' || format[i] == 'O') && i+1 < len(format) {
			i++
		}
		switch format[i] {
		case 'a':
			name(cWeekdays[:], tm.Tm_wday, true)
		case 'A':
			name(cWeekdays[:], tm.Tm_wday, false)
		case 'b', 'h':
			name(cMonths[:], tm.Tm_mon, true)
		case 'B':
			name(cMonths[:], tm.Tm_mon, false)
		case 'c':
			out = append(out, strftime("%a %b %e %H:%M:%S %Y", tm)...)
		case 'C':
			pad(year/100, 2, '0')
		case 'd':
			pad(int(tm.Tm_mday), 2, '0')
		case 'D', 'x':
			out = append(out, strftime("%m/%d/%y", tm)...)
		case 'e':
			pad(int(tm.Tm_mday), 2, ' ')
		case 'F':
			out = append(out, strftime("%Y-%m-%d", tm)...)
		case 'g':
			_, y := isoWeek(tm)
			pad(y%100, 2, '0')
		case 'G':
			_, y := isoWeek(tm)
			pad(y, 1, '0')
		case 'H':
			pad(int(tm.Tm_hour), 2, '0')
		case 'I':
			pad(hour12(), 2, '0')
		case 'j':
			pad(int(tm.Tm_yday)+1, 3, '0')
		case 'k':
			pad(int(tm.Tm_hour), 2, ' ')
		case 'l':
			pad(hour12(), 2, ' ')
		case 'm':
			pad(int(tm.Tm_mon)+1, 2, '0')
		case 'M':
			pad(int(tm.Tm_min), 2, '0')
		case 'n':
			out = append(out, '\n')
		case 'p':
			if tm.Tm_hour < 12 {
				out = append(out, "AM"...)
			} else {
				out = append(out, "PM"...)
			}
		case 'r':
			out = append(out, strftime("%I:%M:%S %p", tm)...)
		case 'R':
			out = append(out, strftime("%H:%M", tm)...)
		case 's':
			local := time.Date(year, time.Month(tm.Tm_mon+1), int(tm.Tm_mday), int(tm.Tm_hour), int(tm.Tm_min), int(tm.Tm_sec), 0, time.Local)
			out = append(out, fmt.Sprintf("%d", local.Unix())...)
		case 'S':
			pad(int(tm.Tm_sec), 2, '0')
		case 't':
			out = append(out, '\t')
		case 'T', 'X':
			out = append(out, strftime("%H:%M:%S", tm)...)
		case 'u':
			pad((int(tm.Tm_wday)+6)%7+1, 1, '0')
		case 'U':
			pad((int(tm.Tm_yday)+7-int(tm.Tm_wday))/7, 2, '0')
		case 'V':
			w, _ := isoWeek(tm)
			pad(w, 2, '0')
		case 'w':
			pad(int(tm.Tm_wday), 1, '0')
		case 'W':
			pad((int(tm.Tm_yday)+7-(int(tm.Tm_wday)+6)%7)/7, 2, '0')
		case 'y':
			pad(((year%100)+100)%100, 2, '0')
		case 'Y':
			pad(year, 1, '0')
		case 'z':
			off, sign := tm.Tm_gmtoff/60, byte('+')
			if off < 0 {
				off, sign = -off, '-'
			}
			out = append(out, sign)
			pad(off/60*100+off%60, 4, '0')
		case 'Z':
			if tm.Tm_zone != nil {
				out = append(out, StringInitCString(tm.Tm_zone)...)
			}
		case '%':
			out = append(out, '%')
		default:
			out = append(out, format[start:i+1]...)
		}
	}
	return string(out)
}

// CFStrftime is C `strftime(_:_:_:_:)` in the C locale: it writes the formatted time and a NUL
// into the buffer and returns the length written, or 0, writing nothing, when maxsize bytes are
// too few to hold both.
func CFStrftime(buf any, maxsize int, format *any, tm *CTm) int {
	text := strftime(cStringArg(format), tm)
	bytes, flush := cBuffer(buf, maxsize)
	if len(text)+1 > len(bytes) {
		return 0
	}
	n := copy(bytes, text)
	bytes[n] = 0
	flush(n + 1)
	return n
}

// UnsafeMutableRawPointerOptInitBitPattern is `UnsafeMutableRawPointer(bitPattern:)`: nil for zero, and
// otherwise a pointer value that compares equal to another made from the same bits.
func UnsafeMutableRawPointerOptInitBitPattern(bits int) *any {
	if bits == 0 {
		return nil
	}
	p := any(uintptr(bits))
	return &p
}
