package gjot

import (
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

// A tab indents like four spaces.
func TestTabIndentation(t *testing.T) {
	for _, pair := range [][2]string{
		{"\t- item\n", "    - item\n"},
		{"- a\n\n\tb\n", "- a\n\n    b\n"},
		{"1. a\n\n\tb\n", "1. a\n\n    b\n"},
		{": term\n\n\tdef\n", ": term\n\n    def\n"},
		{"- a\n\t- b\n", "- a\n    - b\n"},
		{"\t> q\n", "    > q\n"},
		{"\t```\n\tx\n\t```\n", "    ```\n    x\n    ```\n"},
	} {
		if got, want := htmlOf(t, pair[0]), htmlOf(t, pair[1]); got != want {
			t.Errorf("%q\n got %q\nwant %q (as %q)", pair[0], got, want, pair[1])
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
