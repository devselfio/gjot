package gjot

import (
	"strings"
	"testing"
)

// roundTrip re-parses the written Djot and returns its HTML.
func roundTrip(t *testing.T, src string) (djot, html string) {
	t.Helper()
	djot = string(Djot(Parse([]byte(src))))
	out, err := HTML(Parse([]byte(djot)))
	if err != nil {
		t.Fatal(err)
	}
	return djot, string(out)
}

func TestWriterNestingDepth(t *testing.T) {
	src := "> - a\n>\n>   > 1. b\n>   >\n>   >    ::: c\n>   >    _d *e [f]{.g}*_\n>   >    :::\n"
	want, err := HTML(Parse([]byte(src)))
	if err != nil {
		t.Fatal(err)
	}
	if _, got := roundTrip(t, src); got != string(want) {
		t.Errorf("round trip changed HTML:\n---want---\n%s---got---\n%s", want, got)
	}
}

func TestWriterFenceEscalation(t *testing.T) {
	djot, _ := roundTrip(t, "````\n```\nx\n```\n````\n\n::::\n:::\ny\n:::\n::::\n")
	want := "````\n```\nx\n```\n````\n\n::::\n:::\ny\n:::\n::::\n"
	if djot != want {
		t.Errorf("got\n%s\nwant\n%s", djot, want)
	}
}

func TestWriterAttributeLines(t *testing.T) {
	djot, _ := roundTrip(t, "{#intro .big k=\"a \\\"b\\\"\"}\n# Intro\n\n## Auto id\n\n{.note}\n> q\n")
	want := "{#intro .big k=\"a \\\"b\\\"\"}\n# Intro\n\n## Auto id\n\n{.note}\n> q\n"
	if djot != want {
		t.Errorf("got\n%s\nwant\n%s", djot, want)
	}
}

func TestWriterInlineEdges(t *testing.T) {
	for _, src := range []string{
		"a--{}---b and ---{}--c\n",
		"--\\-x and x\\---\n",
		"x\\ \ny\\ \n",
		":ice:scream: and a:b and :a:b:c:\n",
		"1\\. not a list\n\n(a\\) nor this\n\n\\- nor this\n\n\\# nor a heading\n",
		"don't and \"quoted\" and 'single' and {\"x\"}\n",
		"_a _b_ c_ and _a_b_ and *x*y and {_ s _}\n",
		"^a^b^ and {^a{^b^}^} and ~x~\n",
		"{+ins+}{-del-}{=mark=} and \\{+ no\n",
		"`` `x` `` and ` `` ` and $`a`b$ and $$`m`\n",
		// Math is verbatim: a backtick at either end needs its padding, and
		// an unclosed one runs to the end (zjot.test L10724, L10873).
		"$`` `a` `` and $`` `a`\n\nx $`a  \n\n$$`` ` \n",
		// A smart quote's attributes are written, its id registered; the
		// quote it is written as is markup to the text before it.
		"\"{#i}a and 'b{.c}\n\n# i\n",
		"a\\\\\"{.c}b and \"c\"{.d} and \"{.e}f{\"g\"}\n",
		// A lone ’ inside a plain pair, which stays plain: '} closes a braced one.
		"'_x '{.c}_ z' and '[x '{.c}](y) z' and it's '[a '{.c}](b)'\n",
		"[a](b\\)c) and [x][] and [y][z] and ![i](j){.k}\n",
		// A destination's bytes the forward pass would consume or close on.
		"[a](b`)`) and [c](<http://x)>) and [d](e\\[f\\]g) and ![h](i\\(j) and [k](l\\{m)\n",
		// Text a bare "]" leaves standing, and a destination's inner link.
		"[a][b [c] d] and [e](f [g](h) i) and [j]{k and [l](m](n)\n",
		"<http://a\"b> and <x@y> and <mailto:x@y>\n",
		"a... and ...b and \\.\\.\\. and {1--}\n",
		"{#id} at beginning\n\nAfter {#id} space\n{.class}\n",
		"| a | b |\n|:-|-:|\n| c | d |\n| e | f |\n|-:|:-:|\n| g | h |\n\n^ cap\n",
	} {
		want, err := HTML(Parse([]byte(src)))
		if err != nil {
			t.Fatal(err)
		}
		if djot, got := roundTrip(t, src); got != string(want) {
			t.Errorf("%q\n---written---\n%s---want---\n%s---got---\n%s", src, djot, want, got)
		}
	}
}

