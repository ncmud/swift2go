package types

import "testing"

func TestNormalizeLocal(t *testing.T) {
	cases := map[string]string{
		"Foo #1 in Mod.Type.fn() -> ()": "Mod.Foo",
		"Entry #1 in static World.DirectionsEngine.findShortestPaths(from: Swift.Int, wm: World.WorldManager) -> [Swift.Int : Swift.Int]": "World.Entry",
		"[Swift.String : [Appearance #1 in static CombatModel.LoadoutSearch.droppingSurplusCopies([CombatModel.LoadoutSearchProblem.Slot]) -> [CombatModel.LoadoutSearchProblem.Slot]]]": "[Swift.String : [CombatModel.Appearance]]",
		"Swift.Int": "Swift.Int",
	}
	for in, want := range cases {
		if got := normalizeLocal(in); got != want {
			t.Errorf("normalizeLocal(%q) = %q, want %q", in, got, want)
		}
	}
}
