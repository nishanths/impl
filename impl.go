// Package main implements a command line tool that finds implementing types for
// a specified interface. Run 'impl -h' for details.
package main

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const (
	usage = `Find implementing types in go source code.

Examples:
  impl -interface discovery.SwaggerSchemaInterface -path ~/go/src/k8s.io/kubernetes/pkg/client/typed/discovery
  impl -interface datastore.RawInterface -path ./luci/gae/service/datastore -format json 

Flags:`
)

var (
	arg = struct {
		Path         string
		Interface    string
		Format       string
		ConcreteOnly bool
	}{}
	logger = log.New(os.Stderr, "impl: ", 0)
)

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, usage)
		flag.PrintDefaults()
	}
	flag.StringVar(&arg.Path, "path", "", "absolute or relative path to directory or file")
	flag.StringVar(&arg.Interface, "interface", "", "interface name to find implementing types for, format: packageName.interfaceName")
	flag.StringVar(&arg.Format, "format", "plain", "output format, should be one of: {plain,json,xml}")
	flag.BoolVar(&arg.ConcreteOnly, "concrete-only", false, "output concrete types only, by default the output contains both interface and concrete types that implement the specified interface")
	flag.Parse()

	if err := checkFlags(); err != nil {
		logger.Fatal(err)
	}

	mainImpl()
}

func mainImpl() {
	objects, err := getObjects(arg.Path)
	if err != nil {
		logger.Fatal(err)
	}
	results := findImplementers(objects, arg.Interface, arg.ConcreteOnly)
	output(results, arg.Format)
}

func checkFlags() error {
	switch {
	case arg.Path == "":
		return errors.New(`must specify directory to search (-path flag).
Run 'impl -h' for details.`)
	case len(strings.Split(arg.Interface, ".")) != 2:
		return errors.New(`must specify interface name in format: packageName.interfaceName (-interface flag).
Run 'impl -h' for details.`)
	case !contains([]string{"plain", "json", "xml"}, arg.Format):
		return errors.New(`output format should be one of: {plain,json,xml}
Run 'impl -h' for details.`)
	}
	return nil
}

// findDef returns position of the declared type for the supplied type.
// Specific for method receivers, since the Go spec does not allow them to be named
// pointers.
func findDef(typ types.Type) token.Pos {
	switch n := typ.(type) {
	case *types.Named:
		return n.Obj().Pos()
	case *types.Pointer:
		return findDef(n.Elem())
	default:
		return token.NoPos
	}
}

// findImplementers returns the ObjectIdents in the supplied objects that
// implement targetInterface. targetInterface should be of the form:
// packageName.InterfaceName.
func findImplementers(objects []ObjectIdent, targetInterface string, concreteOnly bool) []Result {
	interfaces := filterInterfaces(objects, targetInterface)
	seen := make(map[Char]CharSet)
	var results []Result

	for _, iface := range interfaces {
		in := NewChar(iface)
		if _, ok := seen[in]; ok {
			// Seen this interface before.
			continue
		}
		seen[in] = make(CharSet)
		res := Result{Interface: NewResultIdentifier(iface), Implementers: make([]ResultIdentifier, 0)}

		for _, obj := range objects {
			o := NewChar(obj)
			if seen[in][o] {
				// Seen this interface-object pair before.
				continue
			}
			seen[in][o] = true
			if concreteOnly && types.IsInterface(obj.Type()) {
				continue
			}
			if intuitiveImplements(obj, iface) {
				res.Implementers = append(res.Implementers, NewResultIdentifier(obj))
			}
		}
		results = append(results, res)
	}

	return results
}

// Output prints the Result list in the specified format.
func output(res []Result, format string) {
	switch format {
	case "plain":
		const sep = ": "
		// Aligned printing.
		longest := 0
		for _, r := range res {
			for _, ri := range r.Implementers {
				path := filepath.Base(ri.Pos.String())
				if len(path)+len(sep) > longest {
					longest = len(path) + len(sep)
				}
			}
		}
		for i, r := range res {
			if len(r.Implementers) == 0 {
				fmt.Println("No implementing types.")
			}
			for _, ri := range r.Implementers {
				path := filepath.Base(ri.Pos.String())
				fmt.Printf("%-*s%s\n", longest, path+sep, ri.Name)
			}
			if i != len(res)-1 {
				fmt.Println()
			}
		}
	case "json":
		b, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			logger.Fatal(err)
		}
		fmt.Printf("%s\n", b)
	case "xml":
		b, err := xml.MarshalIndent(res, "", "  ")
		if err != nil {
			logger.Fatal(err)
		}
		fmt.Printf("%s\n", b)
	}
}

// Result represents the final output of the program.
type Result struct {
	Interface    ResultIdentifier
	Implementers []ResultIdentifier
}

// ResultIdentifier is details about the elements in Result.
// Use NewResultIdentifier to create a ResultIdentifier from an ObjectIdent.
type ResultIdentifier struct {
	Name string
	Pos  token.Position
}

