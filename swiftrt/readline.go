package swiftrt

import (
	"bufio"
	"os"
	"strings"
	"sync"
)

var stdin = sync.OnceValue(func() *bufio.Reader { return bufio.NewReader(os.Stdin) })

// ReadLine is Swift `readLine()`: the next line of standard input without its line ending, or nil at the
// end of input.
func ReadLine() *string { return ReadLineStrippingNewline(true) }

// ReadLineStrippingNewline is Swift `readLine(strippingNewline:)`: the next line of standard input,
// without its "\n" or "\r\n" when strip is true, or nil at the end of input. Invalid UTF-8 reads as
// U+FFFD.
func ReadLineStrippingNewline(strip bool) *string {
	line, err := stdin().ReadString('\n')
	if line == "" && err != nil {
		return nil
	}
	if strip && strings.HasSuffix(line, "\n") {
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	}
	line = strings.ToValidUTF8(line, "\uFFFD")
	return &line
}
