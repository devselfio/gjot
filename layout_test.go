package gjot

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

// TestRootPackageLayout holds the root to ADR 0018's rule: one non-test Go
// file, gjot.go, with the entry points. Build tags do not exempt a file;
// everything else lives in ast or internal/djot.
func TestRootPackageLayout(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		got = append(got, name)
	}
	if want := []string{"gjot.go"}; !slices.Equal(got, want) {
		t.Errorf("root non-test Go files = %v, want %v", got, want)
	}
}

// exportedDecls returns the exported top-level names declared in dir's
// non-test files, and the exported methods by receiver type ("Header.Attr").
func exportedDecls(t *testing.T, dir string) (names, methods []string) {
	t.Helper()
	fset := token.NewFileSet()
	matches, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range matches {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if !d.Name.IsExported() {
					continue
				}
				if d.Recv == nil {
					names = append(names, d.Name.Name)
					continue
				}
				recv := d.Recv.List[0].Type
				if star, ok := recv.(*ast.StarExpr); ok {
					recv = star.X
				}
				methods = append(methods, recv.(*ast.Ident).Name+"."+d.Name.Name)
			case *ast.GenDecl:
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.TypeSpec:
						if s.Name.IsExported() {
							names = append(names, s.Name.Name)
						}
					case *ast.ValueSpec:
						for _, n := range s.Names {
							if n.IsExported() {
								names = append(names, n.Name)
							}
						}
					}
				}
			}
		}
	}
	sort.Strings(names)
	sort.Strings(methods)
	return names, methods
}

// astKinds is the 49 node kinds the ast package declares.
var astKinds = []string{
	"Section", "Heading", "Para", "ThematicBreak", "CodeBlock", "RawBlock",
	"BlockQuote", "Div", "BulletList", "TaskList", "OrderedList",
	"DefinitionList", "ListItem", "TaskListItem", "DefinitionListItem",
	"Term", "Definition", "Table", "Caption", "Row", "Cell", "Footnote",
	"Str", "SoftBreak", "HardBreak", "NonBreakingSpace", "Emph", "Strong",
	"Span", "Mark", "Superscript", "Subscript", "Insert", "Delete",
	"DoubleQuoted", "SingleQuoted", "Verbatim", "InlineMath", "DisplayMath",
	"RawInline", "URL", "Email", "FootnoteReference", "Symb", "Link",
	"Image", "Ellipsis", "EmDash", "EnDash",
}

// TestPublicAPISurface pins the exported names of the root and of ast: the
// root declares only the entry points and their options (no node aliases),
// and ast the moved types, Walk, UnmarshalNode and the four accessors the
// split exported: HeaderOf, Children, Header.WrittenID and Header.ClassList.
func TestPublicAPISurface(t *testing.T) {
	root, rootMethods := exportedDecls(t, ".")
	wantRoot := []string{
		"ASTText", "Djot", "ErrTooDeep", "HTML", "HTMLOption", "Hook",
		"MaxNesting", "Parse", "ParseErr", "ParseOption", "WithHook",
		"WithSourcePos",
	}
	sort.Strings(wantRoot)
	if !slices.Equal(root, wantRoot) {
		t.Errorf("root exports\n got %v\nwant %v", root, wantRoot)
	}
	if len(rootMethods) != 0 {
		t.Errorf("root exports methods %v, want none", rootMethods)
	}

	names, methods := exportedDecls(t, "ast")
	wantNames := append([]string{
		"Node", "Header", "Tag", "Attr", "SourcePos", "Range", "CellAlign",
		"AlignDefault", "AlignLeft", "AlignRight", "AlignCenter",
		"WalkStatus", "WalkContinue", "WalkSkipChildren",
		"Walk", "UnmarshalNode", "HeaderOf", "Children",
	}, astKinds...)
	sort.Strings(wantNames)
	if !slices.Equal(names, wantNames) {
		t.Errorf("ast exports\n got %v\nwant %v", names, wantNames)
	}
	wantMethods := []string{
		"Header.Attr", "Header.ClassList", "Header.HasClass", "Header.WrittenID",
	}
	for _, k := range astKinds {
		wantMethods = append(wantMethods, k+".Tag", k+".MarshalJSON", k+".UnmarshalJSON")
	}
	sort.Strings(wantMethods)
	if !slices.Equal(methods, wantMethods) {
		t.Errorf("ast methods\n got %v\nwant %v", methods, wantMethods)
	}
}