func TestWriterRestoresReferenceDefinitions(t *testing.T) {
	src := "[a][r] and [b][R\ntwo]\n\n{title=\"t\"}\n[r]: /u\n[R two]: /v\n"
	djot, html := roundTrip(t, src)
	for _, want := range []string{"[a][r]", "[r]: /u", "[R two]: /v", "{title=\"t\"}"} {
		if !strings.Contains(djot, want) {
			t.Errorf("written Djot lacks %q:\n%s", want, djot)
		}
	}
	if want := htmlOf(t, src); html != want {
		t.Errorf("html\n got %q\nwant %q", html, want)
	}
	if got, want := string(ASTText(Parse([]byte(djot)))), string(ASTText(Parse([]byte(src)))); got != want {
		t.Errorf("ast\n got %s\nwant %s", got, want)
	}
}

// A reference label is keyed with its escapes dropped, so the writer escapes
// the bytes of the key that would read as an escape or a bracket; a
// destination's blanks and punctuation need no escape, its backslash does;
// an attribute value's backslash is escaped wherever it stands.
func TestWriterEscapesLabelsAndDestinations(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"[a][b\\]c]\n", "[a][b\\]c]\n"},
		{"[a][b\\\\,c]\n", "[a][b\\\\,c]\n"},
		{"[a][b\\\\ c]\n", "[a][b\\\\ c]\n"},
		// A definition's label is keyed as written (djot.js), so the link's
		// escaped backslash matches the definition's bare one.
		{"[a][b\\\\c]\n\n[b\\c]: /u\n", "[a][b\\\\c]\n\n[b\\c]: /u\n"},
		{"[a][b [c][d] e]\n", "[a][b \\[c\\]\\[d\\] e]\n"},
		{"[a](u\\ v\\,w\\\\x)\n", "[a](u v,w\\\\x)\n"},
		{"a{k=\"x\\ y\\\\z\"}\n", "[a]{k=\"x\\\\ y\\\\z\"}\n"},
	} {
		djot, _ := roundTrip(t, c.src)
		if djot != c.want {
			t.Errorf("%q written %q, want %q", c.src, djot, c.want)
		}
		want := string(ASTText(Parse([]byte(c.src))))
		if got := string(ASTText(Parse([]byte(djot)))); got != want {
			t.Errorf("%q round trip changed AST:\n---want---\n%s---got---\n%s", c.src, want, got)
		}
	}
}

func TestWriterApostrophe(t *testing.T) {
	src := "# Don't panic\n"
	want, err := HTML(Parse([]byte(src)))
	if err != nil {
		t.Fatal(err)
	}
	djot, got := roundTrip(t, src)
	if djot != src {
		t.Errorf("%q written as %q", src, djot)
	}
	if got != string(want) {
		t.Errorf("%q\n---want---\n%s---got---\n%s", src, want, got)
	}
	for _, src := range []string{
		"{'don't'}\n",
		"'tis and don't\n",
		// An apostrophe inside a quote in a heading keeps the heading's id.
		"# {'don't'}\n\n[don't][]\n",
		"# 'a don't b'\n\n[a don't b][]\n",
		// Brackets inside a quote: a ' written there must not close it.
		"{'a [b's](u) c'}\n",
		"'a [b’s]{.k} c'\n",
		"'a ![b’s](u) c'\n",
		"'a \"b [c's](u)\" d'\n",
		// After a non-ASCII letter, a digit or an underscore.
		"# café's\n\n[café's][]\n",
		"# x_'s\n\n[x_'s][]\n",
		"# 1's\n",
		// A ’ typed as the glyph is written as the glyph, so ids and alt text
		// that read it keep it.
		"# b’s\n\n[b’s][]\n",
		"![b’s](u)\n",
	} {
		want, err := HTML(Parse([]byte(src)))
		if err != nil {
			t.Fatal(err)
		}
		if djot, got := roundTrip(t, src); got != string(want) {
			t.Errorf("%q\n---written---\n%s---want---\n%s---got---\n%s", src, djot, want, got)
		}
	}
}

