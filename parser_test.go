package gjot

import (
	"fmt"
	"strings"
	"testing"
)

func htmlOf(t *testing.T, src string) string {
	t.Helper()
	out, err := HTML(Parse([]byte(src)))
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestNestedHeadingsGetIDsAndImplicitReferences(t *testing.T) {
	tests := []struct{ src, want string }{
		{"::: x\n# Head\n:::\n", "<div class=\"x\">\n<h1 id=\"Head\">Head</h1>\n</div>\n"},
		{"- # Head\n", "<ul>\n<li>\n<h1 id=\"Head\">Head</h1>\n</li>\n</ul>\n"},
		{"::: x\n# Head\n:::\n\n[Head][]\n", "<div class=\"x\">\n<h1 id=\"Head\">Head</h1>\n</div>\n<p><a href=\"#Head\">Head</a></p>\n"},
		{"# Heading\nline two\n\n[Heading line two][]\n", "<section id=\"Heading-line-two\">\n<h1>Heading\nline two</h1>\n<p><a href=\"#Heading-line-two\">Heading line two</a></p>\n</section>\n"},
		{"{#Head}\npara\n\n> # Head\n", "<p id=\"Head\">para</p>\n<blockquote>\n<h1 id=\"Head-1\">Head</h1>\n</blockquote>\n"},
	}
	for _, tt := range tests {
		if got := htmlOf(t, tt.src); got != tt.want {
			t.Errorf("%q\n got %q\nwant %q", tt.src, got, tt.want)
		}
	}
}

func TestBlockQuoteLazyLinesStopAtVerbatimBlocks(t *testing.T) {
	tests := []struct{ src, want string }{
		{"> ```\n> code\nfoo\n", "<blockquote>\n<pre><code>code\n</code></pre>\n</blockquote>\n<p>foo</p>\n"},
		{"> ```\n> code\n> ```\nfoo\n", "<blockquote>\n<pre><code>code\n</code></pre>\n</blockquote>\n<p>foo</p>\n"},
		{"> * * *\nfoo\n", "<blockquote>\n<hr>\n</blockquote>\n<p>foo</p>\n"},
		{"> para\nlazy\n", "<blockquote>\n<p>para\nlazy</p>\n</blockquote>\n"},
	}
	for _, tt := range tests {
		if got := htmlOf(t, tt.src); got != tt.want {
			t.Errorf("%q\n got %q\nwant %q", tt.src, got, tt.want)
		}
	}
}

func TestSourcePositionsInsideNestedBlocks(t *testing.T) {
	tests := []struct{ src, want string }{
		{"> a\n", "para (1:3:2-2:0:3)\n      str (1:3:2-1:3:2) text=\"a\"\n"},
		{"[^1]: note\n\nx[^1]\n", "para (1:7:6-2:0:10)\n"},
		{": term\n\n  def\n", "para (3:3:10-4:0:13)\n"},
		{"- a\n\n  b\n", "para (3:3:7-4:0:8)\n"},
		{"> - a\n>\n>   b\n", "para (3:5:12-4:0:13)\n"},
	}
	for _, tt := range tests {
		got := string(ASTText(Parse([]byte(tt.src), WithSourcePos())))
		if !strings.Contains(got, tt.want) {
			t.Errorf("%q\n got %q\nwant it to contain %q", tt.src, got, tt.want)
		}
	}
}

// TestStackUnits pins the container stack (parser.go) on cases taken from
// the corpus: line splitting, the attribute container, ids where they
// stand, list tightness and laziness, and captions.
func TestStackUnits(t *testing.T) {
	t.Run("splitLines", func(t *testing.T) {
		pos := func(line, col, off int) SourcePos { return SourcePos{Line: line, Col: col, Offset: off} }
		cases := []struct {
			name  string
			input string
			want  []srcLine
		}{
			{"empty input", "", nil},
			{"no trailing newline", "a\nbc", []srcLine{
				{text: "a", pos: pos(1, 1, 0), eol: pos(2, 0, 1), newline: true},
				{text: "bc", pos: pos(2, 1, 2), eol: pos(2, 3, 4), newline: false, newlineBefore: 1},
			}},
			{"CRLF", "a\r\n\r\nb\r\n", []srcLine{
				{text: "a", pos: pos(1, 1, 0), eol: pos(2, 0, 2), newline: true},
				{text: "", pos: pos(2, 1, 3), eol: pos(3, 0, 4), newline: true, newlineBefore: 2},
				{text: "b", pos: pos(3, 1, 5), eol: pos(4, 0, 7), newline: true, newlineBefore: 4},
			}},
		}
		for _, c := range cases {
			got := splitLines(c.input)
			if fmt.Sprint(got) != fmt.Sprint(c.want) {
				t.Errorf("%s: splitLines(%q)\n got %+v\nwant %+v", c.name, c.input, got, c.want)
			}
		}
	})

	// Each case is a corpus case's input and expected HTML, verbatim. A case
	// with a renderer reason fails today for that reason only, outside the
	// block parser: it must give exactly today's HTML, which is the corpus's
	// but for that reason, so any other difference fails. Its mismatch with
	// the corpus is logged, and a match with the corpus fails so the mark is
	// dropped.
	html := []struct {
		name, from, input, want string
		renderer, today         string
	}{
		{"attributes-complete-on-one-line", "attributes.test L146",
			"{#id}\n{key=val}\n{.foo .bar}\n{key=val2}\n{.baz}\n{#id2}\nOkay\n",
			"<p id=\"id2\" key=\"val2\" class=\"foo bar baz\">Okay</p>\n", "", ""},
		{"attributes-continued-over-indented-lines", "attributes.test L124",
			"{#id .class\n  style=\"color:red\"}\nA paragraph\n",
			"<p id=\"id\" class=\"class\" style=\"color:red\">A paragraph</p>\n", "", ""},
		{"attributes-failed-become-a-paragraph", "zjot.test L5797",
			"{.a\n.b}\npara\n",
			"<p>{.a\n.b}\npara</p>\n", "", ""},
		{"attributes-failed-alone-in-a-paragraph", "zjot.test L5829",
			"{.a\n.b}\n\npara\n",
			"<p>{.a\n.b}</p>\n<p>para</p>\n", "", ""},
		{"ids-where-they-stand", "zjot.test L206",
			"# Foo bar\n\n{#Foo-bar}\nParagraph\n",
			"<section id=\"Foo-bar\">\n<h1>Foo bar</h1>\n<p id=\"Foo-bar\">Paragraph</p>\n</section>\n", "", ""},
		{"lists-attribute-after-item-is-the-next-paragraphs", "zjot.test L1528",
			"- item\n{.c}\npara\n",
			"<ul>\n<li>\nitem\n</li>\n</ul>\n<p class=\"c\">para</p>\n", "", ""},
		{"lists-blank-in-item-before-block-quote", "zjot.test L2506",
			"- a\n\n  - b\n\n  > q\n",
			"<ul>\n<li>\na\n<ul>\n<li>\nb\n</li>\n</ul>\n<blockquote>\nq\n</blockquote>\n</li>\n</ul>\n",
			"html.go drops <p> only for a tight item's own paragraphs, not for one in a block quote inside it; the tree is checked by lists-blank-in-item-before-block-quote-tree",
			"<ul>\n<li>\na\n<ul>\n<li>\nb\n</li>\n</ul>\n<blockquote>\n<p>q</p>\n</blockquote>\n</li>\n</ul>\n"},
		{"lists-lazy-text-through-nested-divs", "zjot.test L6843",
			"- ::: a\n  ::: b\n  x\n  :::\n  y\n  :::\nlazy\n",
			"<ul>\n<li>\n<div class=\"a\">\n<div class=\"b\">\n<p>x</p>\n</div>\n</div>\n<p>y\n:::\nlazy</p>\n</li>\n</ul>\n", "", ""},
		{"caption-after-blank-lines", "tables.test L35",
			"| a | b |\n\n^ With a _caption_\nand another line.\n",
			"<table>\n<caption>With a <em>caption</em>\nand another line.</caption>\n<tr>\n<td>a</td>\n<td>b</td>\n</tr>\n</table>\n", "", ""},
		{"caption-with-no-table-is-dropped", "zjot.test L7478",
			"^ alone\n\npara\n",
			"<p>para</p>\n", "", ""},
	}
	for _, c := range html {
		t.Run(c.name, func(t *testing.T) {
			got, err := HTML(Parse([]byte(c.input)))
			if err != nil {
				t.Fatal(err)
			}
			switch {
			case c.renderer != "" && string(got) == c.want:
				t.Errorf("%s now passes: drop its renderer reason", c.from)
			case c.renderer != "" && string(got) != c.today:
				t.Errorf("%s differs from today's HTML, so not only for its renderer reason (%s)\n---input---\n%s---today---\n%s---got---\n%s", c.from, c.renderer, c.input, c.today, got)
			case c.renderer != "":
				t.Logf("%s differs outside the block parser: %s\n---want---\n%s---got---\n%s", c.from, c.renderer, c.want, got)
			case string(got) != c.want:
				t.Errorf("%s\n---input---\n%s---want---\n%s---got---\n%s", c.from, c.input, c.want, got)
			}
		})
	}

	// Each case is an input and its expected AST text: verbatim from a
	// corpus "ap" case, or, where noted, the tree a corpus case's HTML
	// implies.
	ast := []struct {
		name, from, input, want string
		pos                     bool
	}{
		{"lists-tightness-across-a-blank-between-two-lists", "zjot.test L4407",
			"- [ ] task\n- [x] done\n\n- a\n- b\n",
			`doc
  task_list (1:1:0-4:1:23)
    task_list_item (1:1:0-2:0:10) checked=false
      para (1:7:6-2:0:10)
        str (1:7:6-1:10:9) text="task"
    task_list_item (2:1:11-4:0:22) checked=true
      para (2:7:17-3:0:21)
        str (2:7:17-2:10:20) text="done"
  bullet_list (4:1:23-6:0:30) tight=true style="-"
    list_item (4:1:23-5:0:26)
      para (4:3:25-5:0:26)
        str (4:3:25-4:3:25) text="a"
    list_item (5:1:27-6:0:30)
      para (5:3:29-6:0:30)
        str (5:3:29-5:3:29) text="b"
`, true},
		// The tree zjot.test L2506's HTML implies: the outer list stays
		// tight across the blank lines, and the quote is the outer item's.
		{"lists-blank-in-item-before-block-quote-tree", "zjot.test L2506 (implied)",
			"- a\n\n  - b\n\n  > q\n",
			`doc
  bullet_list tight=true style="-"
    list_item
      para
        str text="a"
      bullet_list tight=true style="-"
        list_item
          para
            str text="b"
      block_quote
        para
          str text="q"
`, false},
		// A later caption replaces an earlier one (djot.js issue #57); the
		// only replaced-caption case in the djot.js files.
		{"caption-replaced", "regression.test L46",
			"| 1 | 2 |\n\n ^ cap1\n\n ^ cap2\n",
			`doc
  table
    caption
      str text="cap2"
    row head=false
      cell head=false align="default"
        str text="1"
      cell head=false align="default"
        str text="2"
`, false},
	}
	for _, c := range ast {
		t.Run(c.name, func(t *testing.T) {
			var opts []ParseOption
			if c.pos {
				opts = append(opts, WithSourcePos())
			}
			got := string(ASTText(Parse([]byte(c.input), opts...)))
			if got != c.want {
				t.Errorf("%s\n---input---\n%s---want---\n%s---got---\n%s", c.from, c.input, c.want, got)
			}
		})
	}
}
