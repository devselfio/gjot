package ast

import (
	"reflect"
	"strings"
	"testing"
)

// Walk visits a node before its children, children in order, and skips
// a subtree when told to.
func TestWalkDocumentOrder(t *testing.T) {
	dest := "u"
	doc := &Section{Children: []Node{
		&Heading{Level: 1, Children: []Node{&Str{Text: "T"}}},
		&Para{Children: []Node{
			&Str{Text: "a"},
			&Emph{Children: []Node{&Str{Text: "b"}, &SoftBreak{}}},
			&Link{Destination: &dest, Children: []Node{&Str{Text: "c"}}},
			&FootnoteReference{Label: "n"},
		}},
		&BulletList{Children: []Node{
			&ListItem{Children: []Node{&Para{Children: []Node{&Str{Text: "d"}}}}},
		}},
		&Footnote{Label: "n", Children: []Node{&Para{Children: []Node{&Str{Text: "e"}}}}},
	}}
	name := func(n Node) string {
		if s, ok := n.(*Str); ok {
			return "str:" + s.Text
		}
		return string(n.Tag())
	}
	var got []string
	Walk(doc, func(n Node) WalkStatus {
		got = append(got, name(n))
		return WalkContinue
	})
	want := "section heading str:T para str:a emph str:b soft_break link str:c footnote_reference " +
		"bullet_list list_item para str:d footnote para str:e"
	if g := strings.Join(got, " "); g != want {
		t.Errorf("walk order\n got %s\nwant %s", g, want)
	}

	got = nil
	Walk(doc, func(n Node) WalkStatus {
		got = append(got, name(n))
		switch n.(type) {
		case *Emph, *BulletList:
			return WalkSkipChildren
		}
		return WalkContinue
	})
	want = "section heading str:T para str:a emph link str:c footnote_reference bullet_list footnote para str:e"
	if g := strings.Join(got, " "); g != want {
		t.Errorf("walk with skips\n got %s\nwant %s", g, want)
	}
}

// Children is each container's own child slice, in order and not copied,
// and nil for every leaf.
func TestChildren(t *testing.T) {
	leaves := map[Tag]bool{
		"thematic_break": true, "code_block": true, "raw_block": true, "str": true,
		"soft_break": true, "hard_break": true, "non_breaking_space": true,
		"verbatim": true, "inline_math": true, "display_math": true, "raw_inline": true,
		"url": true, "email": true, "footnote_reference": true, "symb": true,
		"ellipsis": true, "em_dash": true, "en_dash": true,
	}
	containers := 0
	for tag, mk := range nodeKinds {
		n := mk()
		f := reflect.ValueOf(n).Elem().FieldByName("Children")
		if leaves[tag] {
			if f.IsValid() {
				t.Errorf("%s has a Children field but is listed as a leaf", tag)
			}
			if c := Children(n); c != nil {
				t.Errorf("Children(%s) = %v, want nil", tag, c)
			}
			continue
		}
		containers++
		if !f.IsValid() {
			t.Fatalf("%s has no Children field", tag)
		}
		kids := []Node{&Str{Text: "a"}, &SoftBreak{}, &Str{Text: "b"}}
		f.Set(reflect.ValueOf(kids))
		got := Children(n)
		if len(got) != 3 || got[0] != kids[0] || got[1] != kids[1] || got[2] != kids[2] {
			t.Errorf("Children(%s) = %v, want the three children in order", tag, got)
			continue
		}
		got[1] = &HardBreak{}
		if kids[1].Tag() != "hard_break" {
			t.Errorf("Children(%s) is a copy: a change to it did not reach the node", tag)
		}
	}
	if containers+len(leaves) != 49 || containers != 31 {
		t.Errorf("%d containers and %d leaves, want 31 and 18", containers, len(leaves))
	}
}