// The writer keeps an ordered list's delimiter and start, so lists that
// differ only in delimiter stay apart on re-parse (zjot.test L7142, L7165).
func TestWriterOrderedMarkers(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"(1) a\n1) b\n1. c\n", "(1) a\n\n1) b\n\n1. c\n"},
		{"1. a\n2. b\n\n3) c\n", "1. a\n2. b\n\n3) c\n"},
		{"i. x\nj. y\n", "i. x\nj. y\n"},
		{"iv) x\nv) y\n", "iv) x\nv) y\n"},
		{"(C) x\n", "(C) x\n"},
		{"{.x}\n5. a\n", "{.x}\n5. a\n"},
	} {
		djot, html := roundTrip(t, tc.src)
		if djot != tc.want {
			t.Errorf("wrote\n%s\nwant\n%s", djot, tc.want)
		}
		if want, _ := HTML(Parse([]byte(tc.src))); html != string(want) {
			t.Errorf("%q: round trip changed HTML:\n---want---\n%s---got---\n%s", tc.src, want, html)
		}
	}
}

// A caption is written right under its rows, so a table ending a tight list
// item keeps the list tight (grid.test L1166); an empty caption is not
// written at all (zjot.test L4039).
func TestWriterTableCaption(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"caption-in-tight-item",
			"- | a | b |\n  |---|--:|\n  | 1 | 2 |\n  ^ cap\n",
			"- | a | b |\n  |---|--:|\n  | 1 | 2 |\n  ^ cap\n"},
		{"empty-caption",
			"| a |\n|---|\n^ \n",
			"| a |\n|---|\n"},
		{"caption-with-block-attributes",
			"| a |\n{.x}\n^ cap\n",
			"| a |\n{.x}\n^ cap\n"},
		{"empty-caption-with-an-id",
			"| a |\n{#x}\n^ \n",
			"| a |\n{#x}\n^ \n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			djot, _ := roundTrip(t, c.src)
			if djot != c.want {
				t.Errorf("written\n%s\nwant\n%s", djot, c.want)
			}
			want := string(ASTText(Parse([]byte(c.src))))
			if got := string(ASTText(Parse([]byte(djot)))); got != want {
				t.Errorf("round trip changed AST:\n---want---\n%s---got---\n%s", want, got)
			}
		})
	}
}

// Trailing blanks the parser keeps are written so they read back: a space
// before an empty attribute or a hard break gets its `{}`, a trailing tab
// is content as written, and a leaf ending in a hard break is written with
// its backslash last (zjot.test L10714, L10889, L8881, L5490, L5640).
func TestWriterTrailingBlanksAndHardBreak(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"space-before-empty-attribute", "a {}\n", "a {}\n"},
		{"space-before-comment", "a {%x%}\n", "a {}\n"},
		{"space-before-hard-break", "a {}\\\nb\n", "a {}\\\nb\n"},
		{"tab-before-hard-break", "a\t{}\\\nb\n", "a\t{}\\\nb\n"},
		{"trailing-tab", "a\t\n", "a\t\n"},
		{"space-then-tab", "a \t\n", "a \t\n"},
		{"paragraph-ending-in-hard-break", "a\\\n", "a\\\n"},
		{"heading-ending-in-hard-break", "# a\\\n", "# a\\\n"},
		{"item-ending-in-hard-break", "- a\\\n- b\n", "- a\\\n- b\n"},
		{"caption-ending-in-hard-break", "| a |\n^ c\\\n", "| a |\n^ c\\\n"},
		{"escaped-backslash-then-soft-break", "a\\\\\nb\n", "a\\\\\nb\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			djot, _ := roundTrip(t, c.src)
			if djot != c.want {
				t.Errorf("written %q, want %q", djot, c.want)
			}
			want := string(ASTText(Parse([]byte(c.src))))
			if got := string(ASTText(Parse([]byte(djot)))); got != want {
				t.Errorf("round trip changed AST:\n---want---\n%s---got---\n%s", want, got)
			}
		})
	}
}

