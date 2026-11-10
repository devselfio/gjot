package gjot

import (
	"errors"
	"fmt"
	"io"
	"testing"
)

func renderWith(t *testing.T, src string, opts ...HTMLOption) string {
	t.Helper()
	out, err := HTML(Parse([]byte(src)), opts...)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestHookReplacesDivByClass(t *testing.T) {
	chess := func(w io.Writer, n Node, entering bool) (WalkStatus, error) {
		if entering {
			fmt.Fprintf(w, "<chess-board fen=%q></chess-board>\n", n.Attr("fen"))
		}
		return WalkSkipChildren, nil
	}
	got := renderWith(t, "{fen=abc}\n::: chess\nignored *text*\n:::\n", WithHook("chess", chess))
	if want := "<chess-board fen=\"abc\"></chess-board>\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestHookWrapsChildrenAndNests(t *testing.T) {
	wrap := func(open, close string) Hook {
		return func(w io.Writer, n Node, entering bool) (WalkStatus, error) {
			if entering {
				io.WriteString(w, open)
			} else {
				io.WriteString(w, close)
			}
			return WalkContinue, nil
		}
	}
	got := renderWith(t, "::: note\ntext [x]{.kbd} more\n:::\n",
		WithHook("note", wrap("<aside>\n", "</aside>\n")), WithHook("kbd", wrap("<kbd>", "</kbd>")))
	if want := "<aside>\n<p>text <kbd>x</kbd> more</p>\n</aside>\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A hook stands in for the div only: the enclosing tight list still renders
// the paragraphs inside it bare, as it does an unhooked div's.
func TestHookInTightItemKeepsListTightness(t *testing.T) {
	aside := func(w io.Writer, n Node, entering bool) (WalkStatus, error) {
		if entering {
			io.WriteString(w, "<aside>\n")
		} else {
			io.WriteString(w, "</aside>\n")
		}
		return WalkContinue, nil
	}
	got := renderWith(t, "- ::: d\n  q\n  :::\n", WithHook("d", aside))
	if want := "<ul>\n<li>\n<aside>\nq\n</aside>\n</li>\n</ul>\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestHookUnregisteredClassRendersDefault(t *testing.T) {
	got := renderWith(t, "{fen=abc}\n::: chess\n:::\n", WithHook("other", func(io.Writer, Node, bool) (WalkStatus, error) {
		t.Error("hook for another class called")
		return WalkContinue, nil
	}))
	if want := "<div fen=\"abc\" class=\"chess\">\n</div>\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestHookErrorAbortsRender(t *testing.T) {
	boom := errors.New("boom")
	_, err := HTML(Parse([]byte("::: x\n:::\n")), WithHook("x", func(io.Writer, Node, bool) (WalkStatus, error) {
		return WalkContinue, boom
	}))
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
}

func TestNodeHasClassAndAttr(t *testing.T) {
	doc := Parse([]byte("{#id .a .b k=v}\n::: c\n:::\n"))
	div := doc.(*Section).Children[0]
	// The fence class joins the block attribute's classes, as djot.js
	// (zjot.test L6431), so every one of them is found.
	for _, c := range []string{"a", "b", "c"} {
		if !div.HasClass(c) {
			t.Errorf("HasClass(%q) = false", c)
		}
	}
	if div.HasClass("d") {
		t.Error("HasClass(\"d\") = true for a class the div has not")
	}
	if got := div.Attr("k"); got != "v" {
		t.Errorf("Attr(k) = %q, want v", got)
	}
	if got := div.Attr("missing"); got != "" {
		t.Errorf("Attr(missing) = %q, want empty", got)
	}
}
