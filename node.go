// Package gjot parses Djot markup into an AST and renders it as HTML, as
// djot.js AST text, or back to Djot. It is a port of zjot and is proven
// against zjot's vendored djot.js corpus (testdata/spec).
package gjot

import "strings"

// Tag names a node kind. Values are the djot.js tag names, so the AST text
// renderer and JSON encode them unchanged.
type Tag string

// Block tags.
const (
	Para               Tag = "para"
	Heading            Tag = "heading"
	Section            Tag = "section"
	ThematicBreak      Tag = "thematic_break"
	CodeBlock          Tag = "code_block"
	BlockQuote         Tag = "block_quote"
	Div                Tag = "div"
	BulletList         Tag = "bullet_list"
	OrderedList        Tag = "ordered_list"
	TaskList           Tag = "task_list"
	DefinitionList     Tag = "definition_list"
	ListItem           Tag = "list_item"
	TaskListItem       Tag = "task_list_item"
	DefinitionListItem Tag = "definition_list_item"
	Term               Tag = "term"
	Definition         Tag = "definition"
	Table              Tag = "table"
	Caption            Tag = "caption"
	Row                Tag = "row"
	Cell               Tag = "cell"
	RawBlock           Tag = "raw_block"
	Footnote           Tag = "footnote"
)

// Inline tags.
const (
	Str               Tag = "str"
	SoftBreak         Tag = "soft_break"
	HardBreak         Tag = "hard_break"
	NonBreakingSpace  Tag = "non_breaking_space"
	Emph              Tag = "emph"
	Strong            Tag = "strong"
	Verbatim          Tag = "verbatim"
	Link              Tag = "link"
	Image             Tag = "image"
	Span              Tag = "span"
	Mark              Tag = "mark"
	Superscript       Tag = "superscript"
	Subscript         Tag = "subscript"
	Insert            Tag = "insert"
	Delete            Tag = "delete"
	DoubleQuoted      Tag = "double_quoted"
	SingleQuoted      Tag = "single_quoted"
	InlineMath        Tag = "inline_math"
	DisplayMath       Tag = "display_math"
	RawInline         Tag = "raw_inline"
	URL               Tag = "url"
	Email             Tag = "email"
	FootnoteReference Tag = "footnote_reference"
	Symb              Tag = "symb"
	Ellipsis          Tag = "ellipsis"
	EmDash            Tag = "em_dash"
	EnDash            Tag = "en_dash"
)

// CellAlign is a table cell's horizontal alignment.
type CellAlign string

const (
	AlignDefault CellAlign = "default"
	AlignLeft    CellAlign = "left"
	AlignRight   CellAlign = "right"
	AlignCenter  CellAlign = "center"
)

// Attr is one key="value" attribute.
type Attr struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// SourcePos is a 1-based line and column with a 0-based byte offset.
type SourcePos struct {
	Line   int `json:"line"`
	Col    int `json:"col"`
	Offset int `json:"offset"`
}

// Range is the source span a node was parsed from.
type Range struct {
	Start SourcePos `json:"start"`
	End   SourcePos `json:"end"`
}

// Node is one element of the Djot AST. The document root is a Section with
// Level 0. String fields are "" when absent; Destination is nil when a link
// has no destination, since an empty destination ("[a]()") renders href="".
// A list's Style is djot.js's: the bullet of a bullet or task list, and for
// an ordered list the style it settled on in its delimiter ("1.", "(a)",
// "I)"); an ordered list's Text is its first marker's text ("3", "iv"),
// from which its start is read.
//
// Attrs holds every attribute as djot.js's attribute object holds them, in
// source order: an id written in the source is its "id" entry (an empty
// `id=""` included, which renders and names nothing), the classes are one
// "class" entry (a div's fence class joined to them), and a key keeps its
// first position with its last value. ID is the id the document gave the
// node: the non-empty written id, or a heading's generated one (AutoID).
// Renderers write a written id where it stands and an unwritten ID first; a
// footnote reference's HTML writes its fnrefN only and drops a written id
// (zjot.test difference 3). A footnote reference's Text is its label, as is
// a Footnote's (a definition, standing after the document's blocks); the HTML
// renderer numbers them.
type Node struct {
	Tag         Tag       `json:"tag"`
	Children    []*Node   `json:"children,omitempty"`
	Text        string    `json:"text,omitempty"`
	Level       int       `json:"level,omitempty"`
	ID          string    `json:"id,omitempty"`
	Lang        string    `json:"lang,omitempty"`
	Destination *string   `json:"destination,omitempty"`
	Reference   string    `json:"reference,omitempty"`
	Attrs       []Attr    `json:"attrs,omitempty"`
	Tight       bool      `json:"tight,omitempty"`
	Checked     *bool     `json:"checked,omitempty"`
	Head        bool      `json:"head,omitempty"`
	Align       CellAlign `json:"align,omitempty"`
	Style       string    `json:"style,omitempty"`
	// AutoID is set when ID was generated from heading text rather than
	// written in the source, so a writer can omit it.
	AutoID bool `json:"auto_id,omitempty"`
	// Source is set only on a literal smart quote (a Str whose Text is ‘ ’
	// “ or ” and was written as ' or "): the quote it was written as. Heading
	// ids, reference labels, image alt text and the Djot writer read it, as
	// djot.js reads smart punctuation as its source; it is "" on every other
	// node, and absent from the JSON form there.
	Source string `json:"source,omitempty"`
	Pos    *Range `json:"pos,omitempty"`

	// off is the offset of Text in the inline source it was sliced from,
	// plus one; 0 means Text is not a slice of it. Parse-time only.
	off int
}

// writtenID is the id written on the node and whether one was, an empty
// `id=""` included.
func (n *Node) writtenID() (string, bool) {
	for _, a := range n.Attrs {
		if a.Key == "id" {
			return a.Value, true
		}
	}
	return "", false
}

// classList is the classes the node renders with.
func (n *Node) classList() []string {
	return strings.Fields(valueOf(n.Attrs, "class"))
}

// HasClass reports whether class is among the node's rendered classes.
func (n *Node) HasClass(class string) bool {
	for _, c := range n.classList() {
		if c == class {
			return true
		}
	}
	return false
}

// Attr returns the value of the key="value" attribute, or "" if absent.
func (n *Node) Attr(key string) string {
	for _, a := range n.Attrs {
		if a.Key == key {
			return a.Value
		}
	}
	return ""
}