// An escape starts a new text node when read back, so text is escaped only
// where a raw byte would read as markup: at a run's start, and where the
// byte would close an enclosing delimiter, open an attribute, a math span,
// an autolink or a smart quote, or escape or break what follows it. Bytes
// the parser reads as text where they stand are written raw, so a text node
// the parser joined reads back as one node (zjot.test L3051-L3543, L10478,
// L10658, L10669).
func TestWriterTextEscapes(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		// splits: the escape the HTML needs splits a text node the parser
		// joined, so only the HTML round trip is checked.
		splits bool
	}{
		{"unclosed-emph", "_a b\n", "\\_a b\n", false},
		{"empty-pair", "__\n", "\\__\n", false},
		{"lone-star", "a * b\n", "a * b\n", false},
		{"unclosed-link", "[a](b\n", "\\[a](b\n", false},
		{"lone-brackets", "a [ b ] c ![ d\n", "a [ b ] c ![ d\n", false},
		{"lone-brace", "a { b\n", "a { b\n", false},
		{"unclosed-attribute", "a{.c b\n", "a{.c b\n", false},
		{"closing-brace-pair", "a -} b\n", "a -} b\n", false},
		{"dollars", "a $ b $$ c\n", "a $ b $$ c\n", false},
		{"angle-brackets", "<a\nb>\n", "\\<a\nb>\n", false},
		{"backslash-before-tab", "a\\\tb\n", "a\\\tb\n", false},
		{"backslash-before-space", "a \\\\ b\n", "a \\\\ b\n", false},
		{"carets", "x^2^ y^^ ^z\n", "x^2^ y^^ ^z\n", false},
		{"unclosed-marked-openers", "a {_e\n\na {+e\n\n`a` {=html}\n", "a {_e\n\na {+e\n\n`a` {=html}\n", false},
		{"unclosed-marked-opener-then-pair", "a {^+^ b\n", "a {^+^ b\n", false},
		{"empty-emph", "x _{}_ y\n", "x _{}_ y\n", false},
		{"empty-emph-touching-text", "x_{}_y\n", "x_{}_y\n", false},
		{"empty-mark", "a {={}=} b\n", "a {={}=} b\n", false},
		// Text nodes the parser keeps apart stay apart (zjot.test L3456).
		{"escape-splits-text", "a \\* b\n", "a \\* b\n", false},
		{"empty-attribute-splits-text", "a {}b\n", "a {}b\n", false},
		{"comment-splits-text", "a {% c %} b\n", "a {} b\n", false},
		{"marked-text-inside-mark", "{=={}==}\n", "{=={}==}\n", false},
		{"quote-glyph-then-text", "\"a\n", "\u201c{}a\n", false},
		{"apostrophe-then-text", "don't\n", "don't\n", false},
		{"brace-closer-inside-plain-emph", "_a -} b_\n", "_a -} b_\n", false},
		{"own-closer-inside-plain-emph", "_a _} b_\n", "_a _} b_\n", false},
		// What follows the run is judged too: the next node's first bytes,
		// the empty attribute kept at a boundary, the enclosing closer.
		{"dollar-before-verbatim", "x ${}`b`\n", "x \\$`b`\n", true},
		{"dollar-before-math", "x ${}$`b`\n", "x \\$$`b`\n", true},
		{"dollar-before-display-math", "x ${}$$`b`\n", "x \\$$$`b`\n", true},
		{"comment-closed-by-later-brace", "x {%a *b* c\\['\n", "x \\{%a *b* c\\[{}\u2019\n", true},
		{"brace-before-soft-break-then-comment", "x {\n%\\+\n", "x \\{\n%{}+\n", true},
		{"brace-before-verbatim-attribute", "a { .c `x`\n", "a { .c `x`\n", false},
		{"brace-before-emph-attribute", "a {.c _x_\n", "a \\{.c _x_\n", true},
		{"comment-closed-by-trailing-attribute", "a \\{% b {}\n", "a \\{% b {}\n", false},
		{"quoted-value-then-comment-closed-later", "x {k=\"v\" %a *b* c\\['\n", "x \\{k=\"v\" %a *b* c\\[{}\u2019\n", true},
		{"quoted-value-before-soft-break", "x {k=\"v\"\n%\\+\n", "x \\{k=\"v\"\n%{}+\n", true},
		// At a line's start a marked opener's brace is escaped: the line
		// could read as a block attribute.
		{"marked-opener-at-line-start", "\\{-k=v}\nb\n", "\\{\\-k=v}\nb\n", true},
		{"marked-opener-at-item-start", "- \\{_a=b}\n", "- \\{\\_a=b}\n", true},
		{"marked-opener-at-line-start-from-comment", "{-k=v}{%c%}\n", "\\{\\-k=v}\n", true},
		{"marked-opener-at-line-start-then-pair", "{_k=v}_a_\n", "\\{\\_k=v}_a_\n", true},
		{"marked-opener-at-line-start-not-attribute", "{_*_a_*\n", "{_*_a_*\n", false},
		{"marked-opener-alone-on-line", "{_b_\n", "{_b_\n", false},
		// A marked opener escaped for any reason takes its delimiter's
		// escape with it (pass 5: the freed delimiter paired later).
		{"second-marked-opener-of-open-kind", "\\{_\n{_.[_a_](u)\n", "{_\n\\{\\_.[_a_](u)\n", true},
		{"marked-opener-inside-marked-pair-then-pair", "x {_\nb {_.{=_a_=}\n", "x {_\nb \\{\\_.{=_a_=}\n", true},
		{"raw-format-after-verbatim-is-text", "x `a`\\{=html} y\n", "x `a`\\{=html} y\n", false},
		{"brace-then-escaped-delimiter", "{\\_\n", "\\{\\_\n", false},
		{"bracket-before-boundary-attribute", "+][ ]'\n", "\\+][ \\]{}\u2019\n", true},
		{"brace-before-boundary-attribute", "a{\t%'\n", "a\\{\t%{}\u2019\n", true},
		{"opener-before-pair-closer", "{_a __}\n", "_a \\__\n", true},
		{"escaped-brace-after-delimiter", "{*{}\\}*\n", "{*\\}*\n", false},
		{"brace-before-verbatim", "b{`c`\n", "b{`c`\n", false},
		{"unclosed-label", "%[][\n", "%[][\n", false},
		{"brace-after-marked-opener", "{-} x:{+}\n", "{-} x:{+}\n", false},
		{"opener-inside-pair-steals-closer", "{_a _b_}\n", "_a \\_b_\n", true},
		{"bracket-before-superscript", "a [{^b^}]\n", "a \\[^b^\\]\n", true},
		{"closer-inside-bracket-reaches-out", "{~[a~]{.c}~}\n", "~[a\\~]{.c}~\n", true},
		{"marked-opener-inside-braced-pair", "a __{_b c_\n", "a _{_\\{\\_b c_}\n", true},
		// In a span's text both brackets are escaped.
		{"brackets-in-span-text", "b]#{.c}\n", "[b\\]#]{.c}\n", true},
		{"open-bracket-in-span-text", "$[.={.c}\n", "[\\$\\[.=]{.c}\n", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			djot, _ := roundTrip(t, c.src)
			if djot != c.want {
				t.Errorf("written %q, want %q", djot, c.want)
			}
			want := string(ASTText(Parse([]byte(c.src))))
			if got := string(ASTText(Parse([]byte(djot)))); got != want && !c.splits {
				t.Errorf("round trip changed AST:\n---want---\n%s---got---\n%s", want, got)
			}
			wantHTML, _ := HTML(Parse([]byte(c.src)))
			if gotHTML, _ := HTML(Parse([]byte(djot))); string(gotHTML) != string(wantHTML) {
				t.Errorf("round trip changed HTML:\n---want---\n%s---got---\n%s", wantHTML, gotHTML)
			}
		})
	}
}

