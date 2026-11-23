package swiftrt

import "testing"

type witnessNamer interface{ StaticName() string }

type witnessShout struct{}

func (witnessShout) StaticName() string { return "shout" }

type witnessCount struct{}

func (witnessCount) Count() int { return 3 }

// A type can carry several conformances: registering a second carrier keeps the first.
func TestRegisterWitnessKeepsEachCarrier(t *testing.T) {
	type word struct{ _ string }
	RegisterWitness(func(*word) any { return witnessShout{} })
	RegisterWitness(func(*word) any { return witnessCount{} })
	if got := StaticWitness[*word, witnessNamer]().StaticName(); got != "shout" {
		t.Fatalf("StaticWitness: %q", got)
	}
	if c, ok := witnessAs[interface{ Count() int }](&word{}); !ok || c.Count() != 3 {
		t.Fatal("second carrier")
	}
}
