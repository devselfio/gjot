package ast

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"unsafe"
)

// Every kind in the inventory is a distinct type whose Tag is its key.
func TestNodeKinds(t *testing.T) {
	if len(nodeKinds) != 49 {
		t.Errorf("%d kinds, want 49", len(nodeKinds))
	}
	types := map[reflect.Type]Tag{}
	for tag, mk := range nodeKinds {
		n := mk()
		if got := n.Tag(); got != tag {
			t.Errorf("kind %q makes a %q", tag, got)
		}
		typ := reflect.TypeOf(n)
		if other, ok := types[typ]; ok {
			t.Errorf("kinds %q and %q make the same type %v", other, tag, typ)
		}
		types[typ] = tag
	}
}

// TestNodeSize shows each kind's size, and on a 64-bit platform fails if a
// common kind is 100 bytes or more.
func TestNodeSize(t *testing.T) {
	sizes := []struct {
		kind Node
		size uintptr
	}{
		{&Section{}, unsafe.Sizeof(Section{})},
		{&Heading{}, unsafe.Sizeof(Heading{})},
		{&Para{}, unsafe.Sizeof(Para{})},
		{&ThematicBreak{}, unsafe.Sizeof(ThematicBreak{})},
		{&CodeBlock{}, unsafe.Sizeof(CodeBlock{})},
		{&RawBlock{}, unsafe.Sizeof(RawBlock{})},
		{&BlockQuote{}, unsafe.Sizeof(BlockQuote{})},
		{&Div{}, unsafe.Sizeof(Div{})},
		{&BulletList{}, unsafe.Sizeof(BulletList{})},
		{&TaskList{}, unsafe.Sizeof(TaskList{})},
		{&OrderedList{}, unsafe.Sizeof(OrderedList{})},
		{&DefinitionList{}, unsafe.Sizeof(DefinitionList{})},
		{&ListItem{}, unsafe.Sizeof(ListItem{})},
		{&TaskListItem{}, unsafe.Sizeof(TaskListItem{})},
		{&DefinitionListItem{}, unsafe.Sizeof(DefinitionListItem{})},
		{&Term{}, unsafe.Sizeof(Term{})},
		{&Definition{}, unsafe.Sizeof(Definition{})},
		{&Table{}, unsafe.Sizeof(Table{})},
		{&Caption{}, unsafe.Sizeof(Caption{})},
		{&Row{}, unsafe.Sizeof(Row{})},
		{&Cell{}, unsafe.Sizeof(Cell{})},
		{&Footnote{}, unsafe.Sizeof(Footnote{})},
		{&Str{}, unsafe.Sizeof(Str{})},
		{&SoftBreak{}, unsafe.Sizeof(SoftBreak{})},
		{&HardBreak{}, unsafe.Sizeof(HardBreak{})},
		{&NonBreakingSpace{}, unsafe.Sizeof(NonBreakingSpace{})},
		{&Emph{}, unsafe.Sizeof(Emph{})},
		{&Strong{}, unsafe.Sizeof(Strong{})},
		{&Span{}, unsafe.Sizeof(Span{})},
		{&Mark{}, unsafe.Sizeof(Mark{})},
		{&Superscript{}, unsafe.Sizeof(Superscript{})},
		{&Subscript{}, unsafe.Sizeof(Subscript{})},
		{&Insert{}, unsafe.Sizeof(Insert{})},
		{&Delete{}, unsafe.Sizeof(Delete{})},
		{&DoubleQuoted{}, unsafe.Sizeof(DoubleQuoted{})},
		{&SingleQuoted{}, unsafe.Sizeof(SingleQuoted{})},
		{&Verbatim{}, unsafe.Sizeof(Verbatim{})},
		{&InlineMath{}, unsafe.Sizeof(InlineMath{})},
		{&DisplayMath{}, unsafe.Sizeof(DisplayMath{})},
		{&RawInline{}, unsafe.Sizeof(RawInline{})},
		{&URL{}, unsafe.Sizeof(URL{})},
		{&Email{}, unsafe.Sizeof(Email{})},
		{&FootnoteReference{}, unsafe.Sizeof(FootnoteReference{})},
		{&Symb{}, unsafe.Sizeof(Symb{})},
		{&Link{}, unsafe.Sizeof(Link{})},
		{&Image{}, unsafe.Sizeof(Image{})},
		{&Ellipsis{}, unsafe.Sizeof(Ellipsis{})},
		{&EmDash{}, unsafe.Sizeof(EmDash{})},
		{&EnDash{}, unsafe.Sizeof(EnDash{})},
	}
	if len(sizes) != len(nodeKinds) {
		t.Errorf("the table lists %d kinds, nodeKinds %d", len(sizes), len(nodeKinds))
	}
	var table strings.Builder
	for _, s := range sizes {
		fmt.Fprintf(&table, "\n  %-22s %3d", s.kind.Tag(), s.size)
	}
	t.Logf("kind sizes in bytes (unsafe.Sizeof):%s", table.String())
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("the 100-byte bound is for 64-bit platforms")
	}
	common := map[Tag]bool{"str": true, "para": true, "emph": true, "strong": true, "link": true, "image": true,
		"heading": true, "section": true, "list_item": true, "bullet_list": true, "span": true, "verbatim": true}
	for _, s := range sizes {
		if common[s.kind.Tag()] {
			delete(common, s.kind.Tag())
			if s.size >= 100 {
				t.Errorf("%s is %d bytes, want under 100", s.kind.Tag(), s.size)
			}
		}
	}
	for tag := range common {
		t.Errorf("common kind %s is not in the table", tag)
	}
}