// Text the parser never produces, with bytes that would read as markup
// where they stand, is still written so that it reads back as that text.
func TestWriterEscapesMarkupInText(t *testing.T) {
	for _, c := range []struct {
		name string
		para []*Node
	}{
		{"markup-in-text", []*Node{{Tag: Str, Text: "a `b` \"c\" 'd' |e| $`f` <http://g> h{.i} {\"j k}} _l_ *m* [n](o) p\\ q\\"}}},
		{"own-closer-inside-insert", []*Node{{Tag: Insert, Children: []*Node{{Tag: Str, Text: "a +} b"}}}}},
		{"own-closer-inside-braced-emph", []*Node{{Tag: Emph, Children: []*Node{{Tag: Str, Text: " a _} b"}}}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			doc := &Node{Tag: Section, Children: []*Node{{Tag: Para, Children: c.para}}}
			djot := Djot(doc)
			got, err := HTML(Parse(djot))
			if err != nil {
				t.Fatal(err)
			}
			want, err := HTML(doc)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(want) {
				t.Errorf("written %q\n---want---\n%s---got---\n%s", djot, want, got)
			}
		})
	}
}

// Attributes are written in their order, an empty id as `id=""`, the last
// class of a div on its fence, and on math, symbols, autolinks and footnote
// references; each form reads back to the same HTML (zjot.test L6951,
// L6431, L10414, L5961, L6604).
func TestWriterAttributeOrderAndEmptyID(t *testing.T) {
	cases := []struct{ input, want string }{
		{"{k=v #i .c}\n> q\n", "{k=\"v\" #i .c}\n> q\n"},
		{"{id=\"\"}\npara [a]{id=\"\"}\n\n> {id=\"\"}\n> # h\n", "{id=\"\"}\npara [a]{id=\"\"}\n\n> {id=\"\"}\n> # h\n"},
		{"{.x}\n::: y\nz\n:::\n", "{.x}\n::: y\nz\n:::\n"},
		{"{.y k=v}\n:::\nz\n:::\n", "{.y k=\"v\"}\n:::\nz\n:::\n"},
		{"{.y}\n{k=v}\n::: w\nz\n:::\n", "{.y .w k=\"v\"}\n:::\nz\n:::\n"},
		{"$`m`{#i .a k=v} :sym:{.b} <http://x>{.c} <a@b.c>{#j}\n", "$`m`{#i .a k=\"v\"} :sym:{.b} <http://x>{.c} <a@b.c>{#j}\n"},
		{"x[^n]{.a}\n\n[^n]: n\n", "x[^n]{.a}\n\n[^n]: n\n"},
	}
	for _, c := range cases {
		djot, html := roundTrip(t, c.input)
		if djot != c.want {
			t.Errorf("---input---\n%s---want---\n%s---got---\n%s", c.input, c.want, djot)
		}
		want, err := HTML(Parse([]byte(c.input)))
		if err != nil {
			t.Fatal(err)
		}
		if html != string(want) {
			t.Errorf("---input---\n%s---html before---\n%s---after---\n%s", c.input, want, html)
		}
	}
}

