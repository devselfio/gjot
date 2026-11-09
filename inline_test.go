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
		{"*a\n", "str (1:1:0-1:2:1) text=\"*a\"\n"},
		{"a] b\n", "str (1:1:0-1:4:3) text=\"a] b\"\n"},
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
		// A backslash ending the leaf is a hard break on its newline; with
		// blanks after it, from the first blank to the newline (zjot.test
		// L5490, L9448). An unclosed verbatim ends before its trailing
		// spaces (L10705).
		{"a\\\n", "hard_break (2:0:2-2:0:2)\n"},
		{"# foo\\ \n", "hard_break (1:7:6-2:0:7)\n"},
		{"`a  \n", "verbatim (1:1:0-1:2:1) text=\"a\"\n"},
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
	}, total: 6}
	tests := []struct {
		off  int
		want SourcePos
	}{
		{0, SourcePos{Line: 1, Col: 3, Offset: 2}},
		{2, SourcePos{Line: 1, Col: 5, Offset: 4}},
		{3, SourcePos{Line: 2, Col: 0, Offset: 5}},
		{4, SourcePos{Line: 2, Col: 3, Offset: 8}},
		{5, SourcePos{Line: 2, Col: 4, Offset: 9}},
		// One past the joined text: the last line's newline, where a hard
		// break ending the leaf stands.
		{6, SourcePos{Line: 3, Col: 0, Offset: 10}},
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
	}, total: 3}
	if got, want := skipped.resolve(1), (SourcePos{Line: 2, Col: 0, Offset: 3}); got != want {
		t.Errorf("skipped line: resolve(1) = %+v, want %+v", got, want)
	}
}

// A table cell's text has no line end after it, so its trailing blanks
// stay and a backslash ending it is text. A paragraph's trailing spaces
// go, a trailing tab stays as content, and a backslash ending it is a hard
// break (zjot.test L9477, L9395, L8881, L5490). The spaces go only from
// text that ends at the leaf's end or at the hard break: before an empty
// attribute or a comment they stay (L10714, L10889).
func TestLeafInlinesEndsLine(t *testing.T) {
	for _, tt := range []struct {
		src      string
		endsLine bool
		want     string
	}{
		{"a\t", true, "a\t"},
		{"a\t", false, "a\t"},
		{"a \t", true, "a \t"},
		{"a\t ", true, "a\t"},
		{"a  ", true, "a"},
		{"a  ", false, "a  "},
		{"a\\\t", false, "a\\\t"},
		{"a\\", false, "a\\"},
		{"a\\", true, "a" + hardBreak},
		{"a \t\\", true, "a" + hardBreak},
		{"a\\ \t", true, "a" + hardBreak},
		{"a\\ \t", false, "a" + nbsp + "\t"},
		{"a {}", true, "a "},
		{"a {%x%}", true, "a "},
		{"a {}\\\nb", true, "a " + hardBreak + "b"},
		{"a\t{}\\\nb", true, "a\t" + hardBreak + "b"},
		{"`a  ", true, "`a`"},
		{"`a  ", false, "`a  `"},
		{"x ` ", true, "x ``"},
	} {
		nodes, _ := parseLeafInlines(tt.src, 0, tt.endsLine)
		if got := inlineText(nodes); got != tt.want {
			t.Errorf("parseLeafInlines(%q, endsLine=%v) = %q, want %q", tt.src, tt.endsLine, got, tt.want)
		}
	}
}

const (
	hardBreak = "<br>"
	nbsp      = "\u00a0"
)

// inlineText spells inline nodes out for comparison: text as is, a hard
// break as hardBreak, a non-breaking space as nbsp, a verbatim in
// backticks.
func inlineText(nodes []*Node) string {
	var b strings.Builder
	for _, n := range nodes {
		switch n.Tag {
		case HardBreak:
			b.WriteString(hardBreak)
		case NonBreakingSpace:
			b.WriteString(nbsp)
		case Verbatim:
			b.WriteString("`" + n.Text + "`")
		default:
			b.WriteString(n.Text)
		}
	}
	return b.String()
}

func TestStringContent(t *testing.T) {
	// The inline tree before resolution, which replaces a footnote
	// reference's label with its number.
	nodes, _ := parseLeafInlines("Don't \"x\" a...b c--d e[^n]", 0, true)
	for _, tt := range []struct {
		labels bool
		want   string
	}{
		// A paired quote is a DoubleQuoted container, which has no text of
		// its own (djot.js double_quoted): only its children are read.
		{false, "Don't x a...b c--d e"},
		{true, "Don't x a...b c--d en"},
	} {
		if got := stringContent(nodes, tt.labels); got != tt.want {
			t.Errorf("stringContent(..., %v) = %q, want %q", tt.labels, got, tt.want)
		}
	}
}

