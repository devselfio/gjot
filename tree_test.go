package gjot

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
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
			if direct := appendJSON(nil, doc); !bytes.Equal(direct, want) {
				t.Errorf("input %d %q (positions=%v): json.Marshal and appendJSON differ\n got %s\nwant %s", i, in, len(opts) > 0, direct, want)
			}
			back, err := treeUnmarshalNode(want)
			if err != nil {
				t.Fatalf("input %d: treeUnmarshalNode: %v", i, err)
			}
			if again := appendJSON(nil, back); !bytes.Equal(again, want) {
				t.Errorf("input %d %q (positions=%v): decoded tree encodes differently\n got %s\nwant %s", i, in, len(opts) > 0, again, want)
			}
			var sec treeSection
			if err := json.Unmarshal(want, &sec); err != nil {
				t.Fatalf("input %d: json.Unmarshal into a section: %v", i, err)
			}
			if again := appendJSON(nil, &sec); !bytes.Equal(again, want) {
				t.Errorf("input %d %q (positions=%v): section decoded by json.Unmarshal encodes differently\n got %s\nwant %s", i, in, len(opts) > 0, again, want)
			}
		}
	}
}

// appendJSONString quotes as encoding/json does, HTML-safe escaping
// included.
func TestJSONStringEscaping(t *testing.T) {
	var cases []string
	for b := 0; b < 256; b++ {
		cases = append(cases, string([]byte{byte(b)}), "a"+string([]byte{byte(b)})+"b")
	}
	cases = append(cases,
		"",
		"\xe2\x80\xa8", "x\xe2\x80\xa9y", // U+2028, U+2029
		"\xff\xfe", "a\xc3", "\xe2\x82", "ok\xed\xa0\x80ok", // invalid UTF-8, a surrogate's encoding among it
		"<a&b>",
		"\xe2\x80\x98\xe2\x80\x99 \xc3\xa9 \xf0\x9d\x84\x9e \x7f", // curly quotes, e-acute, a 4-byte rune, DEL
		`"quoted" \back\slash`,
	)
	for _, s := range cases {
		want, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if got := appendJSONString(nil, s); !bytes.Equal(got, want) {
			t.Errorf("%q: got %s, want %s", s, got, want)
		}
	}
}

// A node whose tag is unknown or missing does not decode, at the top or
// among children; nor does a kind into another kind's type.
func TestUnmarshalNodeRejectsUnknownTag(t *testing.T) {
	for _, data := range []string{
		`{"tag":"paragraph"}`,
		`{"text":"a"}`,
		`{"tag":""}`,
		`{"tag":"para","children":[{"tag":"str","text":"a"},{"tag":"reference_definition"}]}`,
		`{"tag":"para","children":[{"text":"a"}]}`,
	} {
		if n, err := treeUnmarshalNode([]byte(data)); err == nil {
			t.Errorf("%s: decoded to %#v, want an error", data, n)
		}
	}
	var p treePara
	if err := json.Unmarshal([]byte(`{"tag":"str","text":"a"}`), &p); err == nil {
		t.Error("a str decoded into a para")
	}
}

