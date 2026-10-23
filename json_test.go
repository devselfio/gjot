package gjot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The JSON form of an AST carries everything the AST text does: marshal,
// unmarshal and re-render every corpus input, with and without positions.
func TestJSONRoundTrip(t *testing.T) {
	files, err := filepath.Glob("testdata/spec/*.test")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range parseSpecFile(string(content)) {
			for _, opts := range [][]ParseOption{nil, {WithSourcePos()}} {
				doc := Parse([]byte(c.input), opts...)
				data, err := json.Marshal(doc)
				if err != nil {
					t.Fatalf("%s:%d: %v", file, c.line, err)
				}
				var back Node
				if err := json.Unmarshal(data, &back); err != nil {
					t.Fatalf("%s:%d: %v", file, c.line, err)
				}
				want, got := string(ASTText(doc)), string(ASTText(&back))
				if got != want {
					t.Errorf("%s:%d (positions=%v)\n---before---\n%s---after---\n%s", file, c.line, len(opts) > 0, want, got)
				}
			}
		}
	}
}
