package swiftrt

import "testing"

type ckoEligibility struct {
	MinLevel         int      `swift:"minLevel"`
	RequiresAdvanced bool     `swift:"requiresAdvanced" key:"advanced"`
	MaxLevel         *int     `swift:"maxLevel"`
	Prerequisites    []string `swift:"prerequisites"`
}

func (ckoEligibility) SwiftCodingKeyOrder() []string {
	return []string{"minLevel", "maxLevel", "prerequisites", "advanced"}
}

func TestSynthesizedEncodeFollowsCodingKeysOrder(t *testing.T) {
	got := string(YYJSONEncoderEncode(YYJSONEncoderInit(), ckoEligibility{MinLevel: 1, RequiresAdvanced: true, Prerequisites: []string{"a"}}))
	if want := `{"minLevel":1,"prerequisites":["a"],"advanced":true}`; got != want {
		t.Errorf("encoded %s, want %s", got, want)
	}
}
