package swiftrt

import (
	"sync"
	"unsafe"
)

// Swift appends to a String in place, in amortised constant time; Go's s += t copies s. StringAppend
// gives Go strings Swift's growth: once a string is long enough, it lives at the start of a byte
// buffer with spare capacity, and appending to the string that ends at the buffer's high-water mark
// writes into the spare bytes and returns a longer header over the same buffer.
//
// The bytes below a buffer's high-water mark are never written again, and every string over the
// buffer ends at or below it, so no existing string changes. Any other string over the buffer (an
// older, shorter value, or a copy that has since been appended to) no longer ends at the mark and
// is copied into a buffer of its own instead.

// appendMinLen is the length below which StringAppend concatenates as Go does.
const appendMinLen = 64

// appendBufSlots is how many growable buffers are tracked at once. Each slot keeps its buffer alive
// until another buffer takes the slot; a string whose buffer has lost its slot is copied on its next
// append.
const appendBufSlots = 32

type appendBuf struct {
	data unsafe.Pointer // start of the buffer, which is also the start of the strings over it
	used int            // high-water mark: bytes written
	cap  int
}

type appendBufTable struct {
	sync.Mutex
	slots [appendBufSlots]appendBuf
	next  int
	last  int
}

var appendBufs appendBufTable

// StringAppend is Swift's s += t on a String.
func StringAppend(s *string, t string) {
	if t == "" {
		return
	}
	n := len(*s)
	if n+len(t) < appendMinLen {
		*s += t
		return
	}
	p := unsafe.Pointer(unsafe.StringData(*s)) //nolint:gosec // the high-water-mark invariant above
	b := &appendBufs
	b.Lock()
	if i := b.find(p); i >= 0 {
		sl := &b.slots[i]
		if sl.used == n && n+len(t) <= sl.cap {
			copy(unsafe.Slice((*byte)(p), sl.cap)[n:], t) //nolint:gosec // the high-water-mark invariant above
			sl.used += len(t)
			b.Unlock()
			*s = unsafe.String((*byte)(p), n+len(t)) //nolint:gosec // the high-water-mark invariant above
			return
		}
	}
	*s = b.grow(*s, t, 2*(n+len(t)))
	b.Unlock()
}

// StringReserveCapacityInt is String.reserveCapacity(_:).
func StringReserveCapacityInt(s *string, n int) {
	if n < appendMinLen || n <= len(*s) {
		return
	}
	b := &appendBufs
	b.Lock()
	if i := b.find(unsafe.Pointer(unsafe.StringData(*s))); i >= 0 && b.slots[i].used == len(*s) && b.slots[i].cap >= n { //nolint:gosec // the high-water-mark invariant above
		b.Unlock()
		return
	}
	*s = b.grow(*s, "", n)
	b.Unlock()
}

// find returns the slot of the buffer that starts at p, or -1.
func (b *appendBufTable) find(p unsafe.Pointer) int {
	if p == nil {
		return -1
	}
	if b.slots[b.last].data == p {
		return b.last
	}
	for i := range b.slots {
		if b.slots[i].data == p {
			b.last = i
			return i
		}
	}
	return -1
}

// grow copies s and t into a new buffer of at least capacity c, takes a slot for it and returns the
// string over it.
func (b *appendBufTable) grow(s, t string, c int) string {
	c = max(c, len(s)+len(t))
	buf := make([]byte, len(s)+len(t), c)
	copy(buf, s)
	copy(buf[len(s):], t)
	data := unsafe.Pointer(unsafe.SliceData(buf)) //nolint:gosec // the high-water-mark invariant above
	b.slots[b.next] = appendBuf{data: data, used: len(buf), cap: c}
	b.last = b.next
	b.next = (b.next + 1) % appendBufSlots
	return unsafe.String((*byte)(data), len(buf)) //nolint:gosec // the high-water-mark invariant above
}
