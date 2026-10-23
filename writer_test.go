package gjot

import (
	"os"
	"path/filepath"
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

func TestWriterRoundTripsCorpus(t *testing.T) {
	files, err := filepath.Glob("testdata/spec/*.test")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range parseSpecFile(string(content)) {
			if c.filter {
				continue
			}
			djot, html := roundTrip(t, c.input)
			if c.ast {
				want := string(ASTText(Parse([]byte(c.input))))
				if got := string(ASTText(Parse([]byte(djot)))); got != want {
					t.Errorf("%s:%d\n---input---\n%s---written---\n%s---expected ast---\n%s---got---\n%s", file, c.line, c.input, djot, want, got)
				}
				continue
			}
			if html != c.expected {
				t.Errorf("%s:%d\n---input---\n%s---written---\n%s---expected---\n%s---got---\n%s", file, c.line, c.input, djot, c.expected, html)
			}
		}
	}
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