// NewResultIdentifier creates a ResultIdentifier from o.
func NewResultIdentifier(o ObjectIdent) ResultIdentifier {
	return ResultIdentifier{
		Name: types.TypeString(o.Type(), nil),
		Pos:  o.FileSet.Position(findDef(o.Type())),
	}
}

// ObjectIdent is a combination of types.Object, *ast.Ident, and
// *token.FileSet.
type ObjectIdent struct {
	types.Object
	Ident   *ast.Ident
	FileSet *token.FileSet
}

// Char is the set of characteristics required to determine if two identifiers
// are the same: they are from the same package and have the same type name.
// For Char objects a and b, if a==b then a and b are the same according to the above
// definition. Char comparisons on Char created using NewChar work even in
// for packages structured like:
//
//  foo/
//    bar/ --> package bar declares type Baz
//  qux/
//    bar/ --> package bar declares type Baz
//
// In the above case, Chars for the two bar.Baz's will not be ==, because
// Char uses pointers to types.Package objects in its implementation.
//
// Use NewChar to create a Char.
type Char struct {
	pkg      *types.Package
	typeName string
}

// NewChar creates a Char from the supplied types.Object.
func NewChar(obj types.Object) Char {
	return Char{obj.Pkg(), types.TypeString(obj.Type(), nil)}
}

// CharSet is a set of Char.
type CharSet map[Char]bool

// contains returns whether list contains target.
func contains(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}

// filterInterfaces returns the interface types in objs whose
// packageName.interfaceName==name.
func filterInterfaces(objs []ObjectIdent, name string) (ifaces []ObjectIdent) {
	for _, o := range objs {
		typ := o.Type()
		if types.IsInterface(typ) && types.TypeString(typ, nil) == name {
			ifaces = append(ifaces, o)
		}
	}
	return
}

// intuitiveImplements is similar to types.Implements, except that it returns
// false if obj and iface are types with the same name in the same package.
func intuitiveImplements(obj types.Object, iface types.Object) bool {
	if NewChar(obj) == NewChar(iface) {
		return false
	}
	return types.Implements(obj.Type(), iface.Type().Underlying().(*types.Interface))
}

// getObjects combines and sends a ObjectIdent for each types.Object
// whose ast.ObjKind==Typ found in the package in the supplied path.
func getObjects(path string) ([]ObjectIdent, error) {
	fset := token.NewFileSet()
	m, err := parsePath(path, fset)
	if err != nil {
		return nil, err
	}

	conf := &types.Config{
		IgnoreFuncBodies:         true,
		DisableUnusedImportCheck: true,
		Importer:                 importer.Default(),
	}
	errCh := make(chan error)
	var sharedChs []<-chan ObjectIdent
	var result []ObjectIdent

	for _, tree := range m {
		c := make(chan ObjectIdent)
		sharedChs = append(sharedChs, c)
		go func(tree *ast.Package) {
			errCh <- getObjectsPkg(tree, conf, fset, c)
		}(tree)
	}

	finalCh := converge(sharedChs)
	for {
		select {
		case obj, ok := <-finalCh:
			if !ok {
				return result, nil
			}
			result = append(result, obj)
		case err := <-errCh:
			if err != nil {
				return nil, err
			}
		}
	}
}

// parsePath parses the directory or file specified by path and returns the
// AST of packages.
func parsePath(path string, fset *token.FileSet) (map[string]*ast.Package, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	var pkgs map[string]*ast.Package // Assigned in either of two branches below.

	if info.IsDir() {
		pkgs, err = parser.ParseDir(fset, path, nil, 0)
		if err != nil {
			return nil, wrapErr("failed to parse directory", err)
		}
	} else {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil, wrapErr("failed to parse file", err)
		}
		pkg := &ast.Package{
			Name: file.Name.Name,
			Files: map[string]*ast.File{
				filepath.Base(path): file,
			},
		}
		pkgs = map[string]*ast.Package{pkg.Name: pkg}
	}

	return pkgs, nil
}

func getObjectsPkg(tree *ast.Package, conf *types.Config, fset *token.FileSet, ch chan<- ObjectIdent) error {
	info := types.Info{
		Defs: make(map[*ast.Ident]types.Object),
	}

	pkgName := tree.Name
	files := make([]*ast.File, 0, len(tree.Files))
	for _, f := range tree.Files {
		files = append(files, f)
	}

	if _, err := conf.Check(pkgName, fset, files, &info); err != nil {
		// Related: https://github.com/golang/go/issues/9702
		return wrapErr(`type-checks failed. Make sure dependencies are completely installed`, err)
	}

	go func() {
		defer close(ch)
		for ident, obj := range info.Defs {
			if obj == nil || ident.Obj == nil {
				continue
			}
			ch <- ObjectIdent{obj, ident, fset}
		}
	}()

	return nil
}
