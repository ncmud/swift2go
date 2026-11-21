package swiftrt

import "testing"

type regColor int

func TestRegistries(t *testing.T) {
	RegisterAllCases(func() []regColor { return []regColor{0, 1} })
	RegisterRawInit(func(r string) *regColor {
		if r == "red" {
			v := regColor(1)
			return &v
		}
		return nil
	})
	if len(AllCasesOf[regColor]()) != 2 {
		t.Fatal("allCases")
	}
	if p := RawInitOf[regColor]("red"); p == nil || *p != 1 || RawInitOf[regColor]("blue") != nil {
		t.Fatal("rawValue")
	}
}
