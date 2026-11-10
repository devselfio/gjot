package gjot

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"unicode/utf8"
)

// jsonNode is the wire form of every kind: the old Node's JSON fields, field
// for field and in its order, so the tree marshals byte for byte as the old
// Node does and a document marshalled by an earlier gjot decodes. A kind
// writes only its own fields; the wire names stay the old ones, so an ordered
// list's Marker, a footnote's and a footnote reference's Label and a symbol's
// Alias are "text", and a raw block's or raw inline's Format is "lang".
type jsonNode struct {
	Tag         Tag        `json:"tag"`
	Children    []jsonNode `json:"children,omitempty"`
	Text        string     `json:"text,omitempty"`
	Level       int        `json:"level,omitempty"`
	ID          string     `json:"id,omitempty"`
	Lang        string     `json:"lang,omitempty"`
	Destination *string    `json:"destination,omitempty"`
	Reference   string     `json:"reference,omitempty"`
	Attrs       []Attr     `json:"attrs,omitempty"`
	Tight       bool       `json:"tight,omitempty"`
	Checked     *bool      `json:"checked,omitempty"`
	Head        bool       `json:"head,omitempty"`
	Align       CellAlign  `json:"align,omitempty"`
	Style       string     `json:"style,omitempty"`
	AutoID      bool       `json:"auto_id,omitempty"`
	Source      string     `json:"source,omitempty"`
	Pos         *Range     `json:"pos,omitempty"`
}

// wireFields fills w with n's own fields (its children aside) as they go on
// the wire.
func wireFields(w *jsonNode, n Node) {
	h := n.header()
	w.Tag, w.ID, w.Attrs, w.Pos = n.Tag(), h.ID, h.Attrs, h.Pos
	switch n := n.(type) {
	case *Section:
		w.Level, w.AutoID = n.Level, n.AutoID
	case *Heading:
		w.Level, w.AutoID = n.Level, n.AutoID
	case *CodeBlock:
		w.Lang, w.Text = n.Lang, n.Text
	case *RawBlock:
		w.Lang, w.Text = n.Format, n.Text
	case *BulletList:
		w.Style, w.Tight = n.Style, n.Tight
	case *TaskList:
		w.Style, w.Tight = n.Style, n.Tight
	case *OrderedList:
		w.Style, w.Text, w.Tight = n.Style, n.Marker, n.Tight
	case *TaskListItem:
		w.Checked = &n.Checked
	case *Row:
		w.Head = n.Head
	case *Cell:
		w.Head, w.Align = n.Head, n.Align
	case *Footnote:
		w.Text = n.Label
	case *Str:
		w.Text, w.Source = n.Text, n.Source
	case *Verbatim:
		w.Text = n.Text
	case *InlineMath:
		w.Text = n.Text
	case *DisplayMath:
		w.Text = n.Text
	case *RawInline:
		w.Lang, w.Text = n.Format, n.Text
	case *URL:
		w.Text = n.Text
	case *Email:
		w.Text = n.Text
	case *FootnoteReference:
		w.Text = n.Label
	case *Symb:
		w.Text = n.Alias
	case *Link:
		w.Destination, w.Reference = n.Destination, n.Reference
	case *Image:
		w.Destination, w.Reference = n.Destination, n.Reference
	}
}

// appendJSON appends n's JSON form: jsonNode's fields in its order with its
// omitempty rules, children recursed into here rather than through nested
// MarshalJSON calls, which encoding/json would re-validate at every level.
func appendJSON(dst []byte, n Node) []byte {
	var w jsonNode
	wireFields(&w, n)
	dst = append(dst, `{"tag":`...)
	dst = appendJSONString(dst, string(w.Tag))
	if children := childrenOf(n); len(children) > 0 {
		dst = append(dst, `,"children":[`...)
		for i, c := range children {
			if i > 0 {
				dst = append(dst, ',')
			}
			dst = appendJSON(dst, c)
		}
		dst = append(dst, ']')
	}
	if w.Text != "" {
		dst = append(dst, `,"text":`...)
		dst = appendJSONString(dst, w.Text)
	}
	if w.Level != 0 {
		dst = append(dst, `,"level":`...)
		dst = strconv.AppendInt(dst, int64(w.Level), 10)
	}
	if w.ID != "" {
		dst = append(dst, `,"id":`...)
		dst = appendJSONString(dst, w.ID)
	}
	if w.Lang != "" {
		dst = append(dst, `,"lang":`...)
		dst = appendJSONString(dst, w.Lang)
	}
	if w.Destination != nil {
		dst = append(dst, `,"destination":`...)
		dst = appendJSONString(dst, *w.Destination)
	}
	if w.Reference != "" {
		dst = append(dst, `,"reference":`...)
		dst = appendJSONString(dst, w.Reference)
	}
	if len(w.Attrs) > 0 {
		dst = append(dst, `,"attrs":[`...)
		for i, a := range w.Attrs {
			if i > 0 {
				dst = append(dst, ',')
			}
			dst = append(dst, `{"key":`...)
			dst = appendJSONString(dst, a.Key)
			dst = append(dst, `,"value":`...)
			dst = appendJSONString(dst, a.Value)
			dst = append(dst, '}')
		}
		dst = append(dst, ']')
	}
	if w.Tight {
		dst = append(dst, `,"tight":true`...)
	}
	if w.Checked != nil {
		dst = append(dst, `,"checked":`...)
		dst = strconv.AppendBool(dst, *w.Checked)
	}
	if w.Head {
		dst = append(dst, `,"head":true`...)
	}
	if w.Align != "" {
		dst = append(dst, `,"align":`...)
		dst = appendJSONString(dst, string(w.Align))
	}
	if w.Style != "" {
		dst = append(dst, `,"style":`...)
		dst = appendJSONString(dst, w.Style)
	}
	if w.AutoID {
		dst = append(dst, `,"auto_id":true`...)
	}
	if w.Source != "" {
		dst = append(dst, `,"source":`...)
		dst = appendJSONString(dst, w.Source)
	}
	if w.Pos != nil {
		dst = append(dst, `,"pos":{"start":`...)
		dst = appendJSONPos(dst, w.Pos.Start)
		dst = append(dst, `,"end":`...)
		dst = appendJSONPos(dst, w.Pos.End)
		dst = append(dst, '}')
	}
	return append(dst, '}')
}

