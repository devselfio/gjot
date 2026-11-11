package ast

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

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
		if n, err := UnmarshalNode([]byte(data)); err == nil {
			t.Errorf("%s: decoded to %#v, want an error", data, n)
		}
	}
	var p Para
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
	n, err := UnmarshalNode([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	sec, ok := n.(*Section)
	if !ok || len(sec.Children) != 2 {
		t.Fatalf("decoded %#v", n)
	}
	h, ok := sec.Children[0].(*Heading)
	if !ok || !h.AutoID || h.Level != 1 || h.ID != "A" {
		t.Errorf("heading decoded as %#v", sec.Children[0])
	}
	p, ok := sec.Children[1].(*Para)
	if !ok || p.ID != "p" || !p.HasClass("y") || p.Attr("class") != "x y" || len(p.Children) != 1 {
		t.Fatalf("para decoded as %#v", sec.Children[1])
	}
	got := string(appendJSON(nil, p))
	if want := `{"tag":"para","children":[{"tag":"str","text":"b"}],"id":"p","attrs":[{"key":"class","value":"x y"}]}`; got != want {
		t.Errorf("para re-encodes as\n %s\nwant\n %s", got, want)
	}
}

// appendJSON, which every kind's MarshalJSON is, writes what encoding/json
// writes for the node through it (valid, compact, HTML-escaped JSON): for
// every kind's zero value, for every kind with its fields, attributes and
// position set, and for a nested tree. gjot's TestTreeJSONIdentical checks
// parsed trees against the pinned wire form.
func TestAppendJSONMatchesMarshal(t *testing.T) {
	dest := "<u&v>"
	pos := &Range{Start: SourcePos{Line: 1, Col: 2, Offset: 1}, End: SourcePos{Line: 3, Col: 4, Offset: 9}}
	full := func(n Node) Node {
		h := HeaderOf(n)
		h.ID, h.Pos = "i<d>", pos
		h.Attrs = []Attr{{Key: "id", Value: ""}, {Key: "class", Value: "a b"}, {Key: "k", Value: "\"v\" "}}
		kids := []Node{&Str{Text: "x"}, &Emph{Children: []Node{&Str{Text: "‘", Source: "'"}}}}
		switch n := n.(type) {
		case *Section:
			n.Level, n.AutoID, n.Children = 2, true, kids
		case *Heading:
			n.Level, n.AutoID, n.Children = 3, true, kids
		case *CodeBlock:
			n.Lang, n.Text = "go", "a\tb\n"
		case *RawBlock:
			n.Format, n.Text = "html", "<b>"
		case *BulletList:
			n.Style, n.Tight, n.Children = "-", true, kids
		case *TaskList:
			n.Style, n.Tight, n.Children = "+", true, kids
		case *OrderedList:
			n.Style, n.Marker, n.Tight, n.Children = "(a)", "iv", true, kids
		case *TaskListItem:
			n.Checked, n.Children = true, kids
		case *Row:
			n.Head, n.Children = true, kids
		case *Cell:
			n.Head, n.Align, n.Children = true, AlignCenter, kids
		case *Footnote:
			n.Label, n.Children = "n", kids
		case *Str:
			n.Text, n.Source = "“", "\""
		case *Verbatim:
			n.Text = "a & b"
		case *InlineMath:
			n.Text = "x^2"
		case *DisplayMath:
			n.Text = "y"
		case *RawInline:
			n.Format, n.Text = "html", "<i>"
		case *URL:
			n.Text = "http://e/?a&b"
		case *Email:
			n.Text = "a@b"
		case *FootnoteReference:
			n.Label = "n"
		case *Symb:
			n.Alias = "smile"
		case *Link:
			n.Destination, n.Reference, n.Children = &dest, "r", kids
		case *Image:
			n.Destination, n.Reference, n.Children = &dest, "r", kids
		default:
			if hasChildrenField(n) {
				setChildren(n, kids)
			}
		}
		return n
	}
	check := func(name string, n Node) {
		t.Helper()
		want, err := json.Marshal(n)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := appendJSON(nil, n); !bytes.Equal(got, want) {
			t.Errorf("%s: appendJSON and json.Marshal differ\n got %s\nwant %s", name, got, want)
		}
		back, err := UnmarshalNode(want)
		if err != nil {
			t.Fatalf("%s: UnmarshalNode: %v", name, err)
		}
		if again := appendJSON(nil, back); !bytes.Equal(again, want) {
			t.Errorf("%s: decoded node encodes differently\n got %s\nwant %s", name, again, want)
		}
		if hasChildrenField(n) && len(Children(n)) != len(Children(back)) {
			t.Errorf("%s: decoded node has %d children, want %d", name, len(Children(back)), len(Children(n)))
		}
	}
	for tag, mk := range nodeKinds {
		check(string(tag)+" (zero)", mk())
		f := full(mk())
		if hasChildrenField(f) && len(Children(f)) == 0 {
			t.Errorf("%s (full): container fixture has no children", tag)
		}
		check(string(tag)+" (full)", f)
	}
	empty := ""
	check("nested", &Section{Children: []Node{
		&Heading{Level: 1, Children: []Node{&Str{Text: "T"}}},
		&Div{Children: []Node{&BlockQuote{Children: []Node{&Para{Children: []Node{
			&Link{Destination: &empty, Children: []Node{&Strong{Children: []Node{&Str{Text: "<&>"}}}}},
			&HardBreak{}, &Span{Children: []Node{&Str{Text: "s"}}},
		}}}}}},
		&Table{Children: []Node{&Caption{}, &Row{Children: []Node{&Cell{Align: AlignRight}}}}},
		&Footnote{Label: "n", Children: []Node{&Para{}}},
	}})
}

// setChildren sets the children of a container kind the switch in
// TestAppendJSONMatchesMarshal does not list.
func setChildren(n Node, kids []Node) {
	reflect.ValueOf(n).Elem().FieldByName("Children").Set(reflect.ValueOf(kids))
}

// hasChildrenField reports whether n's kind is a container, one with a
// Children field, whether or not that field is set yet.
func hasChildrenField(n Node) bool {
	return reflect.ValueOf(n).Elem().FieldByName("Children").IsValid()
}