// HeaderOf is the node's own header, so a change through it is a change
// to the node; WrittenID tells a missing id from an explicitly empty one;
// ClassList keeps the written class order.
func TestHeaderAccessors(t *testing.T) {
	for tag, mk := range nodeKinds {
		n := mk()
		h := HeaderOf(n)
		if h == nil {
			t.Fatalf("%s: HeaderOf is nil", tag)
		}
		h.Attrs = append(h.Attrs, Attr{Key: "class", Value: "k"})
		h.ID = "set"
		if !n.HasClass("k") || n.Attr("class") != "k" || HeaderOf(n).ID != "set" || HeaderOf(n) != h {
			t.Errorf("%s: a change through HeaderOf did not reach the node", tag)
		}
	}
	p := &Para{}
	if HeaderOf(p) != &p.Header {
		t.Error("HeaderOf(para) is not its embedded Header")
	}

	for _, tt := range []struct {
		attrs   []Attr
		id      string
		written bool
	}{
		{nil, "", false},
		{[]Attr{{Key: "class", Value: "a"}}, "", false},
		{[]Attr{{Key: "id", Value: ""}}, "", true},
		{[]Attr{{Key: "class", Value: "a"}, {Key: "id", Value: "x"}}, "x", true},
	} {
		h := &Header{ID: "generated", Attrs: tt.attrs}
		if id, written := h.WrittenID(); id != tt.id || written != tt.written {
			t.Errorf("WrittenID of %v = %q, %v; want %q, %v", tt.attrs, id, written, tt.id, tt.written)
		}
	}

	for _, tt := range []struct {
		class string
		want  []string
	}{
		{"", nil},
		{"b a", []string{"b", "a"}},
		{" c\tb  a\n", []string{"c", "b", "a"}},
	} {
		h := &Header{Attrs: []Attr{{Key: "id", Value: "i"}, {Key: "class", Value: tt.class}}}
		if got := h.ClassList(); !slices.Equal(got, tt.want) {
			t.Errorf("ClassList of %q = %q, want %q", tt.class, got, tt.want)
		}
	}
	if got := (&Header{}).ClassList(); len(got) != 0 {
		t.Errorf("ClassList with no class = %q, want none", got)
	}
}
