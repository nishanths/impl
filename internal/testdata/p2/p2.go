package p2

/// Arthur

type Arthur int

func (a Arthur) Drink() {}
func (a Arthur) Dine() int {
	return 42
}

/// Interfaces

type DrinkerDiner interface {
	Drink()
	Diner1
}

type Diner1 interface {
	Dine() int
}

type Diner2 interface {
	Dine() int
}
