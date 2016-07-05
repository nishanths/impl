package main

import (
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// TOOD: Fix assertion locations. Currently it is not easy to find out exactly
// which type was missing/extra/different when there is a failure.

// TestableResults is used to add helper methods for testing.
type TestableResults []Result

// TestableExpect is a simple combination of a type name and filename where
// the type lives, the minimal needed for testing.
type TestableExpect struct {
	Name, Filename string
}

// Matches results in failed assertion if expected does not match output.
// Matches fails assertions if:
//
//   - The output has an item that is not present in expected.
//   - The output does not have an item that is present in expected.
//
func (tr TestableResults) Matches(expected ...TestableExpect) {
	es := make(map[TestableExpect]bool, len(expected))
	for _, e := range expected {
		es[e] = false
	}
	for _, r := range tr {
		for _, im := range r.Implementers {
			e := TestableExpect{im.Name, im.Pos.Filename}
			_, ok := es[e]
			So(ok, ShouldBeTrue) // fails when: output has an item not present in expected.
			es[e] = true
		}
	}
	for _, b := range es {
		So(b, ShouldBeTrue) // fails when: output does not have an item present in expected.
	}
}

func doTest(path, targetInterface string, concreteOnly bool) (TestableResults, error) {
	objects, err := getObjects(path)
	if err != nil {
		return nil, err
	}
	return TestableResults(findImplementers(objects, targetInterface, concreteOnly)), nil
}

func TestImpl(t *testing.T) {
	t.Parallel()

	Convey("impl", t, func() {
		Convey("directory", func() {
			Convey("all types", func() {
				tr, err := doTest(filepath.Join("internal", "testdata"), "testpkg.Foo", false)
				So(err, ShouldBeNil)
				tr.Matches(
					TestableExpect{"*testpkg.Zaphod", filepath.Join("internal", "testdata", "file1.go")},
					TestableExpect{"testpkg.Planet", filepath.Join("internal", "testdata", "file1.go")},
					TestableExpect{"testpkg.B", filepath.Join("internal", "testdata", "file2.go")},
				)
			})

			Convey("concrete types only", func() {
				tr, err := doTest(filepath.Join("internal", "testdata"), "testpkg.Foo", true)
				So(err, ShouldBeNil)
				tr.Matches(
					TestableExpect{"*testpkg.Zaphod", filepath.Join("internal", "testdata", "file1.go")},
				)
			})

		})

		Convey("file", func() {
			Convey("all types Foo", func() {
				tr, err := doTest(filepath.Join("internal", "testdata", "file1.go"), "testpkg.Foo", false)
				So(err, ShouldBeNil)
				tr.Matches(
					TestableExpect{"*testpkg.Zaphod", filepath.Join("internal", "testdata", "file1.go")},
					TestableExpect{"testpkg.Planet", filepath.Join("internal", "testdata", "file1.go")},
				)
			})

			Convey("all types Baz", func() {
				tr, err := doTest(filepath.Join("internal", "testdata", "file1.go"), "testpkg.Baz", false)
				So(err, ShouldBeNil)
				tr.Matches()
			})

			Convey("concrete types only Foo", func() {
				tr, err := doTest(filepath.Join("internal", "testdata", "file1.go"), "testpkg.Foo", true)
				So(err, ShouldBeNil)
				tr.Matches(
					TestableExpect{"*testpkg.Zaphod", filepath.Join("internal", "testdata", "file1.go")},
				)
			})

			Convey("concrete types only Baz", func() {
				tr, err := doTest(filepath.Join("internal", "testdata", "file1.go"), "testpkg.Baz", true)
				So(err, ShouldBeNil)
				tr.Matches()
			})
		})

		Convey("partial implementers", func() {
			Convey("combination DrinkerDiner", func() {
				tr, err := doTest(filepath.Join("internal", "testdata", "p1"), "p1.DrinkerDiner", false)
				So(err, ShouldBeNil)
				tr.Matches(
					TestableExpect{"*p1.Arthur", filepath.Join("internal", "testdata", "p1", "p1.go")},
				)
			})

			Convey("combination Diner", func() {
				tr, err := doTest(filepath.Join("internal", "testdata", "p1"), "p1.Diner1", false)
				So(err, ShouldBeNil)
				tr.Matches(
					TestableExpect{"*p1.Arthur", filepath.Join("internal", "testdata", "p1", "p1.go")},
					TestableExpect{"p1.DrinkerDiner", filepath.Join("internal", "testdata", "p1", "p1.go")},
					TestableExpect{"p1.Diner2", filepath.Join("internal", "testdata", "p1", "p1.go")},
				)
			})

			Convey("base type only", func() {
				tr, err := doTest(filepath.Join("internal", "testdata", "p2"), "p2.DrinkerDiner", false)
				So(err, ShouldBeNil)
				tr.Matches(
					TestableExpect{"p2.Arthur", filepath.Join("internal", "testdata", "p2", "p2.go")},
				)
			})
		})

		Convey("general", func() {
			Convey("embedded interface", func() {
				tr, err := doTest(filepath.Join("internal", "testdata", "p3"), "testpkg.Planet", false)
				So(err, ShouldBeNil)
				tr.Matches(
					TestableExpect{"testpkg.Human", filepath.Join("internal", "testdata", "p3", "p3.go")},
					TestableExpect{"testpkg.Landmass", filepath.Join("internal", "testdata", "p3", "p3.go")},
					TestableExpect{"testpkg.p", filepath.Join("internal", "testdata", "p3", "p3.go")},
					TestableExpect{"testpkg.Fjord", filepath.Join("internal", "testdata", "p3", "p3.go")},
					TestableExpect{"*testpkg.Fjord", filepath.Join("internal", "testdata", "p3", "p3.go")},
				)
			})

			Convey("none Human", func() {
				tr, err := doTest(filepath.Join("internal", "testdata", "p3"), "testpkg.Human", false)
				So(err, ShouldBeNil)
				tr.Matches()
			})

			Convey("embedded concrete", func() {
				tr, err := doTest(filepath.Join("internal", "testdata", "p3"), "testpkg.Landmass", false)
				So(err, ShouldBeNil)
				tr.Matches(
					TestableExpect{"*testpkg.Fjord", filepath.Join("internal", "testdata", "p3", "p3.go")},
				)
			})
		})
	})
}
