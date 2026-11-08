package gjot

import (
	"strings"
	"testing"
)

// Inline nodes the corpus does not pin still carry the range of the source
// bytes they stand for when parsed with WithSourcePos.
func TestInlinePositions(t *testing.T) {
	tests := []struct{ src, want string }{
		{"a[^x]\n", "footnote_reference (1:2:1-1:5:4)"},
		{"a `x`{=html} b\n", "raw_inline (1:3:2-1:12:11) text=\"x\" lang=\"html\"\n"},
		{"a -- b\n", "en_dash (1:3:2-1:4:3)\n"},
		{"a --- b\n", "em_dash (1:3:2-1:5:4)\n"},
		{"a... b\n", "ellipsis (1:2:1-1:4:3)\n"},
		{"*a\n", "str (1:1:0-1:1:0) text=\"*\"\n"},
		{"a] b\n", "str (1:2:1-1:2:1) text=\"]\"\n"},
		{"word{.c} x\n", "span (1:1:0-1:4:3) class=\"c\"\n"},
		{"word{.c} x\n", "str (1:1:0-1:4:3) text=\"word\"\n"},
		// Attributes on literal text keep the literal's extent, on the
		// word span and on the text inside it.
		{"]{.c}\n", "span (1:1:0-1:1:0) class=\"c\"\n"},
		{"]{.c}\n", "str (1:1:0-1:1:0) text=\"]\"\n"},
		{"*{.c}\n", "str (1:1:0-1:1:0) text=\"*\"\n"},
		{"'{.c}\n", "str (1:1:0-1:1:0) text=\"’\"\n"},
		{"![{.c}\n", "span (1:1:0-1:2:1) class=\"c\"\n"},
		{"![{.c}\n", "str (1:1:0-1:2:1) text=\"![\"\n"},
		{"a ]{.c}\n", "str (1:3:2-1:3:2) text=\"]\"\n"},
		// Nesting: containers and the text inside them.
		{"_a *b*_\n", "emph (1:1:0-1:7:6)\n"},
		{"_a *b*_\n", "strong (1:4:3-1:6:5)\n"},
		{"_a *b*_\n", "str (1:5:4-1:5:4) text=\"b\"\n"},
		// Inside a block quote, offsets and columns follow the source.
		{"> a\n> *b*\n", "soft_break (2:0:3-2:0:3)\n"},
		{"> a\n> *b*\n", "strong (2:3:6-2:5:8)\n"},
	}
	for _, tt := range tests {
		got := string(ASTText(Parse([]byte(tt.src), WithSourcePos())))
		if !strings.Contains(got, tt.want) {
			t.Errorf("%q\n got %q\nwant it to contain %q", tt.src, got, tt.want)
		}
	}
}

// Every inline node under a paragraph has a position.
func TestEveryInlineNodeHasPosition(t *testing.T) {
	src := "a _b_ *c* [l](u) ![i](u) [s]{.x} `v` x\\\ny\\ z {=m=} {+i+} {-d-} " +
		"H~2~O x^2^ \"q\" {'s'} $`m` $$`n` :smile: <http://x> <a@b> a[^n] -- --- ... w{.c} *u ]{.c} {\"q <y ![i] { a\\\n"
	doc := Parse([]byte(src), WithSourcePos())
	var walk func(n *Node)
	walk = func(n *Node) {
		for _, c := range n.Children {
			if c.Pos == nil {
				t.Errorf("%s %q has no position", c.Tag, c.Text)
			}
			walk(c)
		}
	}
	para := doc.Children[0]
	if para.Tag != Para {
		t.Fatalf("got %s, want para", para.Tag)
	}
	walk(para)
}

// The newline that joins two lines resolves to the next line, column 0, at
// the newline's own offset.
func TestLineMapResolveJoiningNewline(t *testing.T) {
	m := lineMap{segments: []lineSegment{
		{joinedStart: 0, orig: SourcePos{Line: 1, Col: 3, Offset: 2}, eol: SourcePos{Line: 2, Col: 0, Offset: 5}},
		{joinedStart: 4, orig: SourcePos{Line: 2, Col: 3, Offset: 8}, eol: SourcePos{Line: 3, Col: 0, Offset: 10}},
	}}
	tests := []struct {
		off  int
		want SourcePos
	}{
		{0, SourcePos{Line: 1, Col: 3, Offset: 2}},
		{2, SourcePos{Line: 1, Col: 5, Offset: 4}},
		{3, SourcePos{Line: 2, Col: 0, Offset: 5}},
		{4, SourcePos{Line: 2, Col: 3, Offset: 8}},
		{5, SourcePos{Line: 2, Col: 4, Offset: 9}},
	}
	for _, tt := range tests {
		if got := m.resolve(tt.off); got != tt.want {
			t.Errorf("resolve(%d) = %+v, want %+v", tt.off, got, tt.want)
		}
	}
	if r := m.rangeOf(3, 3); r != nil {
		t.Errorf("rangeOf(3, 3) = %+v, want nil", r)
	}
	want := Range{Start: SourcePos{Line: 1, Col: 4, Offset: 3}, End: SourcePos{Line: 2, Col: 0, Offset: 5}}
	if r := m.rangeOf(1, 4); r == nil || *r != want {
		t.Errorf("rangeOf(1, 4) = %+v, want %+v", r, want)
	}

	// Segments on lines 1 and 3, line 2 skipped (zjot.test L9563, "# a",
	// "#", "# b"): the joining newline is line 1's own, not line 3's.
	skipped := lineMap{segments: []lineSegment{
		{joinedStart: 0, orig: SourcePos{Line: 1, Col: 3, Offset: 2}, eol: SourcePos{Line: 2, Col: 0, Offset: 3}},
		{joinedStart: 2, orig: SourcePos{Line: 3, Col: 3, Offset: 8}, eol: SourcePos{Line: 4, Col: 0, Offset: 9}},
	}}
	if got, want := skipped.resolve(1), (SourcePos{Line: 2, Col: 0, Offset: 3}); got != want {
		t.Errorf("skipped line: resolve(1) = %+v, want %+v", got, want)
	}
}
