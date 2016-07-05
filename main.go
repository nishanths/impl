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

	"github.com/fatih/color"
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
		Format    string
		NoColor   bool
	}{}
	logger = log.New(os.Stderr, "impl: ", 0)
	green  = color.New(color.FgHiGreen, color.Bold).SprintFunc()
	yellow = color.New(color.FgHiYellow).SprintFunc()
)

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, usage)
		flag.PrintDefaults()
	}
	flag.StringVar(&arg.Path, "path", "", "absolute or relative path to directory to search")
	flag.StringVar(&arg.Interface, "interface", "", "interface name to find implementing types for, format: packageName.interfaceName")
	flag.StringVar(&arg.Format, "format", "plain", "output format, one of: {plain,json,xml}")
	flag.BoolVar(&arg.NoColor, "no-color", false, "disable color output")
	flag.Parse()

	if err := checkFlags(); err != nil {
		logger.Fatal(err)
	}

	mainImpl()
}

func mainImpl() {
	objects, err := getObjectsRecursive(arg.Path)
	if err != nil {
		logger.Fatal(err)
	}
	results := findImplementers(objects, arg.Interface)
	output(results, arg.Format)
}

func checkFlags() error {
	switch {
	case arg.Path == "":
		return errors.New(`must specify directory to search.
Run 'impl -h' for details.`)
	case len(strings.Split(arg.Interface, ".")) != 2:
		return errors.New(`must specify interface name in format: packageName.interfaceName.
Run 'impl -h' for details.`)
	case !contains([]string{"plain", "json", "xml"}, arg.Format):
		return errors.New(`output format should be one of: {plain,json,xml}`)
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

func findImplementers(objects []ObjectIdent, targetInterface string) []Result {
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
		res := Result{Interface: NewResultIdentifier(iface)}

		for _, obj := range objects {
			o := NewChar(obj)
			if seen[in][o] {
				// Seen this interface-object pair before.
				continue
			}
			seen[in][o] = true
			if intuitiveImplements(obj, iface) {
				res.Implementers = append(res.Implementers, NewResultIdentifier(obj))
			}
		}
		results = append(results, res)
	}

	return results
}

func output(res []Result, format string) {
	switch format {
	case "plain":
		for i, r := range res {
			fmt.Println(formatHeader(r.Interface))
			for _, ri := range r.Implementers {
				fmt.Println(formatImplementer(ri))
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

type Result struct {
	Interface    ResultIdentifier
	Implementers []ResultIdentifier
}

type ResultIdentifier struct {
	Name string
	Pos  token.Position
}

func NewResultIdentifier(o ObjectIdent) ResultIdentifier {
	return ResultIdentifier{
		Name: types.TypeString(o.Type(), nil),
		Pos:  o.FileSet.Position(o.Pos()),
	}
}

func formatHeader(r ResultIdentifier) string {
	if !arg.NoColor {
		r.Name = green(r.Name)
	}
	return fmt.Sprintf("%s: %s", r.Name, r.Pos)
}

func formatImplementer(r ResultIdentifier) string {
	if !arg.NoColor {
		r.Name = yellow(r.Name)
	}
	return fmt.Sprintf("  %s: %s", r.Name, r.Pos)
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

// getObjectsRecusrsive walks each directory in path and calls getObjects.
func getObjectsRecursive(path string) ([]ObjectIdent, error) {
	// Ensure provided path is directory.
	f, err := os.Open(path)
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

	filepath.Walk(path, func(path string, finfo os.FileInfo, err error) error {
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
