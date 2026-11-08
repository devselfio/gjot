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

	// Each case is a corpus case's input and expected HTML, verbatim.
	html := []struct {
		name, from, input, want string
	}{
		{"attributes-complete-on-one-line", "attributes.test L146",
			"{#id}\n{key=val}\n{.foo .bar}\n{key=val2}\n{.baz}\n{#id2}\nOkay\n",
			"<p id=\"id2\" key=\"val2\" class=\"foo bar baz\">Okay</p>\n"},
		{"attributes-continued-over-indented-lines", "attributes.test L124",
			"{#id .class\n  style=\"color:red\"}\nA paragraph\n",
			"<p id=\"id\" class=\"class\" style=\"color:red\">A paragraph</p>\n"},
		{"attributes-failed-become-a-paragraph", "zjot.test L5797",
			"{.a\n.b}\npara\n",
			"<p>{.a\n.b}\npara</p>\n"},
		{"attributes-failed-alone-in-a-paragraph", "zjot.test L5829",
			"{.a\n.b}\n\npara\n",
			"<p>{.a\n.b}</p>\n<p>para</p>\n"},
		{"ids-where-they-stand", "zjot.test L206",
			"# Foo bar\n\n{#Foo-bar}\nParagraph\n",
			"<section id=\"Foo-bar\">\n<h1>Foo bar</h1>\n<p id=\"Foo-bar\">Paragraph</p>\n</section>\n"},
		{"lists-attribute-after-item-is-the-next-paragraphs", "zjot.test L1528",
			"- item\n{.c}\npara\n",
			"<ul>\n<li>\nitem\n</li>\n</ul>\n<p class=\"c\">para</p>\n"},
		{"lists-blank-in-item-before-block-quote", "zjot.test L2506",
			"- a\n\n  - b\n\n  > q\n",
			"<ul>\n<li>\na\n<ul>\n<li>\nb\n</li>\n</ul>\n<blockquote>\nq\n</blockquote>\n</li>\n</ul>\n"},
		{"lists-lazy-text-through-nested-divs", "zjot.test L6843",
			"- ::: a\n  ::: b\n  x\n  :::\n  y\n  :::\nlazy\n",
			"<ul>\n<li>\n<div class=\"a\">\n<div class=\"b\">\n<p>x</p>\n</div>\n</div>\n<p>y\n:::\nlazy</p>\n</li>\n</ul>\n"},
		{"caption-after-blank-lines", "tables.test L35",
			"| a | b |\n\n^ With a _caption_\nand another line.\n",
			"<table>\n<caption>With a <em>caption</em>\nand another line.</caption>\n<tr>\n<td>a</td>\n<td>b</td>\n</tr>\n</table>\n"},
		{"caption-with-no-table-is-dropped", "zjot.test L7478",
			"^ alone\n\npara\n",
			"<p>para</p>\n"},
	}
	for _, c := range html {
		t.Run(c.name, func(t *testing.T) {
			got, err := HTML(Parse([]byte(c.input)))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != c.want {
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

// positionedBlocks are the block tags that carry Node.Pos under
// WithSourcePos (issue #16), at any depth.
var positionedBlocks = []Tag{
	BlockQuote, Heading, Section, CodeBlock, RawBlock, Div, ThematicBreak,
	Table, Row, Cell, Caption, DefinitionList, DefinitionListItem,
	BulletList, OrderedList, TaskList, ListItem, TaskListItem,
}

// TestEveryBlockNodeHasPosition parses one document holding each
// positioned block kind at top level, inside a block quote and inside a
// list item, and fails on any of them without a position. A definition
// list item's term and definition carry none, as zjot's AST prints none.
func TestEveryBlockNodeHasPosition(t *testing.T) {
	const blocks = "# Head\n\n> quote\n\n```\ncode\n```\n\n``` =html\n<b>\n```\n\n" +
		"::: d\nin div\n:::\n\n***\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n^ cap\n\n" +
		": term\n\n  def\n\n- bullet\n\n1. ordered\n\n- [ ] task\n"
	prefix := func(s, p, blank string) string {
		var b strings.Builder
		for _, l := range strings.SplitAfter(s, "\n") {
			switch l {
			case "":
			case "\n":
				b.WriteString(blank + "\n")
			default:
				b.WriteString(p + l)
			}
		}
		return b.String()
	}
	input := blocks + "\n" + prefix(blocks, "> ", ">") + "\n- item\n\n" + prefix(blocks, "  ", "")

	// where records, per tag, whether it was seen at top level (outside any
	// block quote or list item), inside a block quote and inside a list item.
	type seen struct{ top, quote, item bool }
	where := map[Tag]*seen{}
	var walk func(n *Node, inQuote, inItem bool)
	walk = func(n *Node, inQuote, inItem bool) {
		s := where[n.Tag]
		if s == nil {
			s = &seen{}
			where[n.Tag] = s
		}
		s.top = s.top || (!inQuote && !inItem)
		s.quote = s.quote || inQuote
		s.item = s.item || inItem
		switch n.Tag {
		case Term, Definition:
			if n.Pos != nil {
				t.Errorf("%s has a position %+v; zjot prints none", n.Tag, *n.Pos)
			}
		default:
			for _, tag := range positionedBlocks {
				if n.Tag == tag && n.Pos == nil {
					t.Errorf("%s (quote=%v item=%v) has no position", n.Tag, inQuote, inItem)
				}
			}
		}
		for _, c := range n.Children {
			walk(c, inQuote || n.Tag == BlockQuote, inItem || n.Tag == ListItem)
		}
	}
	// The root is a section printed as doc, without a position.
	for _, c := range Parse([]byte(input), WithSourcePos()).Children {
		walk(c, false, false)
	}
	for _, tag := range positionedBlocks {
		s := where[tag]
		switch {
		case s == nil:
			t.Errorf("%s does not occur in the document", tag)
		case tag == Section:
			// Sections are built at the document level only.
		case !s.top || !s.quote || !s.item:
			t.Errorf("%s occurs top=%v, in a quote=%v, in an item=%v; want all three", tag, s.top, s.quote, s.item)
		}
	}
	for _, tag := range []Tag{Term, Definition} {
		if where[tag] == nil {
			t.Errorf("%s does not occur in the document", tag)
		}
	}

	var none func(n *Node)
	none = func(n *Node) {
		if n.Pos != nil {
			t.Errorf("without WithSourcePos, %s has a position %+v", n.Tag, *n.Pos)
		}
		for _, c := range n.Children {
			none(c)
		}
	}
	none(Parse([]byte(input)))
}

// TestBlockEnds pins where blocks end, as exact AST text with positions,
// so a moved end fails here as well as in the corpus.
func TestBlockEnds(t *testing.T) {
	cases := []struct {
		name, from, input, want string
	}{
		{"quote-in-item-ends-on-blank", "issue #16 rule, no corpus case",
			"- > q\n\n  b\n",
			`doc
  bullet_list (1:1:0-4:0:10) tight=false style="-"
    list_item (1:1:0-4:0:10)
      block_quote (1:3:2-3:0:6)
        para (1:5:4-2:0:5)
          str (1:5:4-1:5:4) text="q"
      para (3:3:9-4:0:10)
        str (3:3:9-3:3:9) text="b"
`},
		{"caption-ends-at-last-line-newline", "zjot.test L3838",
			"| a |\n|---|\n| 1 |\n\n^ Cap one\n  two\n\npara\n",
			`doc
  table (1:1:0-5:0:18)
    caption (5:3:21-7:0:34)
      str (5:3:21-5:9:27) text="Cap one"
      soft_break (6:0:28-6:0:28)
      str (6:3:31-6:5:33) text="two"
    row (1:1:0-2:0:5) head=true
      cell (1:1:0-1:5:4) head=true align="default"
        str (1:3:2-1:3:2) text="a"
    row (3:1:12-4:0:17) head=false
      cell (3:1:12-3:5:16) head=false align="default"
        str (3:3:14-3:3:14) text="1"
  para (8:1:36-9:0:40)
    str (8:1:36-8:4:39) text="para"
`},
		{"definition-list-closed-by-div-fence-ends-past-it", "zjot.test L4274",
			":::\n: t\n\n  d\n:::\n",
			`doc
  div (1:1:0-5:3:15)
    definition_list (2:1:4-6:0:16)
      definition_list_item (2:1:4-5:3:15)
        term
          str (2:3:6-2:3:6) text="t"
        definition
          para (4:3:11-5:0:12)
            str (4:3:11-4:3:11) text="d"
`},
		{"heading-soft-break-across-bare-marker-line", "zjot.test L9563",
			"# a\n#\n# b\n",
			`doc
  section (1:1:0-4:0:9) id="a-b"
    heading (1:1:0-4:0:9) level=1
      str (1:3:2-1:3:2) text="a"
      soft_break (2:0:3-2:0:3)
      str (3:3:8-3:3:8) text="b"
`},
		{"code-block-in-block-quote", "grid.test L310",
			"> ```\n> code\n> ```\n",
			`doc
  block_quote (1:1:0-4:0:18)
    code_block (1:3:2-3:5:17) text="code\n"
`},
		{"thematic-break-in-block-quote", "grid.test L223",
			"> ***\n",
			`doc
  block_quote (1:1:0-2:0:5)
    thematic_break (1:3:2-2:0:5)
`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := string(ASTText(Parse([]byte(c.input), WithSourcePos())))
			if got != c.want {
				t.Errorf("%s\n---input---\n%s---want---\n%s---got---\n%s", c.from, c.input, c.want, got)
			}
		})
	}
}

// sectionIDs is the ids of the sections in html, in order.
func sectionIDs(html string) []string {
	var ids []string
	for rest := html; ; {
		i := strings.Index(rest, "<section id=\"")
		if i < 0 {
			return ids
		}
		rest = rest[i+len("<section id=\""):]
		j := strings.IndexByte(rest, '"')
		ids = append(ids, rest[:j])
		rest = rest[j:]
	}
}

func TestHeadingIDFromText(t *testing.T) {
	tests := []struct {
		src  string
		want []string
	}{
		{"# [x](u)\n", []string{"x"}},
		{"# `code` and $`m` [l](u)\n", []string{"code-and-m-l"}},
		{"# a\\\nb\n", []string{"a-b"}},
		{"# Don't panic\n", []string{"Don't-panic"}},
		{"# a--b c---d\n", []string{"a--b-c---d"}},
		{"# a...b\n", []string{"a-b"}},
		{"# -a-\n", []string{"-a-"}},
		{"# .a\n", []string{"a"}},
		{"# a\\_b\n", []string{"a_b"}},
		{"# é ü\n", []string{"é-ü"}},
		{"# 　c d \n", []string{"c-d"}},
		{"# a\\ b\n", []string{"ab"}},
		{"# :sym: b\n", []string{"b"}},
		{"# Intro[^n]\n\n[^n]: note\n", []string{"Intro"}},
		{"# [Foo]{#Foo}\n\n# Foo\n", []string{"Foo-1", "Foo-2"}},
		{"#\n\n#\n\n# a\n\n# a\n\n# a\n", []string{"s-1", "s-2", "a", "a-1", "a-2"}},
		// A quote carrying attributes keeps the source it was written as.
		{"# '{.c}\n", []string{"'"}},
		{"# \"{.c}\n", []string{"&quot;"}},
	}
	for _, tt := range tests {
		html := htmlOf(t, tt.src)
		if got := sectionIDs(html); fmt.Sprint(got) != fmt.Sprint(tt.want) {
			t.Errorf("%q: section ids %q, want %q\n%s", tt.src, got, tt.want, html)
		}
	}
	if html := htmlOf(t, "#\n\n[][]\n"); !strings.Contains(html, "<a></a>") {
		t.Errorf("[][] after an empty heading resolved:\n%s", html)
	}
	for _, src := range []string{"# '{.c}\n\n['][]\n", "# \"{.c}\n\n[\"][]\n"} {
		if html := htmlOf(t, src); !strings.Contains(html, `href="#`) {
			t.Errorf("%q: the heading's implicit reference did not resolve:\n%s", src, html)
		}
	}
}
