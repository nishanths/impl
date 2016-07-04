package main

import (
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
	usage = `Find types that implement a specified interface in go programs.

Example:
  impl -interface datastore.RawInterface -path ~/go/src/github.com/luci/gae

Usage:`
)

var (
	arg = struct {
		Path      string
		Interface string
	}{}
	logger = log.New(os.Stderr, "impl: ", 0)
)

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, usage)
		flag.PrintDefaults()
	}
	flag.StringVar(&arg.Path, "path", "", "absolute or relative path to directory to search")
	flag.StringVar(&arg.Interface, "interface", "", "interface name to find implementing types for, format: packageName.interfaceName")
	flag.Parse()
	if err := checkFlags(); err != nil {
		logger.Fatal(err)
	}
	mainImpl()
}

func checkFlags() error {
	switch {
	case arg.Path == "":
		return errors.New(`must specify directory to search.
Run 'impl -h' for details.`)
	case len(strings.Split(arg.Interface, ".")) != 2:
		return errors.New(`must specify interface name in format: packageName.interfaceName.
Run 'impl -h' for details.`)
	}
	return nil
}

// findDef returns position of the declared type for the supplied type.
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

func mainImpl() {
	objects, err := getObjectsRecursive(arg.Path)
	if err != nil {
		logger.Fatal(err)
	}

	interfaces := filterInterfaces(objects, arg.Interface)
	seen := make(map[Char]CharSet)

	for _, iface := range interfaces {
		i := NewChar(iface)
		if _, ok := seen[i]; ok {
			// Seen this interface before.
			continue
		}
		seen[i] = make(CharSet)
		printHeader(iface)

		for _, obj := range objects {
			o := NewChar(obj)
			if seen[i][o] {
				// Seen this interface-object pair before.
				continue
			}
			seen[i][o] = true
			if intuitiveImplements(obj, iface) {
				printImplementer(obj)
			}
		}
		fmt.Println()
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
// definition.
//
// Use NewChar to create a Char.
type Char struct {
	pkg      *types.Package
	typeName string
}

func NewChar(obj types.Object) Char {
	return Char{obj.Pkg(), types.TypeString(obj.Type(), nil)}
}

// CharSet is a set of Char.
type CharSet map[Char]bool

func printHeader(o ObjectIdent) {
	name := types.TypeString(o.Type(), nil)
	pos := o.FileSet.Position(o.Pos())
	fmt.Printf("%s (%s) is implemented by:\n", name, pos)
}

func printImplementer(o ObjectIdent) {
	name := types.TypeString(o.Type(), nil)
	pos := o.FileSet.Position(findDef(o.Type()))
	fmt.Printf("  %s: %s\n", pos, name)
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

// getObjectsRecusrsive walks each directory in path and calls getObjects.
func getObjectsRecursive(path string) ([]ObjectIdent, error) {
	// Ensure provided path is directory.
	f, err := os.Open(arg.Path)
	if err != nil {
		return nil, err
	}
	if info, err := f.Stat(); err != nil {
		return nil, err
	} else if !info.IsDir() {
		return nil, errors.New("path must be a directory.")
	}

	var (
		result    []ObjectIdent
		sharedChs []<-chan ObjectIdent
		errCh     = make(chan error)
	)

	filepath.Walk(arg.Path, func(path string, finfo os.FileInfo, err error) error {
		if !finfo.IsDir() {
			return nil
		}
		c := make(chan ObjectIdent)
		sharedChs = append(sharedChs, c)
		go func() {
			if err != nil {
				errCh <- err
				return
			}
			errCh <- getObjects(path, c)
		}()
		return nil
	})

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

// getObjects combines and sends a ObjectIdent for each types.Object
// whose ast.ObjKind==Typ found in the packages in the supplied path.
func getObjects(path string, ch chan<- ObjectIdent) error {
	fset := token.NewFileSet()
	m, err := parser.ParseDir(fset, path, nil, 0)
	if err != nil {
		return wrapErr("failed to parse directory", err)
	}

	var (
		sharedChs []<-chan ObjectIdent
		errCh     = make(chan error)
	)

	conf := &types.Config{
		IgnoreFuncBodies:         true,
		DisableUnusedImportCheck: true,
		Importer:                 importer.Default(),
	}

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
				close(ch)
				return nil
			}
			ch <- obj
		case err := <-errCh:
			if err != nil {
				return err
			}
		}
	}
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
		return wrapErr(`type-checks failed.
Make sure dependencies are completely installed.
See issue: https://github.com/golang/go/issues/9702.
`, err)
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