func appendJSONPos(dst []byte, p SourcePos) []byte {
	dst = append(dst, `{"line":`...)
	dst = strconv.AppendInt(dst, int64(p.Line), 10)
	dst = append(dst, `,"col":`...)
	dst = strconv.AppendInt(dst, int64(p.Col), 10)
	dst = append(dst, `,"offset":`...)
	dst = strconv.AppendInt(dst, int64(p.Offset), 10)
	return append(dst, '}')
}

const jsonHex = "0123456789abcdef"

// appendJSONString appends s quoted as encoding/json quotes a string: \" \\
// \b \f \n \r \t, other control bytes as a six-character \u escape, < > &
// escaped the same way (HTML-safe), U+2028 and U+2029 escaped, and each byte
// of invalid UTF-8 as the escaped replacement character U+FFFD.
func appendJSONString(dst []byte, s string) []byte {
	dst = append(dst, '"')
	start := 0
	for i := 0; i < len(s); {
		if b := s[i]; b < utf8.RuneSelf {
			if b >= ' ' && b != '"' && b != '\\' && b != '<' && b != '>' && b != '&' {
				i++
				continue
			}
			dst = append(dst, s[start:i]...)
			switch b {
			case '"', '\\':
				dst = append(dst, '\\', b)
			case '\b':
				dst = append(dst, '\\', 'b')
			case '\f':
				dst = append(dst, '\\', 'f')
			case '\n':
				dst = append(dst, '\\', 'n')
			case '\r':
				dst = append(dst, '\\', 'r')
			case '\t':
				dst = append(dst, '\\', 't')
			default:
				dst = append(dst, '\\', 'u', '0', '0', jsonHex[b>>4], jsonHex[b&0xF])
			}
			i++
			start = i
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			dst = append(dst, s[start:i]...)
			dst = append(dst, '\\', 'u', 'f', 'f', 'f', 'd')
			i += size
			start = i
			continue
		}
		if r == 0x2028 || r == 0x2029 { // LINE SEPARATOR, PARAGRAPH SEPARATOR
			dst = append(dst, s[start:i]...)
			dst = append(dst, '\\', 'u', '2', '0', '2', jsonHex[r&0xF])
			i += size
			start = i
			continue
		}
		i += size
	}
	dst = append(dst, s[start:]...)
	return append(dst, '"')
}

// UnmarshalNode decodes one node of any kind from its JSON form,
// switching on "tag"; an unknown or missing tag is an error. A document
// marshalled by an earlier gjot decodes: a key no kind has (its "classes")
// is ignored, as is a field the node's kind does not carry ("auto_id" is
// read on headings and sections only).
func UnmarshalNode(data []byte) (Node, error) {
	var w jsonNode
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, err
	}
	return fromWire(&w)
}

var errNoTag = errors.New(`gjot: JSON node has no "tag"`)

