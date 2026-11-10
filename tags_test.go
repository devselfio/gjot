package gjot

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// TestEveryTagIsParsed pins that the package declares no kind that Parse
// never produces: each kind in nodeKinds, the inventory the JSON decoder
// reads, must appear in the tree Parse builds for some input of the corpus
// (testdata/spec/*.test). A kind no input yields is dead public API and
// dead renderer branches; remove it, or add the corpus case that produces
// it. Every kind the corpus yields must be in nodeKinds too, or the decoder
// could not read it back.
func TestEveryTagIsParsed(t *testing.T) {
	files, err := filepath.Glob("testdata/spec/*.test")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no testdata/spec/*.test files found")
	}
	seen := map[Tag]bool{}
	visit := func(n treeNode) WalkStatus {
		seen[n.Tag()] = true
		return WalkContinue
	}
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range parseSpecFile(string(content)) {
			treeWalk(Parse([]byte(c.input)), visit)
		}
	}
	var missing, undeclared []string
	for tag := range nodeKinds {
		if !seen[tag] {
			missing = append(missing, string(tag))
		}
	}
	for tag := range seen {
		if _, ok := nodeKinds[tag]; !ok {
			undeclared = append(undeclared, string(tag))
		}
	}
	sort.Strings(missing)
	sort.Strings(undeclared)
	for _, tag := range missing {
		t.Errorf("kind %s is in nodeKinds but no corpus input parses to it", tag)
	}
	for _, tag := range undeclared {
		t.Errorf("the corpus parses to kind %s, which nodeKinds lacks", tag)
	}
}
