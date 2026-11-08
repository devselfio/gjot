package gjot

import "testing"

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
