package gjot

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
)

// declaredTags reads node.go and returns every constant declared with type
// Tag, by name. It reads the source because Go cannot enumerate a type's
// constants at run time.
func declaredTags(t *testing.T) map[Tag]string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "node.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	tags := map[Tag]string{}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			if id, ok := vs.Type.(*ast.Ident); !ok || id.Name != "Tag" {
				continue
			}
			for i, name := range vs.Names {
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					t.Fatalf("Tag constant %s is not a string literal", name.Name)
				}
				v, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatal(err)
				}
				tags[Tag(v)] = name.Name
			}
		}
	}
	if len(tags) == 0 {
		t.Fatal("no Tag constants found in node.go")
	}
	return tags
}

// TestEveryTagIsParsed pins that the package declares no Tag that Parse never
// produces: each Tag constant in node.go must appear in the tree Parse builds
// for some input of the corpus (testdata/spec/*.test). A tag no input yields
// is dead public API and dead renderer branches; remove it, or add the
// corpus case that produces it.
func TestEveryTagIsParsed(t *testing.T) {
	files, err := filepath.Glob("testdata/spec/*.test")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no testdata/spec/*.test files found")
	}
	seen := map[Tag]bool{}
	var visit func(*Node)
	visit = func(n *Node) {
		seen[n.Tag] = true
		for _, c := range n.Children {
			visit(c)
		}
	}
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range parseSpecFile(string(content)) {
			visit(Parse([]byte(c.input)))
		}
	}
	var missing []string
	for tag, name := range declaredTags(t) {
		if !seen[tag] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	for _, name := range missing {
		t.Errorf("Tag %s is declared in node.go but no corpus input parses to it", name)
	}
}