// fromWire builds the kind w's tag names and fills its fields from w,
// recursing through w's children.
func fromWire(w *jsonNode) (Node, error) {
	if w.Tag == "" {
		return nil, errNoTag
	}
	mk, ok := nodeKinds[w.Tag]
	if !ok {
		return nil, fmt.Errorf("gjot: JSON node has unknown tag %q", w.Tag)
	}
	n := mk()
	h := n.header()
	h.ID, h.Attrs, h.Pos = w.ID, w.Attrs, w.Pos
	var children []Node
	if len(w.Children) > 0 {
		children = make([]Node, len(w.Children))
		for i := range w.Children {
			c, err := fromWire(&w.Children[i])
			if err != nil {
				return nil, err
			}
			children[i] = c
		}
	}
	switch n := n.(type) {
	case *Section:
		n.Level, n.AutoID, n.Children = w.Level, w.AutoID, children
	case *Heading:
		n.Level, n.AutoID, n.Children = w.Level, w.AutoID, children
	case *Para:
		n.Children = children
	case *CodeBlock:
		n.Lang, n.Text = w.Lang, w.Text
	case *RawBlock:
		n.Format, n.Text = w.Lang, w.Text
	case *BlockQuote:
		n.Children = children
	case *Div:
		n.Children = children
	case *BulletList:
		n.Style, n.Tight, n.Children = w.Style, w.Tight, children
	case *TaskList:
		n.Style, n.Tight, n.Children = w.Style, w.Tight, children
	case *OrderedList:
		n.Style, n.Marker, n.Tight, n.Children = w.Style, w.Text, w.Tight, children
	case *DefinitionList:
		n.Children = children
	case *ListItem:
		n.Children = children
	case *TaskListItem:
		n.Checked, n.Children = w.Checked != nil && *w.Checked, children
	case *DefinitionListItem:
		n.Children = children
	case *Term:
		n.Children = children
	case *Definition:
		n.Children = children
	case *Table:
		n.Children = children
	case *Caption:
		n.Children = children
	case *Row:
		n.Head, n.Children = w.Head, children
	case *Cell:
		n.Head, n.Align, n.Children = w.Head, w.Align, children
	case *Footnote:
		n.Label, n.Children = w.Text, children
	case *Str:
		n.Text, n.Source = w.Text, w.Source
	case *Emph:
		n.Children = children
	case *Strong:
		n.Children = children
	case *Span:
		n.Children = children
	case *Mark:
		n.Children = children
	case *Superscript:
		n.Children = children
	case *Subscript:
		n.Children = children
	case *Insert:
		n.Children = children
	case *Delete:
		n.Children = children
	case *DoubleQuoted:
		n.Children = children
	case *SingleQuoted:
		n.Children = children
	case *Verbatim:
		n.Text = w.Text
	case *InlineMath:
		n.Text = w.Text
	case *DisplayMath:
		n.Text = w.Text
	case *RawInline:
		n.Format, n.Text = w.Lang, w.Text
	case *URL:
		n.Text = w.Text
	case *Email:
		n.Text = w.Text
	case *FootnoteReference:
		n.Label = w.Text
	case *Symb:
		n.Alias = w.Text
	case *Link:
		n.Destination, n.Reference, n.Children = w.Destination, w.Reference, children
	case *Image:
		n.Destination, n.Reference, n.Children = w.Destination, w.Reference, children
	}
	return n, nil
}

// unmarshalKind decodes data into *p, failing if it holds another kind.
func unmarshalKind[T any, P interface {
	*T
	Node
}](p P, data []byte) error {
	m, err := UnmarshalNode(data)
	if err != nil {
		return err
	}
	q, ok := m.(P)
	if !ok {
		return fmt.Errorf("gjot: JSON node is %q, not %q", m.Tag(), p.Tag())
	}
	*p = *q
	return nil
}

// Each kind marshals through appendJSON and unmarshals through
// UnmarshalNode, so encoding/json reads and writes the wire form whether
// it holds a kind or the interface.

func (n *Section) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *Section) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *Heading) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *Heading) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *Para) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *Para) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *ThematicBreak) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *ThematicBreak) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *CodeBlock) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *CodeBlock) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *RawBlock) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *RawBlock) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *BlockQuote) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *BlockQuote) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *Div) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *Div) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *BulletList) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *BulletList) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *TaskList) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *TaskList) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *OrderedList) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *OrderedList) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *DefinitionList) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *DefinitionList) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *ListItem) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *ListItem) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *TaskListItem) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *TaskListItem) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *DefinitionListItem) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *DefinitionListItem) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *Term) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *Term) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *Definition) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *Definition) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *Table) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *Table) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *Caption) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *Caption) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *Row) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *Row) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *Cell) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *Cell) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *Footnote) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *Footnote) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *Str) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *Str) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *SoftBreak) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *SoftBreak) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *HardBreak) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *HardBreak) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *NonBreakingSpace) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *NonBreakingSpace) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *Emph) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *Emph) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *Strong) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *Strong) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *Span) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *Span) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *Mark) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *Mark) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *Superscript) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *Superscript) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *Subscript) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *Subscript) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *Insert) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *Insert) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *Delete) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *Delete) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *DoubleQuoted) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *DoubleQuoted) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *SingleQuoted) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *SingleQuoted) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *Verbatim) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *Verbatim) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *InlineMath) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *InlineMath) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *DisplayMath) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *DisplayMath) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *RawInline) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *RawInline) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *URL) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *URL) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *Email) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *Email) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *FootnoteReference) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *FootnoteReference) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *Symb) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *Symb) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *Link) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *Link) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *Image) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *Image) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *Ellipsis) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *Ellipsis) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *EmDash) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *EmDash) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *EnDash) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *EnDash) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }
