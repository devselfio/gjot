package gjot_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/devselfio/gjot"
	"github.com/devselfio/gjot/ast"
)

// The entry points' signatures, as a program outside the module sees them.
var (
	_ func([]byte, ...gjot.ParseOption) ast.Node              = gjot.Parse
	_ func([]byte, ...gjot.ParseOption) (ast.Node, error)     = gjot.ParseErr
	_ func() gjot.ParseOption                                 = gjot.WithSourcePos
	_ func(ast.Node, ...gjot.HTMLOption) ([]byte, error)      = gjot.HTML
	_ func(string, gjot.Hook) gjot.HTMLOption                 = gjot.WithHook
	_ func(ast.Node) []byte                                   = gjot.ASTText
	_ func(ast.Node) []byte                                   = gjot.Djot
	_ func(ast.Node, func(ast.Node) ast.WalkStatus)           = ast.Walk
	_ func([]byte) (ast.Node, error)                          = ast.UnmarshalNode
	_ func(ast.Node) *ast.Header                              = ast.HeaderOf
	_ func(ast.Node) []ast.Node                               = ast.Children
	_ func(io.Writer, ast.Node, bool) (ast.WalkStatus, error) = gjot.Hook(nil)
	_ int                                                     = gjot.MaxNesting
	_ error                                                   = gjot.ErrTooDeep
)

// TestPublicASTAPI uses gjot and gjot/ast as an outside program does: it
// parses with positions (every node but the root carries one), type-switches on the kinds, renders with a hook
// returning ast.WalkStatus, walks with a skip, and marshals and decodes the
// tree.
func TestPublicASTAPI(t *testing.T) {
	if gjot.MaxNesting != 1000 {
		t.Errorf("MaxNesting = %d, want 1000", gjot.MaxNesting)
	}
	src := []byte("{#t .c}\n# Title\n\n::: box\nin *box*\n:::\n\nSee [docs](u) and `code`.\n")
	doc := gjot.Parse(src, gjot.WithSourcePos())
	sec, ok := doc.(*ast.Section)
	if !ok || sec.Level != 0 || len(sec.Children) != 1 {
		t.Fatalf("Parse root = %#v, want an *ast.Section of level 0 holding the heading's section", doc)
	}

	var kinds []string
	ast.Walk(doc, func(n ast.Node) ast.WalkStatus {
		switch n := n.(type) {
		case *ast.Section:
			if n == doc {
				break
			}
			// The attributes before a heading are its section's.
			kinds = append(kinds, fmt.Sprintf("section %d %s", n.Level, n.ID))
			if id, written := n.WrittenID(); !written || id != "t" {
				t.Errorf("section WrittenID = %q, %v", id, written)
			}
			if got := n.ClassList(); len(got) != 1 || got[0] != "c" {
				t.Errorf("section ClassList = %q", got)
			}
		case *ast.Heading:
			kinds = append(kinds, fmt.Sprintf("heading %d", n.Level))
		case *ast.Div:
			kinds = append(kinds, "div")
			return ast.WalkSkipChildren
		case *ast.Link:
			kinds = append(kinds, "link "+*n.Destination)
		case *ast.Verbatim:
			kinds = append(kinds, "verbatim "+n.Text)
		case *ast.Emph:
			t.Error("Walk entered a skipped div")
		}
		if n != doc && ast.HeaderOf(n).Pos == nil {
			t.Errorf("%s has no position", n.Tag())
		}
		return ast.WalkContinue
	})
	if got, want := strings.Join(kinds, "; "), "section 1 t; heading 1; div; link u; verbatim code"; got != want {
		t.Errorf("walk saw %q, want %q", got, want)
	}

	var hook gjot.Hook = func(w io.Writer, n ast.Node, entering bool) (ast.WalkStatus, error) {
		if _, ok := n.(*ast.Div); !ok {
			t.Errorf("hook called with %T", n)
		}
		if entering {
			io.WriteString(w, "<box>")
			return ast.WalkSkipChildren, nil
		}
		io.WriteString(w, "</box>\n")
		return ast.WalkContinue, nil
	}
	html, err := gjot.HTML(doc, gjot.WithHook("box", hook))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(html), "<box></box>\n") || strings.Contains(string(html), "<em>") {
		t.Errorf("hooked HTML:\n%s", html)
	}
	failing := gjot.WithHook("box", func(io.Writer, ast.Node, bool) (ast.WalkStatus, error) {
		return ast.WalkContinue, errors.New("no")
	})
	if out, err := gjot.HTML(doc, failing); err == nil || out != nil {
		t.Errorf("HTML with a failing hook = %q, %v; want nil and the error", out, err)
	}

	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ast.UnmarshalNode(data)
	if err != nil {
		t.Fatal(err)
	}
	if string(gjot.ASTText(back)) != string(gjot.ASTText(doc)) {
		t.Errorf("decoded tree differs:\n%s", gjot.ASTText(back))
	}
	var para ast.Para
	if err := json.Unmarshal([]byte(`{"tag":"para","children":[{"tag":"str","text":"x"}]}`), &para); err != nil {
		t.Fatal(err)
	}
	if s, ok := ast.Children(&para)[0].(*ast.Str); !ok || s.Text != "x" {
		t.Errorf("para decoded as %#v", para)
	}
	if err := json.Unmarshal([]byte(`{"tag":"str","text":"x"}`), &para); err == nil {
		t.Error("a str decoded into an ast.Para")
	}

	deep := strings.Repeat("> ", gjot.MaxNesting+1) + "x\n"
	if tree, err := gjot.ParseErr([]byte(deep)); !errors.Is(err, gjot.ErrTooDeep) || tree == nil {
		t.Errorf("ParseErr past MaxNesting = %v, %v; want a tree and ErrTooDeep", tree, err)
	}
	if out := gjot.Djot(gjot.Parse([]byte("a *b*\n"))); string(out) != "a *b*\n" {
		t.Errorf("Djot = %q", out)
	}
}
