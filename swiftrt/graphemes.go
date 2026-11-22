package swiftrt

import "github.com/rivo/uniseg"

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
