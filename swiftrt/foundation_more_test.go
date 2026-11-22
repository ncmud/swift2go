package swiftrt

import (
	"encoding/hex"
	"testing"
)

func TestRemovingPercentEncoding(t *testing.T) {
	for in, want := range map[string]string{"a%20b": "a b", "plain": "plain", "%E2%9C%93": "✓", "a+b": "a+b"} {
		if got := StringRemovingPercentEncoding(in); got == nil || *got != want {
			t.Errorf("%q -> %v, want %q", in, got, want)
		}
	}
	for _, in := range []string{"%", "%2", "%zz", "%FF"} {
		if got := StringRemovingPercentEncoding(in); got != nil {
			t.Errorf("%q decoded to %q, want nil", in, *got)
		}
	}
}

func TestHMACAuthenticationCode(t *testing.T) {
	mac := HMACAuthenticationCodeForUsing([]byte("The quick brown fox jumps over the lazy dog"), SymmetricKeyInitData([]byte("key")))
	if got := hex.EncodeToString(DataInitHashedAuthenticationCode(mac)); got != "f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8" {
		t.Errorf("HMAC-SHA256 = %s", got)
	}
	if !HMACIsValidAuthenticationCodeAuthenticatingUsing(mac, []byte("The quick brown fox jumps over the lazy dog"), SymmetricKeyInitData([]byte("key"))) {
		t.Error("the code does not verify")
	}
}

func TestUTF8ViewAndBits(t *testing.T) {
	var v any = []byte("ab{x}\n\r")
	if i := UTF8ViewLastIndexOf(v, '{'); i == nil || *i != 2 {
		t.Errorf("lastIndex = %v", i)
	}
	if !UTF8ViewElementsEqual(UTF8ViewSuffixInt(v, 2), []uint8{'\n', '\r'}) {
		t.Error("suffix")
	}
	if AnyTrailingZeroBitCount(uint32(8)) != 3 || AnyTrailingZeroBitCount(int8(0)) != 8 {
		t.Error("trailingZeroBitCount")
	}
	if NSNumberStringValue(NSNumberDouble(1.5)) != "1.5" || NSNumberStringValue(NSNumberBool(true)) != "1" {
		t.Error("stringValue")
	}
}

func TestLocalizedCompareCollates(t *testing.T) {
	// Results recorded from Swift's Foundation on macOS.
	for _, c := range []struct {
		a, b         string
		ci, loc, std int
	}{
		{"-=FM=- Clubhouse", "(TIC) Clubhouse", -1, -1, -1},
		{"a", "A", 0, -1, -1},
		{"e", "é", -1, -1, -1},
		{"10", "9", -1, -1, 1},
		{"ab", "a b", 1, 1, 1},
		{"Zone", "zone", 0, 1, 1},
		{"_x", "-x", -1, -1, -1},
		{"[a]", "(a)", 1, 1, 1},
		{"*", "#", -1, -1, -1},
	} {
		if got := StringLocalizedCaseInsensitiveCompare(c.a, c.b); got != c.ci {
			t.Errorf("localizedCaseInsensitiveCompare(%q, %q) = %d, want %d", c.a, c.b, got, c.ci)
		}
		if got := StringLocalizedCompare(c.a, c.b); got != c.loc {
			t.Errorf("localizedCompare(%q, %q) = %d, want %d", c.a, c.b, got, c.loc)
		}
		if got := StringLocalizedStandardCompare(c.a, c.b); got != c.std {
			t.Errorf("localizedStandardCompare(%q, %q) = %d, want %d", c.a, c.b, got, c.std)
		}
	}
}
