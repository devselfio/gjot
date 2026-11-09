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
		"[a](b\\)c) and [x][] and [y][z] and ![i](j){.k}\n",
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
