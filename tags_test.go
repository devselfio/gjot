package gjot

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/devselfio/gjot/ast"
)

// TestEveryTagIsParsed pins that package ast declares no kind that Parse
// never produces, and that the decoder reads every kind Parse does: the
// trees Parse builds for the corpus inputs (testdata/spec/*.test) hold
// exactly 49 distinct kinds, and each decodes through ast.UnmarshalNode to
// a node of its tag. ast's TestNodeKinds proves its inventory, the one the
// decoder reads, is 49 distinct kinds, so the two sets are the same. A kind
// no input yields is dead public API and dead renderer branches; remove it,
// or add the corpus case that produces it.
func TestEveryTagIsParsed(t *testing.T) {
	files, err := filepath.Glob("testdata/spec/*.test")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no testdata/spec/*.test files found")
	}
	seen := map[ast.Tag]bool{}
	visit := func(n ast.Node) ast.WalkStatus {
		seen[n.Tag()] = true
		return ast.WalkContinue
	}
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range parseSpecFile(string(content)) {
			ast.Walk(Parse([]byte(c.input)), visit)
		}
	}
	var tags, undeclared []string
	for tag := range seen {
		tags = append(tags, string(tag))
		n, err := ast.UnmarshalNode([]byte(`{"tag":"` + string(tag) + `"}`))
		if err != nil || n.Tag() != tag {
			undeclared = append(undeclared, string(tag))
		}
	}
	sort.Strings(tags)
	sort.Strings(undeclared)
	if len(tags) != 49 {
		t.Errorf("the corpus parses to %d kinds, want all 49 ast declares: %v", len(tags), tags)
	}
	for _, tag := range undeclared {
		t.Errorf("the corpus parses to kind %s, which ast.UnmarshalNode does not decode", tag)
	}
}
