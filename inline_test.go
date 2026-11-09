package gjot

import (
	"fmt"
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
		// word span and on the text inside it; a smart quote takes them
		// itself.
		{"]{.c}\n", "span (1:1:0-1:1:0) class=\"c\"\n"},
		{"]{.c}\n", "str (1:1:0-1:1:0) text=\"]\"\n"},
		{"*{.c}\n", "str (1:1:0-1:1:0) text=\"*\"\n"},
		{"'{.c}\n", "str (1:1:0-1:1:0) text=\"’\" class=\"c\"\n"},
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
		// L5490, L9448). An unclosed verbatim or math ends before its
		// trailing spaces (L10705, L10724).
		{"a\\\n", "hard_break (2:0:2-2:0:2)\n"},
		{"# foo\\ \n", "hard_break (1:7:6-2:0:7)\n"},
		{"`a  \n", "verbatim (1:1:0-1:2:1) text=\"a\"\n"},
		{"x $`a  \n", "inline_math (1:3:2-1:5:4)\n"},
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
		nodes, _, _ := parseLeafInlines(tt.src, 0, tt.endsLine)
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
	nodes, _, _ := parseLeafInlines("Don't \"x\" a...b c--d e[^n]", 0, true)
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

// A backslash escapes any ASCII punctuation in text (zjot.test L9201), and
// in a destination or a reference label also a space and a line end: the
// line end goes with the blanks before it, as djot.js trims the text before
// a hard break (zjot.test L9208-L9283).
func TestInlineEscapes(t *testing.T) {
	for _, tt := range []struct{ src, want string }{
		{"a\\,b \\&c \\/d \\;e \\?f \\@g\n", "<p>a,b &amp;c /d ;e ?f @g</p>\n"},
		{"a\\qb\n", "<p>a\\qb</p>\n"},
		{"[a](u\\,v)\n", "<p><a href=\"u,v\">a</a></p>\n"},
		{"[a](u\\ v)\n", "<p><a href=\"u v\">a</a></p>\n"},
		{"[a](u\\\nv)\n", "<p><a href=\"uv\">a</a></p>\n"},
		{"[a](u\\\t\nv)\n", "<p><a href=\"u\tv\">a</a></p>\n"},
		{"[a](u  \\\nv)\n", "<p><a href=\"uv\">a</a></p>\n"},
		{"[a](u\\\\,v)\n", "<p><a href=\"u\\,v\">a</a></p>\n"},
		{"[a][b\\ c]\n\n[b c]: /v\n", "<p><a href=\"/v\">a</a></p>\n"},
		{"[a][b\\\nc]\n\n[b c]: /u\n", "<p><a href=\"/u\">a</a></p>\n"},
		{"[a][b\\qc]\n\n[b\\qc]: /w\n", "<p><a href=\"/w\">a</a></p>\n"},
	} {
		if got := htmlOf(t, tt.src); got != tt.want {
			t.Errorf("%q\n got %q\nwant %q", tt.src, got, tt.want)
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

// Brackets close forward, as djot.js: a bare "]" is text and leaves the
// openers inside its pair standing; "]{" is a span whether or not the
// attributes parse; "][" and "](" mark the bracket, whose label or
// destination is then read as text until the next "]" or the ")" with no
// "(" of its own to pair, so brackets, verbatim, autolinks and parens inside
// it do not end it; a second "](" moves the destination; a footnote label
// ends at the first "]" and stays on one line (zjot.test L8976-L9177,
// L10493-L10603, L10845).
func TestInlineBracketsCloseForward(t *testing.T) {
	for _, tt := range []struct{ src, want string }{
		{"q[a]b](u)\n", "<p>q<a href=\"u\">a]b</a></p>\n"},
		{"q[[a]b](u)\n", "<p>q[<a href=\"u\">a]b</a></p>\n"},
		{"![a]b](u)\n", "<p><img alt=\"a]b\" src=\"u\"></p>\n"},
		{"[_a]_\n", "<p>[<em>a]</em></p>\n"},
		{"['a]'\n", "<p>[‘a]’</p>\n"},
		{"[a _b] c_ d](u)\n", "<p><a href=\"u\">a <em>b] c</em> d</a></p>\n"},
		{"[a]{x\n", "<p><span>a</span>{x</p>\n"},
		{"![a]{.c}\n", "<p>!<span class=\"c\">a</span></p>\n"},
		{"[a]{.c b](u)\n", "<p><span>a</span>{.c b](u)</p>\n"},
		{"[a](u\\)b)\n", "<p><a href=\"u)b\">a</a></p>\n"},
		{"[a][b [c] d]\n", "<p>[a][b [c] d]</p>\n"},
		{"[a](u [b] c)\n", "<p>[a](u [b] c)</p>\n"},
		{"[a](u [b](v) c)\n", "<p>[a](u <a href=\"v\">b</a> c)</p>\n"},
		{"[a](b`)`)\n", "<p><a href=\"b`)`\">a</a></p>\n"},
		{"[a](<http://x)>)\n", "<p><a href=\"&lt;http://x)&gt;\">a</a></p>\n"},
		{"[a](b(c)d) [a](b _(c_ d) e)\n", "<p><a href=\"b(c)d\">a</a> <a href=\"b _(c_ d\">a</a> e)</p>\n"},
		{"[a](b](c)\n", "<p><a href=\"c\">a](b</a></p>\n"},
		{"[a](b [c](d) e\n", "<p>[a](b <a href=\"d\">c</a> e</p>\n"},
		{"[a](b [c][d] e)\n", "<p><a href=\"b [c][d] e\">a</a></p>\n"},
		{"[a](b]{.c} d)\n", "<p><span class=\"c\">a](b</span> d)</p>\n"},
		{"*[a](b*)\n", "<p>*<a href=\"b*\">a</a></p>\n"},
		{"[x](a ([y (][c) d] e)\n", "<p><a href=\"a ([y (][c) d] e\">x</a></p>\n"},
		{"q[^a\nb]\n", "<p>q[^a\nb]</p>\n"},
		{"q[^a[b]c]\n", "<p>q<a id=\"fnref1\" href=\"#fn1\" role=\"doc-noteref\"><sup>1</sup></a>c]</p>\n"},
		{"q![^a](u)\n", "<p>q!<a id=\"fnref1\" href=\"#fn1\" role=\"doc-noteref\"><sup>1</sup></a>(u)</p>\n"},
	} {
		out, err := HTML(Parse([]byte(tt.src)))
		if err != nil {
			t.Fatal(err)
		}
		got := string(out)
		if i := strings.Index(got, "<section role"); i >= 0 {
			got = got[:i]
		}
		if got != tt.want {
			t.Errorf("%q\n got %q\nwant %q", tt.src, got, tt.want)
		}
	}
}

// Math is verbatim with a dollar prefix (zjot.test L10724, L10873): unclosed,
// it runs to the leaf's end, losing trailing spaces as an unclosed verbatim
// does; one space next to a backtick at either end goes, closed or not.
func TestInlineMathIsVerbatim(t *testing.T) {
	tests := []struct{ src, want string }{
		{"$`a\n", "<p><span class=\"math inline\">\\(a\\)</span></p>\n"},
		{"$$`a b\n", "<p><span class=\"math display\">\\[a b\\]</span></p>\n"},
		{"x $`a  \n", "<p>x <span class=\"math inline\">\\(a\\)</span></p>\n"},
		{"$`` `a` ``\n", "<p><span class=\"math inline\">\\(`a`\\)</span></p>\n"},
		{"$`` `a`\n", "<p><span class=\"math inline\">\\(`a`\\)</span></p>\n"},
		{"$$`` `a`\n", "<p><span class=\"math display\">\\[`a`\\]</span></p>\n"},
		{"$`` ` \n", "<p><span class=\"math inline\">\\(`\\)</span></p>\n"},
		// A dollar without a backtick is text.
		{"$a $$b\n", "<p>$a $$b</p>\n"},
	}
	for _, tt := range tests {
		got, err := HTML(Parse([]byte(tt.src)))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != tt.want {
			t.Errorf("%q\n got %q\nwant %q", tt.src, got, tt.want)
		}
	}
}

// A raw format is `{=`, one or more bytes other than blanks (JavaScript's
// \s), braces and backticks, and `}`, as djot.js; anything else after a
// verbatim is text (zjot.test L10737).
func TestRawFormatGrammar(t *testing.T) {
	tests := []struct{ src, want string }{
		{"`a`{=html}\n", "raw_inline (1:1:0-1:10:9) text=\"a\" lang=\"html\"\n"},
		{"`a`{=ht\nml}\n", "verbatim (1:1:0-1:3:2) text=\"a\"\n    str (1:4:3-1:7:6) text=\"{=ht\"\n"},
		{"`a`{=h\tt}\n", "verbatim (1:1:0-1:3:2) text=\"a\"\n    str (1:4:3-1:9:8) text=\"{=h\\tt}\"\n"},
		{"`a`{=h t}\n", "verbatim (1:1:0-1:3:2) text=\"a\"\n"},
		{"`a`{=h{t}\n", "verbatim (1:1:0-1:3:2) text=\"a\"\n    str (1:4:3-1:9:8) text=\"{=h{t}\"\n"},
		{"`a`{=h`t}\n", "verbatim (1:1:0-1:3:2) text=\"a\"\n"},
		{"`a`{=}\n", "verbatim (1:1:0-1:3:2) text=\"a\"\n    str (1:4:3-1:6:5) text=\"{=}\"\n"},
	}
	for _, tt := range tests {
		got := string(ASTText(Parse([]byte(tt.src), WithSourcePos())))
		if !strings.Contains(got, tt.want) {
			t.Errorf("%q\n got %q\nwant it to contain %q", tt.src, got, tt.want)
		}
	}
}

// An inline attribute attaches to the last word of the text node before it
// (after the last blank JavaScript's \s counts, NBSP and ideographic space
// included), gathering nothing across another node; a smart quote takes it
// itself and renders none, its id registered all the same (zjot.test
// L10755, L10768, L10781, L10908).
func TestInlineAttributeAttachment(t *testing.T) {
	tests := []struct{ src, want string }{
		{"a\\*b{.c}\n", "str text=\"a\"\n    span class=\"c\"\n      str text=\"*b\"\n"},
		{"a{_b{.c}\n", "str text=\"a\"\n    span class=\"c\"\n      str text=\"{_b\"\n"},
		{"a b{.c}\n", "str text=\"a \"\n    span class=\"c\"\n      str text=\"b\"\n"},
		{"a　b{.c}\n", "str text=\"a　\"\n    span class=\"c\"\n      str text=\"b\"\n"},
		{"a {.c}\n", "para\n    str text=\"a \"\n"},
		{"\"{.c}*\n", "para\n    str text=\"“\" class=\"c\"\n    str text=\"*\"\n"},
		{"\"a{.c}\n", "para\n    str text=\"“\"\n    span class=\"c\"\n      str text=\"a\"\n"},
		{"x 'a{.c}\n", "str text=\"x \"\n    str text=\"’\"\n    span class=\"c\"\n      str text=\"a\"\n"},
		{"\"{#i}a\n\n# i\n", "str text=\"“\" id=\"i\"\n    str text=\"a\"\n  section id=\"i-1\"\n"},
	}
	for _, tt := range tests {
		got := string(ASTText(Parse([]byte(tt.src))))
		if !strings.Contains(got, tt.want) {
			t.Errorf("%q\n got %q\nwant it to contain %q", tt.src, got, tt.want)
		}
	}
	// The smart quote's attributes render nothing.
	html, err := HTML(Parse([]byte("\"{#i .c}a\n")))
	if err != nil {
		t.Fatal(err)
	}
	if want := "<p>“a</p>\n"; string(html) != want {
		t.Errorf("HTML = %q, want %q", html, want)
	}
}

// The opener stack keeps its pairing when openers are looked up by kind:
// unclosed marked openers stay below the closers, and the bracket lookup
// still keeps a closer in an open destination from an opener before the
// link. The outputs are what main printed before the index by kind: this
// is the semantic guard and passes on either lookup, while
// TestInlineOpenerIndexByKind guards the index itself.
func TestInlineOpenerStackShape(t *testing.T) {
	tests := []struct{ src, want string }{
		{strings.Repeat("{_", 3) + strings.Repeat("x*", 3) + "\n", "<p>{_{_{_x<strong>x</strong>x*</p>\n"},
		{"[a](b*) *c*\n", "<p><a href=\"b*\">a</a> <strong>c</strong></p>\n"},
		{"*[a](b*)\n", "<p>*<a href=\"b*\">a</a></p>\n"},
	}
	for _, tt := range tests {
		got, err := HTML(Parse([]byte(tt.src)))
		if err != nil {
			t.Fatalf("%q: %v", tt.src, err)
		}
		if string(got) != tt.want {
			t.Errorf("%q:\n got %q\nwant %q", tt.src, got, tt.want)
		}
	}
}

// The opener stack is indexed by kind: after any run of pushes and
// truncations, tops holds the highest open opener of each kind, prevSame
// chains each opener to the one of its kind below it, and topOpener and
// nearestBracket answer what a scan of the stack from the top would.
// Against the scanning lookup there is no index to hold these.
func TestInlineOpenerIndexByKind(t *testing.T) {
	chars := []byte("[!*_~^+-='\"")
	scan := func(p *inlineParser, char byte, marked bool) int {
		for i := len(p.openers) - 1; i >= 0; i-- {
			if p.openers[i].char == char && p.openers[i].marked == marked {
				return i
			}
		}
		return -1
	}
	p := newInlineParser("", 0, true)
	seed := uint32(1)
	next := func(n int) int {
		seed = seed*1664525 + 1013904223
		return int(seed>>8) % n
	}
	for step := 0; step < 5000; step++ {
		if len(p.openers) > 0 && next(3) == 0 {
			p.truncateOpeners(next(len(p.openers) + 1))
		} else {
			c := chars[next(len(chars))]
			p.push(openerInfo{char: c, marked: next(2) == 1, srcPos: step})
		}
		for _, c := range chars {
			for _, marked := range []bool{false, true} {
				want := scan(p, c, marked)
				if got := p.tops[openerKind(c, marked)]; got != want {
					t.Fatalf("step %d: tops[%q marked=%v] = %d, want %d", step, c, marked, got, want)
				}
				if got := p.topOpener(c, marked); got != want {
					t.Fatalf("step %d: topOpener(%q, %v) = %d, want %d", step, c, marked, got, want)
				}
			}
		}
		for i, op := range p.openers {
			if want := scan(&inlineParser{openers: p.openers[:i]}, op.char, op.marked); op.prevSame != want {
				t.Fatalf("step %d: openers[%d].prevSame = %d, want %d", step, i, op.prevSame, want)
			}
		}
		if got, want := p.nearestBracket(), max(scan(p, '[', false), scan(p, '!', false)); got != want {
			t.Fatalf("step %d: nearestBracket() = %d, want %d", step, got, want)
		}
	}
}

// BenchmarkInlineOpenerStack is the input of the brief's "gjot hardened"
// (swarm-gridapp/briefs/m0-libraries.md): n marked "_" openers that never
// close, then n "*" delimiters, each of which looks up the nearest "*"
// opener. MaxNesting keeps at most 1000 openers open, so a scanning
// lookup passes up to 1000 of them per delimiter; with openers indexed by
// kind the lookup is one read, and ns/op grows linearly with n.
func BenchmarkInlineOpenerStack(b *testing.B) {
	for _, n := range []int{10000, 20000, 40000} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			src := []byte(strings.Repeat("{_", n) + strings.Repeat("x*", n))
			for b.Loop() {
				Parse(src)
			}
		})
	}
}
