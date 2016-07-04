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

var (
	arg = struct {
		Path      string
		Interface string
	}{}
	logger *log.Logger
	usage  = `Find types that implement a specified interface in go programs.

Example:
  impl -interface datastore.RawInterface -path ~/go/src/github.com/luci/gae

Usage:`
)

func main() {
	logger = log.New(os.Stderr, "impl: ", 0)
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

func mainImpl() {
	objects, err := getObjectsRecursive(arg.Path)
	if err != nil {
		logger.Fatal(err)
	}

	interfaces := filterInterfaces(objects, arg.Interface)
	seen := make(map[Char]CharSet)

	for _, iface := range interfaces {
		i := Char{iface.Pkg(), iface.Name()}
		if _, ok := seen[i]; ok {
			// Seen this interface before.
			continue
		}
		seen[i] = make(CharSet)
		printHeader(iface)

		for _, obj := range objects {
			o := Char{obj.Pkg(), obj.Name()}
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

type Char struct {
	Package *types.Package
	Name    string
}

type CharSet map[Char]bool

func sameObj(a, b types.Object) bool {
	return Char{a.Pkg(), a.Name()} == Char{b.Pkg(), b.Name()}
}

func printHeader(i interface{})      {}
func printImplementer(i interface{}) {}

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
	if sameObj(obj, iface) {
		return false
	}
	return types.Implements(obj.Type(), iface.Type().Underlying().(*types.Interface))
}

func getObjectsRecursive(path string) ([]ObjectIdent, error) {
	// Ensure provided path is directory.
	f, err := os.Open(arg.Path)
	if err != nil {
		return nil, err
	}
	if info, err := f.Stat(); err != nil {
		return nil, err
	} else if !info.IsDir() {
		return nil, errors.New("path should be a directory.")
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
	defer close(ch)
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
		close(ch)
		return wrapErr(`type-checks failed.
		Make sure dependencies are completely installed.
		See issue: https://github.com/golang/go/issues/9702`, err)
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
