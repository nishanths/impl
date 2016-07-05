package testpkg

/// Interfaces

type Foo interface {
	Exist()
	Bar()
	Baz(qux int)
}

type Planet interface {
	Exist()
	Bar()
	Baz(qux int)
}

type Baz interface {
	Crazy()
}

/// Zaphod

type Zaphod struct{}

func (s *Zaphod) Bar()           {}
func (k *Zaphod) Baz(a int)      {}
func (z *Zaphod) Exist()         {}
func (z *Zaphod) Speak(s string) {}
