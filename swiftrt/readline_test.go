package swiftrt

import (
	"bufio"
	"strings"
	"testing"
)

func TestReadLineStrippingNewline(t *testing.T) {
	input := "one\r\ntwo\nbad \xff\nlast"
	read := func(strip bool) []string {
		reader := bufio.NewReader(strings.NewReader(input))
		saved := stdin
		stdin = func() *bufio.Reader { return reader }
		defer func() { stdin = saved }()
		var lines []string
		for line := ReadLineStrippingNewline(strip); line != nil; line = ReadLineStrippingNewline(strip) {
			lines = append(lines, *line)
		}
		return lines
	}
	for strip, want := range map[bool][]string{
		true:  {"one", "two", "bad �", "last"},
		false: {"one\r\n", "two\n", "bad �\n", "last"},
	} {
		if got := read(strip); strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("strippingNewline %v: %q, want %q", strip, got, want)
		}
	}
}
