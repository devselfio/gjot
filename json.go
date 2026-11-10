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
func wireFields(w *jsonNode, n treeNode) {
	h := n.header()
	w.Tag, w.ID, w.Attrs, w.Pos = n.Tag(), h.ID, h.Attrs, h.Pos
	switch n := n.(type) {
	case *treeSection:
		w.Level, w.AutoID = n.Level, n.AutoID
	case *treeHeading:
		w.Level, w.AutoID = n.Level, n.AutoID
	case *treeCodeBlock:
		w.Lang, w.Text = n.Lang, n.Text
	case *treeRawBlock:
		w.Lang, w.Text = n.Format, n.Text
	case *treeBulletList:
		w.Style, w.Tight = n.Style, n.Tight
	case *treeTaskList:
		w.Style, w.Tight = n.Style, n.Tight
	case *treeOrderedList:
		w.Style, w.Text, w.Tight = n.Style, n.Marker, n.Tight
	case *treeTaskListItem:
		w.Checked = &n.Checked
	case *treeRow:
		w.Head = n.Head
	case *treeCell:
		w.Head, w.Align = n.Head, n.Align
	case *treeFootnote:
		w.Text = n.Label
	case *treeStr:
		w.Text, w.Source = n.Text, n.Source
	case *treeVerbatim:
		w.Text = n.Text
	case *treeInlineMath:
		w.Text = n.Text
	case *treeDisplayMath:
		w.Text = n.Text
	case *treeRawInline:
		w.Lang, w.Text = n.Format, n.Text
	case *treeURL:
		w.Text = n.Text
	case *treeEmail:
		w.Text = n.Text
	case *treeFootnoteReference:
		w.Text = n.Label
	case *treeSymb:
		w.Text = n.Alias
	case *treeLink:
		w.Destination, w.Reference = n.Destination, n.Reference
	case *treeImage:
		w.Destination, w.Reference = n.Destination, n.Reference
	}
}

// appendJSON appends n's JSON form: jsonNode's fields in its order with its
// omitempty rules, children recursed into here rather than through nested
// MarshalJSON calls, which encoding/json would re-validate at every level.
func appendJSON(dst []byte, n treeNode) []byte {
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

// treeUnmarshalNode decodes one node of any kind from its JSON form,
// switching on "tag"; an unknown or missing tag is an error. A document
// marshalled by an earlier gjot decodes: a key no kind has (its "classes")
// is ignored, as is a field the node's kind does not carry ("auto_id" is
// read on headings and sections only).
func treeUnmarshalNode(data []byte) (treeNode, error) {
	var w jsonNode
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, err
	}
	return fromWire(&w)
}

var errNoTag = errors.New(`gjot: JSON node has no "tag"`)

