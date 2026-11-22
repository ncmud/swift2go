package exports

const Limit = 4

type Kind int

const (
	KindA Kind = iota
	kindB
)

func Exported() {}

func unexported() {}

func (Kind) Method() {}

type Box[K comparable, V any] struct {
	Items map[K]V
}
