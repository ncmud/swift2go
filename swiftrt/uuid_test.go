package swiftrt

import (
	"regexp"
	"testing"
)

func TestUUIDInitIsRandomVersion4(t *testing.T) {
	a, b := UUIDInit(), UUIDInit()
	if a == b {
		t.Fatalf("two UUIDs are equal: %v", a)
	}
	if a[6]>>4 != 4 || a[8]>>6 != 2 {
		t.Errorf("version/variant bits wrong: %x", a)
	}
}

func TestUUIDUuidStringIsUppercaseHyphenated(t *testing.T) {
	u := UUID{0xe6, 0x21, 0xe1, 0xf8, 0xc3, 0x6c, 0x49, 0x5a, 0x93, 0xfc, 0x0c, 0x24, 0x7a, 0x3e, 0x6e, 0x5f}
	const want = "E621E1F8-C36C-495A-93FC-0C247A3E6E5F"
	if got := UUIDUuidString(u); got != want {
		t.Errorf("UUIDUuidString = %q, want %q", got, want)
	}
	if got := UUIDInit().Description(); !regexp.MustCompile(`^[0-9A-F]{8}(-[0-9A-F]{4}){3}-[0-9A-F]{12}$`).MatchString(got) {
		t.Errorf("Description = %q", got)
	}
	back := UUIDOptInitUuidString(want)
	if back == nil || *back != u {
		t.Errorf("round trip = %v", back)
	}
}
