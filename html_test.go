package gjot

import (
	"strings"
	"testing"
)

// A tight bullet, ordered or task list renders every paragraph inside its
// items bare, however deep in block quotes, divs and definition lists, as
// djot.js does; the innermost list decides, and a definition list never
// does. These pin the shapes whether or not the corpus has them.
func TestTightListReachesNestedParagraphs(t *testing.T) {
	cases := []struct{ name, input, want string }{
		{"quote-only-block",
			"-\n  > q\n",
			"<ul>\n<li>\n<blockquote>\nq\n</blockquote>\n</li>\n</ul>\n"},
		{"quote-first-then-sublist",
			"- > q\n  - b\n",
			"<ul>\n<li>\n<blockquote>\nq\n</blockquote>\n<ul>\n<li>\nb\n</li>\n</ul>\n</li>\n</ul>\n"},
		{"quote-first-then-code",
			"- > q\n  ```\n  c\n  ```\n",
			"<ul>\n<li>\n<blockquote>\nq\n</blockquote>\n<pre><code>c\n</code></pre>\n</li>\n</ul>\n"},
		{"quote-after-sublist",
			"- a\n\n  - b\n  > q\n",
			"<ul>\n<li>\na\n<ul>\n<li>\nb\n</li>\n</ul>\n<blockquote>\nq\n</blockquote>\n</li>\n</ul>\n"},
		{"two-paragraphs-in-quote",
			"-\n  > q\n  >\n  > r\n",
			"<ul>\n<li>\n<blockquote>\nq\nr\n</blockquote>\n</li>\n</ul>\n"},
		{"div",
			"- ::: d\n  q\n  :::\n",
			"<ul>\n<li>\n<div class=\"d\">\nq\n</div>\n</li>\n</ul>\n"},
		{"definition",
			"- x\n\n  : t\n\n    d\n",
			"<ul>\n<li>\nx\n<dl>\n<dt>t</dt>\n<dd>\nd\n</dd>\n</dl>\n</li>\n</ul>\n"},
		{"loose-list-quote-keeps-p",
			"- a\n\n- > q\n",
			"<ul>\n<li>\n<p>a</p>\n</li>\n<li>\n<blockquote>\n<p>q</p>\n</blockquote>\n</li>\n</ul>\n"},
		{"blank-before-quote-makes-loose",
			"- a\n\n  > q\n- b\n",
			"<ul>\n<li>\n<p>a</p>\n<blockquote>\n<p>q</p>\n</blockquote>\n</li>\n<li>\n<p>b</p>\n</li>\n</ul>\n"},
		{"loose-list-in-quote-then-tight-item",
			"- > - a\n  >\n  > - b\n- d\n",
			"<ul>\n<li>\n<blockquote>\n<ul>\n<li>\n<p>a</p>\n</li>\n<li>\n<p>b</p>\n</li>\n</ul>\n</blockquote>\n</li>\n<li>\nd\n</li>\n</ul>\n"},
		{"tight-quote-then-loose-sublist",
			"- > q\n  - a\n\n    b\n  - c\n",
			"<ul>\n<li>\n<blockquote>\nq\n</blockquote>\n<ul>\n<li>\n<p>a</p>\n<p>b</p>\n</li>\n<li>\n<p>c</p>\n</li>\n</ul>\n</li>\n</ul>\n"},
		{"paragraph-after-tight-list",
			"- a\n\nb\n",
			"<ul>\n<li>\na\n</li>\n</ul>\n<p>b</p>\n"},
		{"top-level-definition-list",
			": t\n\n  d\n",
			"<dl>\n<dt>t</dt>\n<dd>\n<p>d</p>\n</dd>\n</dl>\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := HTML(Parse([]byte(c.input)))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != c.want {
				t.Errorf("---input---\n%s---want---\n%s---got---\n%s", c.input, c.want, got)
			}
		})
	}
}

// A task list's own class comes first and the node's classes join it in
// one class attribute (grid.test L770); the id and other attributes follow.
func TestTaskListMergesClass(t *testing.T) {
	cases := []struct{ name, input, want string }{
		{"grid.test L770", "{.x}\n- [x] done\n- [ ] todo\n",
			"<ul class=\"task-list x\">\n<li>\n<input disabled=\"\" type=\"checkbox\" checked=\"\"/>\ndone\n</li>\n<li>\n<input disabled=\"\" type=\"checkbox\"/>\ntodo\n</li>\n</ul>\n"},
		{"id-and-attr", "{#i .x .y k=v}\n- [ ] a\n",
			"<ul class=\"task-list x y\" id=\"i\" k=\"v\">\n<li>\n<input disabled=\"\" type=\"checkbox\"/>\na\n</li>\n</ul>\n"},
		{"no-class", "{#i}\n- [ ] a\n",
			"<ul class=\"task-list\" id=\"i\">\n<li>\n<input disabled=\"\" type=\"checkbox\"/>\na\n</li>\n</ul>\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := HTML(Parse([]byte(c.input)))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != c.want {
				t.Errorf("---input---\n%s---want---\n%s---got---\n%s", c.input, c.want, got)
			}
		})
	}
}

