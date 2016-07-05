package testpkg

/// Interfaces

type Planet interface {
	Exist()
	Bar()
	Baz(qux int)
}

type Human interface {
	Planet
	Speak(s string)
}

type Landmass interface {
	Planet
	Form(y int) error
}

type p Planet

/// Fjord

type Fjord struct {
	p
}

func (f *Fjord) Form(a int) error { return nil }