func TestNormalizeLabel(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{" a\tb ", "a b"},
		{"a  b", "a b"},
		{" c ", "c"},
		{"a\nb", "a b"},
	} {
		if got := normalizeLabel(tt.in); got != tt.want {
			t.Errorf("normalizeLabel(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// Markup that matches nothing is text in one node with the text around it,
// as djot.js reports it (zjot.test L3051-L3543, L10478, L10658, L10669): an
// unmatched delimiter, bracket, brace, dollar or angle bracket joins its
// neighbours, a bracket that opens a link does not take the link's text, an
// unclosed marked opener starts a node of its own, a pair with only an empty
// attribute inside is an empty container, and a delimiter whose pair would
// be empty closes nothing.
func TestInlineUnmatchedMarkupIsText(t *testing.T) {
	tests := []struct{ src, want string }{
		{"a [ b\n", "str (1:1:0-1:5:4) text=\"a [ b\"\n"},
		{"a ![ b\n", "str (1:1:0-1:6:5) text=\"a ![ b\"\n"},
		{"[a](b\n", "str (1:1:0-1:5:4) text=\"[a](b\"\n"},
		{"a [t](u) b\n", "str (1:4:3-1:4:3) text=\"t\"\n"},
		{"a { b\n", "str (1:1:0-1:5:4) text=\"a { b\"\n"},
		{"a -} b\n", "str (1:1:0-1:6:5) text=\"a -} b\"\n"},
		{"a $ b $$ c\n", "str (1:1:0-1:10:9) text=\"a $ b $$ c\"\n"},
		{"<a\nb>\n", "str (1:1:0-1:2:1) text=\"<a\"\n"},
		{"a\\\tb\n", "str (1:1:0-1:4:3) text=\"a\\\\\\tb\"\n"},
		{"__\n", "str (1:1:0-1:2:1) text=\"__\"\n"},
		{"_a b\n", "str (1:1:0-1:4:3) text=\"_a b\"\n"},
		{"y^^ ^z\n", "str (1:1:0-1:6:5) text=\"y^^ ^z\"\n"},
		{"x _{}_ y\n", "emph (1:3:2-1:6:5)\n    str (1:7:6-1:8:7) text=\" y\"\n"},
		{"a {_e\n", "str (1:1:0-1:2:1) text=\"a \"\n    str (1:3:2-1:5:4) text=\"{_e\"\n"},
		{"`a` {=html}\n", "str (1:4:3-1:4:3) text=\" \"\n    str (1:5:4-1:11:10) text=\"{=html}\"\n"},
		// Inside a container the same joining happens.
		{"_a [ b_\n", "emph (1:1:0-1:7:6)\n      str (1:2:1-1:6:5) text=\"a [ b\"\n"},
		// An unmatched smart quote is not source text and stays its own node.
		{"a\"\n", "str (1:1:0-1:1:0) text=\"a\"\n    str (1:2:1-1:2:1) text=\"\u201c\"\n"},
	}
	for _, tt := range tests {
		got := string(ASTText(Parse([]byte(tt.src), WithSourcePos())))
		if !strings.Contains(got, tt.want) {
			t.Errorf("%q\n got %q\nwant it to contain %q", tt.src, got, tt.want)
		}
	}
}

// An autolink is djot.js's <([^<>\s]+)>: nothing blank or a second < inside,
// an email when an @ follows a byte other than a colon, else a URL when a
// letter is followed by a colon.
func TestAutolinkAt(t *testing.T) {
	for _, tt := range []struct {
		src   string
		ok    bool
		email bool
	}{
		{"<http://a>", true, false},
		{"<a@b>", true, true},
		{"<mailto:a@b>", true, true},
		{"<a\nb>", false, false},
		{"<a b:c>", false, false},
		{"<a<b:c>", false, false},
		{"<>", false, false},
		{"<a:>", true, false},
		{"<1:a>", false, false},
		{"<a", false, false},
	} {
		_, email, ok := autolinkAt(tt.src, 0)
		if ok != tt.ok || email != tt.email {
			t.Errorf("%q: got ok=%v email=%v, want ok=%v email=%v", tt.src, ok, email, tt.ok, tt.email)
		}
	}
}