// fromWire builds the kind w's tag names and fills its fields from w,
// recursing through w's children.
func fromWire(w *jsonNode) (treeNode, error) {
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
	var children []treeNode
	if len(w.Children) > 0 {
		children = make([]treeNode, len(w.Children))
		for i := range w.Children {
			c, err := fromWire(&w.Children[i])
			if err != nil {
				return nil, err
			}
			children[i] = c
		}
	}
	switch n := n.(type) {
	case *treeSection:
		n.Level, n.AutoID, n.Children = w.Level, w.AutoID, children
	case *treeHeading:
		n.Level, n.AutoID, n.Children = w.Level, w.AutoID, children
	case *treePara:
		n.Children = children
	case *treeCodeBlock:
		n.Lang, n.Text = w.Lang, w.Text
	case *treeRawBlock:
		n.Format, n.Text = w.Lang, w.Text
	case *treeBlockQuote:
		n.Children = children
	case *treeDiv:
		n.Children = children
	case *treeBulletList:
		n.Style, n.Tight, n.Children = w.Style, w.Tight, children
	case *treeTaskList:
		n.Style, n.Tight, n.Children = w.Style, w.Tight, children
	case *treeOrderedList:
		n.Style, n.Marker, n.Tight, n.Children = w.Style, w.Text, w.Tight, children
	case *treeDefinitionList:
		n.Children = children
	case *treeListItem:
		n.Children = children
	case *treeTaskListItem:
		n.Checked, n.Children = w.Checked != nil && *w.Checked, children
	case *treeDefinitionListItem:
		n.Children = children
	case *treeTerm:
		n.Children = children
	case *treeDefinition:
		n.Children = children
	case *treeTable:
		n.Children = children
	case *treeCaption:
		n.Children = children
	case *treeRow:
		n.Head, n.Children = w.Head, children
	case *treeCell:
		n.Head, n.Align, n.Children = w.Head, w.Align, children
	case *treeFootnote:
		n.Label, n.Children = w.Text, children
	case *treeStr:
		n.Text, n.Source = w.Text, w.Source
	case *treeEmph:
		n.Children = children
	case *treeStrong:
		n.Children = children
	case *treeSpan:
		n.Children = children
	case *treeMark:
		n.Children = children
	case *treeSuperscript:
		n.Children = children
	case *treeSubscript:
		n.Children = children
	case *treeInsert:
		n.Children = children
	case *treeDelete:
		n.Children = children
	case *treeDoubleQuoted:
		n.Children = children
	case *treeSingleQuoted:
		n.Children = children
	case *treeVerbatim:
		n.Text = w.Text
	case *treeInlineMath:
		n.Text = w.Text
	case *treeDisplayMath:
		n.Text = w.Text
	case *treeRawInline:
		n.Format, n.Text = w.Lang, w.Text
	case *treeURL:
		n.Text = w.Text
	case *treeEmail:
		n.Text = w.Text
	case *treeFootnoteReference:
		n.Label = w.Text
	case *treeSymb:
		n.Alias = w.Text
	case *treeLink:
		n.Destination, n.Reference, n.Children = w.Destination, w.Reference, children
	case *treeImage:
		n.Destination, n.Reference, n.Children = w.Destination, w.Reference, children
	}
	return n, nil
}

// unmarshalKind decodes data into *p, failing if it holds another kind.
func unmarshalKind[T any, P interface {
	*T
	treeNode
}](p P, data []byte) error {
	m, err := treeUnmarshalNode(data)
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
// treeUnmarshalNode, so encoding/json reads and writes the wire form whether
// it holds a kind or the interface.

func (n *treeSection) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeSection) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeHeading) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeHeading) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treePara) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treePara) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeThematicBreak) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeThematicBreak) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeCodeBlock) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeCodeBlock) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeRawBlock) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeRawBlock) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeBlockQuote) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeBlockQuote) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeDiv) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeDiv) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeBulletList) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeBulletList) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeTaskList) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeTaskList) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeOrderedList) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeOrderedList) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeDefinitionList) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeDefinitionList) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeListItem) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeListItem) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeTaskListItem) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeTaskListItem) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeDefinitionListItem) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeDefinitionListItem) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeTerm) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeTerm) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeDefinition) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeDefinition) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeTable) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeTable) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeCaption) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeCaption) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeRow) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeRow) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeCell) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeCell) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeFootnote) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeFootnote) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeStr) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeStr) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeSoftBreak) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeSoftBreak) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeHardBreak) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeHardBreak) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeNonBreakingSpace) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeNonBreakingSpace) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeEmph) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeEmph) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeStrong) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeStrong) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeSpan) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeSpan) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeMark) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeMark) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeSuperscript) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeSuperscript) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeSubscript) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeSubscript) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeInsert) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeInsert) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeDelete) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeDelete) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeDoubleQuoted) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeDoubleQuoted) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeSingleQuoted) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeSingleQuoted) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeVerbatim) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeVerbatim) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeInlineMath) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeInlineMath) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeDisplayMath) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeDisplayMath) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeRawInline) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeRawInline) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeURL) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeURL) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeEmail) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeEmail) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeFootnoteReference) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeFootnoteReference) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeSymb) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeSymb) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeLink) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeLink) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeImage) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeImage) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeEllipsis) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeEllipsis) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeEmDash) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeEmDash) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }

func (n *treeEnDash) MarshalJSON() ([]byte, error)    { return appendJSON(nil, n), nil }
func (n *treeEnDash) UnmarshalJSON(data []byte) error { return unmarshalKind(n, data) }