// An ordered list's start and type come before its written attributes, as
// djot.js (zjot.test L6455).
func TestOrderedListAttributeOrder(t *testing.T) {
	cases := []struct{ name, input, want string }{
		{"zjot.test L6455", "{#i .x k=v}\ni. a\n", "<ol type=\"i\" id=\"i\" class=\"x\" k=\"v\">\n<li>\na\n</li>\n</ol>\n"},
		{"start before id", "{#i}\n5) a\n", "<ol start=\"5\" id=\"i\">\n<li>\na\n</li>\n</ol>\n"},
		{"start then type", "b) x\n", "<ol start=\"2\" type=\"a\">\n<li>\nx\n</li>\n</ol>\n"},
		{"roman before alpha", "c) x\n", "<ol start=\"100\" type=\"i\">\n<li>\nx\n</li>\n</ol>\n"},
		{"roman start", "(iv) x\n", "<ol start=\"4\" type=\"i\">\n<li>\nx\n</li>\n</ol>\n"},
		{"decimal one", "1. x\n", "<ol>\n<li>\nx\n</li>\n</ol>\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := HTML(Parse([]byte(c.input)))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != c.want {
				t.Errorf("---input---\n%s---want---\n%s---got---\n%s", c.input, c.want, got)
			}
		})
	}
}

// Attributes render in source order, as djot.js: the id where an `id` was
// written (an empty one included, over a generated id), first when the
// document gave it; a class joins the intrinsic class of math, a symbol and
// a task list; autolinks and footnote references take attributes too
// (zjot.test L5933-L6003, L6941, L10336, L10367).
func TestAttributeOrder(t *testing.T) {
	cases := []struct{ name, input, want string }{
		{"block key id class", "{k=v #i .c}\n> q\n", "<blockquote k=\"v\" id=\"i\" class=\"c\">\n<p>q</p>\n</blockquote>\n"},
		{"section keeps order", "{k=v #i}\n# H\n", "<section k=\"v\" id=\"i\">\n<h1>H</h1>\n</section>\n"},
		{"generated id first", "# h{k=v}\n", "<section id=\"h\">\n<h1><span k=\"v\">h</span></h1>\n</section>\n"},
		{"empty id on a heading", "{id=\"\"}\n# h\n", "<section id=\"h\">\n<h1 id=\"\">h</h1>\n</section>\n"},
		{"empty id over a generated one", "> {id=\"\"}\n> # h\n", "<blockquote>\n<h1 id=\"\">h</h1>\n</blockquote>\n"},
		{"inline order", "[t](u){k=v #i .c}\n", "<p><a href=\"u\" k=\"v\" id=\"i\" class=\"c\">t</a></p>\n"},
		{"math", "$`m`{#i .a k=v}\n", "<p><span class=\"math inline a\" id=\"i\" k=\"v\">\\(m\\)</span></p>\n"},
		{"symbol", ":sym:{.a #i}\n", "<p><span class=\"symbol a\" id=\"i\">sym</span></p>\n"},
		{"autolink", "<http://x>{#i k=v}\n", "<p><a href=\"http://x\" id=\"i\" k=\"v\">http://x</a></p>\n"},
		{"email", "<a@b.c>{.a}\n", "<p><a href=\"mailto:a@b.c\" class=\"a\">a@b.c</a></p>\n"},
		{"footnote reference", "x[^n]{.a #i}\n\n[^n]: n\n", "<p>x<a id=\"fnref1\" href=\"#fn1\" role=\"doc-noteref\" class=\"a\"><sup>1</sup></a></p>\n<section role=\"doc-endnotes\">\n<hr>\n<ol>\n<li id=\"fn1\">\n<p>n<a href=\"#fnref1\" role=\"doc-backlink\">\u21a9\ufe0e</a></p>\n</li>\n</ol>\n</section>\n"},
		{"div fence class joins last", "{.x}\n::: y\nz\n:::\n", "<div class=\"x y\">\n<p>z</p>\n</div>\n"},
		{"an id an empty id overrides was read", "[a]{#y}{id=\"\"}\n\n# y\n", "<p><span id=\"\">a</span></p>\n<section id=\"y-1\">\n<h1>y</h1>\n</section>\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := HTML(Parse([]byte(c.input)))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != c.want {
				t.Errorf("---input---\n%s---want---\n%s---got---\n%s", c.input, c.want, got)
			}
		})
	}
}

// Endnotes are numbered as djot.js's HTML renderer numbers them: body
// references in reading order, then every definition's references in
// JavaScript's key order, array-index labels ascending first (zjot.test
// L10160, L10192; internal/djot's TestJSArrayIndex checks which labels are
// array indexes). A definition nothing references renders nothing; a
// reference to no definition gets an endnote with only its backlink; the
// endnotes follow the whole document, not the definition's place.
func TestFootnoteEndnotes(t *testing.T) {
	backlink := func(n string) string {
		return "<a href=\"#fnref" + n + "\" role=\"doc-backlink\">↩︎</a>"
	}
	cases := []struct{ name, input, want string }{
		{"unreferenced definition", "[^n]: note\n\npara\n", "<p>para</p>\n"},
		{"reference to no definition", "a[^n]\n", "<p>a<a id=\"fnref1\" href=\"#fn1\" role=\"doc-noteref\"><sup>1</sup></a></p>\n<section role=\"doc-endnotes\">\n<hr>\n<ol>\n<li id=\"fn1\">\n<p>" + backlink("1") + "</p>\n</li>\n</ol>\n</section>\n"},
		{"endnotes after the document", "[^n]: note\n\n# H\n\na[^n]\n", "<p>a<a id=\"fnref1\" href=\"#fn1\" role=\"doc-noteref\"><sup>1</sup></a></p>\n<section role=\"doc-endnotes\">\n<hr>\n<ol>\n<li id=\"fn1\">\n<p>note" + backlink("1") + "</p>\n</li>\n</ol>\n</section>\n"},
	}
	cases[2].want = "<section id=\"H\">\n<h1>H</h1>\n" + cases[2].want[:strings.Index(cases[2].want, "<section role")] + "</section>\n" + cases[2].want[strings.Index(cases[2].want, "<section role"):]
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := HTML(Parse([]byte(c.input)))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != c.want {
				t.Errorf("---input---\n%s---want---\n%s---got---\n%s", c.input, c.want, got)
			}
		})
	}
}
