package gjot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
				wantHTML, _ := HTML(doc)
				gotHTML, _ := HTML(&back)
				if string(gotHTML) != string(wantHTML) {
					t.Errorf("%s:%d (positions=%v) HTML\n---before---\n%s---after---\n%s", file, c.line, len(opts) > 0, wantHTML, gotHTML)
				}
				checkSourceOnlyOnQuotes(t, fmt.Sprintf("%s:%d", file, c.line), doc)
			}
		}
	}
}

// A tree decoded from JSON renders and writes back as the parsed tree does:
// a smart quote's source travels as "source", which image alt text, the
// writer and labels read. Every other node leaves the key out.
func TestJSONKeepsSmartQuoteSource(t *testing.T) {
	for _, src := range []string{
		"![Don't](u)\n",
		"![\"x\"](u)\n",
		"![a 'b' \"c](u)\n",
		"# Don't panic\n\n[Don't panic][]\n",
		"# '{.c}\n\n['][]\n",
	} {
		doc := Parse([]byte(src))
		data, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		var back Node
		if err := json.Unmarshal(data, &back); err != nil {
			t.Fatal(err)
		}
		want, _ := HTML(doc)
		got, _ := HTML(&back)
		if string(got) != string(want) {
			t.Errorf("%q: HTML through JSON\n got %s\nwant %s", src, got, want)
		}
		if w, g := string(Djot(doc)), string(Djot(&back)); g != w {
			t.Errorf("%q: Djot through JSON\n got %q\nwant %q", src, g, w)
		}
	}
	data, _ := json.Marshal(Parse([]byte("![Don't](u) a\n")))
	if want := `{"tag":"str","text":"’","source":"'"}`; !strings.Contains(string(data), want) {
		t.Errorf("JSON lacks %s:\n%s", want, data)
	}
	if n := strings.Count(string(data), `"source"`); n != 1 {
		t.Errorf("JSON has %d source keys, want 1:\n%s", n, data)
	}
}

// checkSourceOnlyOnQuotes fails unless every node with a Source is a Str
// holding one smart-quote glyph written as ' or ".
func checkSourceOnlyOnQuotes(t *testing.T, where string, n *Node) {
	t.Helper()
	if n.Source != "" {
		single := n.Source == "'" && (n.Text == "‘" || n.Text == "’")
		double := n.Source == "\"" && (n.Text == "“" || n.Text == "”")
		if n.Tag != Str || !single && !double {
			t.Errorf("%s: Source %q on %v %q", where, n.Source, n.Tag, n.Text)
		}
	}
	for _, c := range n.Children {
		checkSourceOnlyOnQuotes(t, where, c)
	}
}
