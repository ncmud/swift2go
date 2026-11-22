package swiftrt

import (
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// firstGrapheme splits the first grapheme cluster off s, threading uniseg's state between calls (-1 at the
// start). An ASCII byte followed by another ASCII byte other than CR LF is a cluster of its own, and the
// next cluster starts afresh, so ASCII text skips the Unicode segmentation tables.
func firstGrapheme(s string, state int) (cluster, rest string, newState int) {
	if len(s) > 0 && s[0] < 0x80 && (len(s) == 1 || s[1] < 0x80 && (s[0] != '\r' || s[1] != '\n')) {
		return s[:1], s[1:], -1
	}
	cluster, rest, _, newState = uniseg.FirstGraphemeClusterInString(s, state)
	return cluster, rest, newState
}

// graphemeCount is the number of grapheme clusters in s.
func graphemeCount(s string) int {
	n, state := 0, -1
	for len(s) > 0 {
		_, s, state = firstGrapheme(s, state)
		n++
	}
	return n
}

// graphemeRestart is the largest offset i <= at at which s certainly has a cluster boundary, or 0. It
// looks back for an ASCII byte that no cluster continues across: every one except the LF of a CR LF and
// a printable byte after a Prepend character. Segmenting from there with a fresh state gives the same
// clusters as segmenting from the start of s, so text with ASCII in it is segmented backwards in time
// proportional to the distance walked, not to the length of s.
func graphemeRestart(s string, at int) int {
	for i := at; i > 0; i-- {
		switch b := s[i]; {
		case b >= 0x80, b == '\n' && s[i-1] == '\r':
			continue
		case s[i-1] < 0x80:
			return i
		}
		_, size := utf8.DecodeLastRuneInString(s[:i])
		if c, _, _, _ := uniseg.FirstGraphemeClusterInString(s[i-size:i+1], -1); len(c) == size {
			return i
		}
	}
	return 0
}