// A thematic break opening an item goes under a bare marker, since the
// marker and `* * *` on one line read as a thematic break (grid.test L232,
// L244); a loose list of one item, which only an AST-invisible block after a
// blank line can have made loose (a reference definition in the source),
// ends its item with `{}` after a blank line so it reads back loose
// (grid.test L1463, zjot.test L1705).
func TestWriterItemThematicBreakAndLooseSingleItem(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"thematic-break-item", "-\n  ***\n", "-\n  * * *\n"},
		{"thematic-break-ordered-item", "1.\n   ***\n", "1.\n   * * *\n"},
		{"thematic-break-task-item", "- [x]\n  ***\n", "- [x]\n  * * *\n"},
		{"thematic-break-second-block", "- a\n\n  ***\n", "- a\n\n  * * *\n"},
		{"loose-item-with-its-definition", "- [t][r]\n\n  [r]: /u\n", "- [t][r]\n\n  {}\n\n[r]: /u\n"},
		{"loose-item-with-unused-definition", "- a\n\n  [r]: /u\nlazy\n", "- a\n\n  {}\n\nlazy\n"},
		{"loose-empty-item", "-\n\n  [r]: /u\n", "-\n\n  {}\n"},
		{"loose-nested-item", "- - a\n\n    [r]: /u\n", "- - a\n\n    {}\n"},
		{"loose-two-items-unchanged", "- a\n\n- b\n", "- a\n\n- b\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			djot, _ := roundTrip(t, c.src)
			if djot != c.want {
				t.Errorf("written\n%s\nwant\n%s", djot, c.want)
			}
			want := string(ASTText(Parse([]byte(c.src))))
			if got := string(ASTText(Parse([]byte(djot)))); got != want {
				t.Errorf("round trip changed AST:\n---want---\n%s---got---\n%s", want, got)
			}
		})
	}
}

