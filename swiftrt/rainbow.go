package swiftrt

import (
	"os"
	"strconv"
	"strings"
	"sync"
)

// rainbowSegment is a Rainbow `Rainbow.Segment`: a run of text with its parsed mode codes. color and
// background hold the codes that set them, or nil when unset.
type rainbowSegment struct {
	text       string
	color      []byte
	background []byte
	styles     []byte
}

func (s rainbowSegment) isPlain() bool {
	return s.color == nil && s.background == nil && (len(s.styles) == 0 || len(s.styles) == 1 && s.styles[0] == 0)
}

type rainbowCodes struct {
	color, background []byte
	styles            []byte
	stylesSet         bool
	resets            bool
}

func isRainbowColor(c byte) bool { return c >= 30 && c <= 37 || c == 39 || c >= 90 && c <= 97 }

func isRainbowBackground(c byte) bool { return c >= 40 && c <= 47 || c == 49 || c >= 100 && c <= 107 }

func isRainbowStyle(c byte) bool { return c <= 5 || c == 7 || c == 9 }

// parseRainbowCodes is Rainbow's `ConsoleCodesParser.parse(modeCodes:)`.
func parseRainbowCodes(codes []byte) rainbowCodes {
	var out rainbowCodes
	for i := 0; i < len(codes); i++ {
		code := codes[i]
		if code == 0 {
			out = rainbowCodes{styles: []byte{0}, stylesSet: true, resets: true}
			continue
		}
		if out.stylesSet && len(out.styles) == 1 && out.styles[0] == 0 {
			out.styles, out.stylesSet = nil, false
		}
		switch {
		case code == 38 || code == 48:
			var set []byte
			if i+2 < len(codes) && codes[i+1] == 5 {
				set = []byte{code, 5, codes[i+2]}
				i += 2
			} else if i+4 < len(codes) && codes[i+1] == 2 {
				set = []byte{code, 2, codes[i+2], codes[i+3], codes[i+4]}
				i += 4
			}
			if set != nil && code == 38 {
				out.color = set
			} else if set != nil {
				out.background = set
			}
		case isRainbowColor(code):
			out.color = []byte{code}
		case isRainbowBackground(code):
			out.background = []byte{code}
		case isRainbowStyle(code):
			out.styles = append(out.styles, code)
			out.stylesSet = true
		}
	}
	return out
}

func (s *rainbowSegment) update(in rainbowCodes, overwriteColor bool) {
	if in.resets {
		s.color, s.background, s.styles = nil, nil, nil
	}
	if s.isPlain() {
		s.styles = nil
	}
	if in.color != nil && (s.color == nil || overwriteColor) {
		s.color = in.color
	}
	if in.background != nil && (s.background == nil || overwriteColor) {
		s.background = in.background
	}
	s.styles = append(append([]byte{}, s.styles...), in.styles...)
}

// parseUInt8 is Swift `UInt8(_: String)`.
func parseUInt8(s string) (byte, bool) {
	s = strings.TrimPrefix(s, "+")
	if s == "" || s[0] == '-' && s != "-0" {
		return 0, false
	}
	v, err := strconv.ParseUint(strings.TrimPrefix(s, "-"), 10, 8)
	return byte(v), err == nil
}

// parseRainbowEntry is Rainbow's `ConsoleEntryParser.parse()`.
func parseRainbowEntry(text string) []rainbowSegment {
	runes := []rune(text)
	var segments []rainbowSegment
	var temp []byte
	appendSegment := func(t string) {
		c := parseRainbowCodes(temp)
		segments = append(segments, rainbowSegment{text: t, color: c.color, background: c.background, styles: c.styles})
		temp = nil
	}
	starting := false
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		switch {
		case starting && c == '[':
			temp = nil
			current := ""
			for i++; i < len(runes) && runes[i] != 'm'; i++ {
				if code, ok := parseUInt8(current); runes[i] == ';' && ok {
					temp = append(temp, code)
					current = ""
				} else {
					current += string(runes[i])
				}
			}
			if code, ok := parseUInt8(current); ok {
				temp = append(temp, code)
			}
			starting = false
		case c == 0x1b:
			if len(temp) > 0 {
				appendSegment("")
			}
			starting = true
		default:
			var b strings.Builder
			b.WriteRune(c)
			for i++; i < len(runes) && runes[i] != 0x1b; i++ {
				b.WriteRune(runes[i])
			}
			appendSegment(b.String())
			starting = true
		}
	}
	return segments
}