// A document marshalled by an earlier gjot, with a "classes" key and an
// "auto_id" on a paragraph, decodes: the paragraph keeps what a paragraph
// has, and a heading keeps its auto_id.
func TestUnmarshalNodeIgnoresClasses(t *testing.T) {
	data := `{"tag":"section","children":[` +
		`{"tag":"heading","children":[{"tag":"str","text":"A"}],"level":1,"id":"A","auto_id":true},` +
		`{"tag":"para","children":[{"tag":"str","text":"b"}],"id":"p","classes":["x","y"],"attrs":[{"key":"class","value":"x y"}],"auto_id":true}` +
		`]}`
	n, err := treeUnmarshalNode([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	sec, ok := n.(*treeSection)
	if !ok || len(sec.Children) != 2 {
		t.Fatalf("decoded %#v", n)
	}
	h, ok := sec.Children[0].(*treeHeading)
	if !ok || !h.AutoID || h.Level != 1 || h.ID != "A" {
		t.Errorf("heading decoded as %#v", sec.Children[0])
	}
	p, ok := sec.Children[1].(*treePara)
	if !ok || p.ID != "p" || !p.HasClass("y") || p.Attr("class") != "x y" || len(p.Children) != 1 {
		t.Fatalf("para decoded as %#v", sec.Children[1])
	}
	got := string(appendJSON(nil, p))
	if want := `{"tag":"para","children":[{"tag":"str","text":"b"}],"id":"p","attrs":[{"key":"class","value":"x y"}]}`; got != want {
		t.Errorf("para re-encodes as\n %s\nwant\n %s", got, want)
	}
}

// treeWalk visits a node before its children, children in order, and skips
// a subtree when told to.
func TestWalkDocumentOrder(t *testing.T) {
	dest := "u"
	doc := &treeSection{Children: []treeNode{
		&treeHeading{Level: 1, Children: []treeNode{&treeStr{Text: "T"}}},
		&treePara{Children: []treeNode{
			&treeStr{Text: "a"},
			&treeEmph{Children: []treeNode{&treeStr{Text: "b"}, &treeSoftBreak{}}},
			&treeLink{Destination: &dest, Children: []treeNode{&treeStr{Text: "c"}}},
			&treeFootnoteReference{Label: "n"},
		}},
		&treeBulletList{Children: []treeNode{
			&treeListItem{Children: []treeNode{&treePara{Children: []treeNode{&treeStr{Text: "d"}}}}},
		}},
		&treeFootnote{Label: "n", Children: []treeNode{&treePara{Children: []treeNode{&treeStr{Text: "e"}}}}},
	}}
	name := func(n treeNode) string {
		if s, ok := n.(*treeStr); ok {
			return "str:" + s.Text
		}
		return string(n.Tag())
	}
	var got []string
	treeWalk(doc, func(n treeNode) WalkStatus {
		got = append(got, name(n))
		return WalkContinue
	})
	want := "section heading str:T para str:a emph str:b soft_break link str:c footnote_reference " +
		"bullet_list list_item para str:d footnote para str:e"
	if g := strings.Join(got, " "); g != want {
		t.Errorf("walk order\n got %s\nwant %s", g, want)
	}

	got = nil
	treeWalk(doc, func(n treeNode) WalkStatus {
		got = append(got, name(n))
		switch n.(type) {
		case *treeEmph, *treeBulletList:
			return WalkSkipChildren
		}
		return WalkContinue
	})
	want = "section heading str:T para str:a emph link str:c footnote_reference bullet_list footnote para str:e"
	if g := strings.Join(got, " "); g != want {
		t.Errorf("walk with skips\n got %s\nwant %s", g, want)
	}
}

// Every kind in the inventory is a distinct type whose Tag is its key, and
// linkFields reaches a link's and an image's fields.
func TestNodeKinds(t *testing.T) {
	if len(nodeKinds) != 49 {
		t.Errorf("%d kinds, want 49", len(nodeKinds))
	}
	for tag, mk := range nodeKinds {
		if got := mk().Tag(); got != tag {
			t.Errorf("kind %q makes a %q", tag, got)
		}
	}
	dest := "x"
	for _, n := range []treeNode{&treeLink{}, &treeImage{}} {
		d, r, _, ok := linkFields(n)
		if !ok {
			t.Fatalf("%s: linkFields not ok", n.Tag())
		}
		*d, *r = &dest, "ref"
		if got := string(appendJSON(nil, n)); !strings.Contains(got, `"destination":"x","reference":"ref"`) {
			t.Errorf("%s: linkFields did not write through: %s", n.Tag(), got)
		}
	}
	if _, _, _, ok := linkFields(&treeURL{}); ok {
		t.Error("linkFields ok on a url")
	}
}