// A paragraph line that is only blanks (what a block attribute with text
// after its `}` leaves of its first line) is written `{}`, the blanks, `{}`:
// without the closing `{}` the line reads back as an empty attribute block
// and the paragraph loses it (zjot.test L9026, L9045).
func TestWriterBlanksOnlyLine(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"tab-and-space-first-line", "{#x .b}\t {#y}\npara\n", "{}\t {}\npara\n"},
		{"space-first-line", "{#x} {.c}\npara\n", "{} {}\npara\n"},
		{"blanks-then-word", "{#x} junk\npara\n", "{} junk\npara\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			djot, _ := roundTrip(t, c.src)
			if djot != c.want {
				t.Errorf("written\n%s\nwant\n%s", djot, c.want)
			}
			want := string(ASTText(Parse([]byte(c.src))))
			if got := string(ASTText(Parse([]byte(djot)))); got != want {
				t.Errorf("round trip changed AST:\n---want---\n%s---got---\n%s", want, got)
			}
		})
	}
}

// Footnotes are written with their labels, the definition's content indented
// under its marker's width so a code block keeps its indentation relative to
// the fence (zjot.test L1845, L1867); each reads back to the same HTML.
func TestWriterFootnotes(t *testing.T) {
	cases := []struct{ input, want string }{
		{"x[^note]\n\n[^note]: n\n", "x[^note]\n\n[^note]: n\n"},
		{"[^n]:\n    ```\n      x\n    ```\n\nx[^n]\n", "x[^n]\n\n[^n]: ```\n        x\n      ```\n"},
		{"> a[^x]\n>\n> [^x]: in quote\n", "> a[^x]\n\n[^x]: in quote\n"},
	}
	for _, c := range cases {
		djot, html := roundTrip(t, c.input)
		if djot != c.want {
			t.Errorf("Djot(%q) = %q, want %q", c.input, djot, c.want)
		}
		if want := htmlOf(t, c.input); html != want {
			t.Errorf("round trip of %q: HTML %q, want %q", c.input, html, want)
		}
	}
}
