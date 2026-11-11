package gjot

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/devselfio/gjot/ast"
)

// goldenJSON is what the gjot before the swap to per-kind types (gjot#32,
// 498581e, its old Node struct through encoding/json) marshalled
// goldenSource to, parsed WithSourcePos: the wire form is pinned to it.
const (
	goldenSource = "# A\n\nb *c* [d](e)\n\n- [x] f\n\n[^n]: g\n"
	goldenJSON   = `{"tag":"section","children":[{"tag":"section","children":[{"tag":"heading","children":[{"tag":"str","text":"A","pos":{"start":{"line":1,"col":3,"offset":2},"end":{"line":1,"col":3,"offset":2}}}],"level":1,"pos":{"start":{"line":1,"col":1,"offset":0},"end":{"line":2,"col":0,"offset":3}}},{"tag":"para","children":[{"tag":"str","text":"b ","pos":{"start":{"line":3,"col":1,"offset":5},"end":{"line":3,"col":2,"offset":6}}},{"tag":"strong","children":[{"tag":"str","text":"c","pos":{"start":{"line":3,"col":4,"offset":8},"end":{"line":3,"col":4,"offset":8}}}],"pos":{"start":{"line":3,"col":3,"offset":7},"end":{"line":3,"col":5,"offset":9}}},{"tag":"str","text":" ","pos":{"start":{"line":3,"col":6,"offset":10},"end":{"line":3,"col":6,"offset":10}}},{"tag":"link","children":[{"tag":"str","text":"d","pos":{"start":{"line":3,"col":8,"offset":12},"end":{"line":3,"col":8,"offset":12}}}],"destination":"e","pos":{"start":{"line":3,"col":7,"offset":11},"end":{"line":3,"col":12,"offset":16}}}],"pos":{"start":{"line":3,"col":1,"offset":5},"end":{"line":4,"col":0,"offset":17}}},{"tag":"task_list","children":[{"tag":"task_list_item","children":[{"tag":"para","children":[{"tag":"str","text":"f","pos":{"start":{"line":5,"col":7,"offset":25},"end":{"line":5,"col":7,"offset":25}}}],"pos":{"start":{"line":5,"col":7,"offset":25},"end":{"line":6,"col":0,"offset":26}}}],"checked":true,"pos":{"start":{"line":5,"col":1,"offset":19},"end":{"line":7,"col":0,"offset":27}}}],"tight":true,"style":"-","pos":{"start":{"line":5,"col":1,"offset":19},"end":{"line":7,"col":1,"offset":28}}}],"level":1,"id":"A","auto_id":true,"pos":{"start":{"line":1,"col":1,"offset":0},"end":{"line":8,"col":0,"offset":35}}},{"tag":"footnote","children":[{"tag":"para","children":[{"tag":"str","text":"g","pos":{"start":{"line":7,"col":7,"offset":34},"end":{"line":7,"col":7,"offset":34}}}],"pos":{"start":{"line":7,"col":7,"offset":34},"end":{"line":8,"col":0,"offset":35}}}],"text":"n","pos":{"start":{"line":7,"col":1,"offset":28},"end":{"line":8,"col":0,"offset":35}}}]}`
)

// The tree's JSON is pinned to the old tree's (goldenJSON), and for every
// corpus input and the bench page, with and without positions, decoding it
// gives a tree that encodes to the same bytes again, through the package
// decoder and through json.Unmarshal into a section. (Before the swap this
// test compared every input with the old tree's JSON byte for byte.)
func TestTreeJSONIdentical(t *testing.T) {
	got, err := json.Marshal(Parse([]byte(goldenSource), WithSourcePos()))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != goldenJSON {
		t.Errorf("JSON of %q\n got %s\nwant %s", goldenSource, got, goldenJSON)
	}
	inputs := append(corpusInputs(t), readBenchPage(t))
	for i, in := range inputs {
		for _, opts := range [][]ParseOption{nil, {WithSourcePos()}} {
			doc := Parse(in, opts...)
			want, err := json.Marshal(doc)
			if err != nil {
				t.Fatalf("input %d: %v", i, err)
			}
			if direct := directJSON(t, doc); !bytes.Equal(direct, want) {
				t.Errorf("input %d %q (positions=%v): json.Marshal and the node's own MarshalJSON differ\n got %s\nwant %s", i, in, len(opts) > 0, direct, want)
			}
			back, err := ast.UnmarshalNode(want)
			if err != nil {
				t.Fatalf("input %d: UnmarshalNode: %v", i, err)
			}
			if again := directJSON(t, back); !bytes.Equal(again, want) {
				t.Errorf("input %d %q (positions=%v): decoded tree encodes differently\n got %s\nwant %s", i, in, len(opts) > 0, again, want)
			}
			var sec ast.Section
			if err := json.Unmarshal(want, &sec); err != nil {
				t.Fatalf("input %d: json.Unmarshal into a section: %v", i, err)
			}
			if again := directJSON(t, &sec); !bytes.Equal(again, want) {
				t.Errorf("input %d %q (positions=%v): section decoded by json.Unmarshal encodes differently\n got %s\nwant %s", i, in, len(opts) > 0, again, want)
			}
		}
	}
}

// directJSON is n's own MarshalJSON output, which encoding/json does not
// re-validate or compact as json.Marshal does.
func directJSON(t *testing.T, n ast.Node) []byte {
	t.Helper()
	out, err := n.(json.Marshaler).MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	return out
}