func envSet(key string) bool {
	v, ok := os.LookupEnv(key)
	return ok && v != "" && v != "0"
}

// rainbowEnabled is `Rainbow.enabled`: false when NO_COLOR is set unless FORCE_COLOR is.
var rainbowEnabled = sync.OnceValue(func() bool { return envSet("FORCE_COLOR") || !envSet("NO_COLOR") })

// rainbowConsole is `Rainbow.outputTarget == .console`: TERM is set and not "dumb", and standard output
// is a terminal.
var rainbowConsole = sync.OnceValue(func() bool {
	term, ok := os.LookupEnv("TERM")
	if !ok || strings.ToLower(term) == "dumb" {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	if null, err := os.Stat(os.DevNull); err == nil && os.SameFile(fi, null) {
		return false
	}
	return true
})

// rainbowGenerate is `Rainbow.generateString(for:)`.
func rainbowGenerate(segments []rainbowSegment, console bool) string {
	var b strings.Builder
	for _, s := range segments {
		if !console || s.isPlain() {
			b.WriteString(s.text)
			continue
		}
		codes := append(append(append([]byte{}, s.color...), s.background...), s.styles...)
		if len(codes) == 0 || s.text == "" {
			b.WriteString(s.text)
			continue
		}
		b.WriteString("\x1b[")
		for i, c := range codes {
			if i > 0 {
				b.WriteByte(';')
			}
			b.WriteString(strconv.Itoa(int(c)))
		}
		b.WriteString("m")
		b.WriteString(s.text)
		b.WriteString("\x1b[0m")
	}
	return b.String()
}

// rainbowApply is Rainbow's `String.applyingCodes(_:)`.
func rainbowApply(s string, codes []byte, enabled, console bool) string {
	if !enabled {
		return s
	}
	segments := parseRainbowEntry(s)
	in := parseRainbowCodes(codes)
	if len(segments) == 1 {
		segments[0].update(in, true)
	} else {
		for i := range segments {
			segments[i].update(in, false)
		}
	}
	return rainbowGenerate(segments, console)
}

func rainbow(s string, code byte) string {
	return rainbowApply(s, []byte{code}, rainbowEnabled(), rainbowConsole())
}

// StringBlack is Rainbow `String.black`.
func StringBlack(s string) string { return rainbow(s, 30) }

// StringRed is Rainbow `String.red`.
func StringRed(s string) string { return rainbow(s, 31) }

// StringGreen is Rainbow `String.green`.
func StringGreen(s string) string { return rainbow(s, 32) }

// StringYellow is Rainbow `String.yellow`.
func StringYellow(s string) string { return rainbow(s, 33) }

// StringBlue is Rainbow `String.blue`.
func StringBlue(s string) string { return rainbow(s, 34) }

// StringMagenta is Rainbow `String.magenta`.
func StringMagenta(s string) string { return rainbow(s, 35) }

// StringCyan is Rainbow `String.cyan`.
func StringCyan(s string) string { return rainbow(s, 36) }

// StringWhite is Rainbow `String.white`.
func StringWhite(s string) string { return rainbow(s, 37) }

// StringLightBlack is Rainbow `String.lightBlack`.
func StringLightBlack(s string) string { return rainbow(s, 90) }

// StringLightRed is Rainbow `String.lightRed`.
func StringLightRed(s string) string { return rainbow(s, 91) }

// StringLightGreen is Rainbow `String.lightGreen`.
func StringLightGreen(s string) string { return rainbow(s, 92) }

// StringLightYellow is Rainbow `String.lightYellow`.
func StringLightYellow(s string) string { return rainbow(s, 93) }

// StringLightBlue is Rainbow `String.lightBlue`.
func StringLightBlue(s string) string { return rainbow(s, 94) }

// StringLightMagenta is Rainbow `String.lightMagenta`.
func StringLightMagenta(s string) string { return rainbow(s, 95) }

// StringLightCyan is Rainbow `String.lightCyan`.
func StringLightCyan(s string) string { return rainbow(s, 96) }

// StringLightWhite is Rainbow `String.lightWhite`.
func StringLightWhite(s string) string { return rainbow(s, 97) }

// StringBold is Rainbow `String.bold`.
func StringBold(s string) string { return rainbow(s, 1) }

// StringDim is Rainbow `String.dim`.
func StringDim(s string) string { return rainbow(s, 2) }

// StringItalic is Rainbow `String.italic`.
func StringItalic(s string) string { return rainbow(s, 3) }

// StringUnderline is Rainbow `String.underline`.
func StringUnderline(s string) string { return rainbow(s, 4) }
