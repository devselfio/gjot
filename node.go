// Package gjot parses Djot markup into an AST and renders it as HTML, as
// djot.js AST text, or back to Djot. It is a port of zjot and is proven
// against zjot's vendored djot.js corpus (testdata/spec).
package gjot

import "strings"

// Tag names a node kind: the djot.js tag name ("para", "str", ...), which
// a node's Tag method returns and the AST text renderer and JSON print.
type Tag string

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

// Node is one element of the Djot AST: a pointer to one of the kinds below
// (*Para, *Str, *Link, ...), each of which carries only its own fields
// beside the Header every kind embeds. Only those kinds implement it,
// through the unexported header method. The document root is a *Section of
// Level 0. A renderer or consumer type-switches on the kind:
//
//	switch n := n.(type) {
//	case *gjot.Heading:
//		// n.Level, n.Children
//	case *gjot.Str:
//		// n.Text
//	}
//
// and Walk visits a tree in document order. String fields are "" when
// absent.
type Node interface {
	// Tag is the djot.js name of the node's kind ("para", "str", ...), as
	// ASTText and the JSON form print it.
	Tag() Tag
	HasClass(class string) bool
	Attr(key string) string
	header() *Header
}

// Header is what every kind carries: the id the document gave the node, its
// attributes in source order, and its source range when parsed
// WithSourcePos.
//
// Attrs holds every attribute as djot.js's attribute object holds them, in
// source order: an id written in the source is its "id" entry (an empty
// `id=""` included, which renders and names nothing), the classes are one
// "class" entry (a div's fence class joined to them), and a key keeps its
// first position with its last value. ID is the id the document gave the
// node: the non-empty written id, or a heading's and its section's
// generated one (their AutoID). Renderers write a written id where it
// stands and an unwritten ID first; a footnote reference's HTML writes its
// fnrefN only and drops a written id (zjot.test difference 3).
type Header struct {
	ID    string
	Attrs []Attr
	Pos   *Range
}

func (h *Header) header() *Header { return h }

// writtenID is the id written on the node and whether one was, an empty
// `id=""` included.
func (h *Header) writtenID() (string, bool) {
	for _, a := range h.Attrs {
		if a.Key == "id" {
			return a.Value, true
		}
	}
	return "", false
}

// classList is the classes the node renders with.
func (h *Header) classList() []string {
	return strings.Fields(valueOf(h.Attrs, "class"))
}

// HasClass reports whether class is among the node's rendered classes.
func (h *Header) HasClass(class string) bool {
	for _, c := range h.classList() {
		if c == class {
			return true
		}
	}
	return false
}

// Attr returns the value of the key="value" attribute, or "" if absent.
func (h *Header) Attr(key string) string {
	for _, a := range h.Attrs {
		if a.Key == key {
			return a.Value
		}
	}
	return ""
}

// Blocks.

// Section is a heading and the blocks up to the next heading of its
// level or higher; the document root is a section of Level 0. AutoID is set
// when ID was generated from the heading's text rather than written.
type Section struct {
	Header
	Level    int
	AutoID   bool
	Children []Node
}

// Heading is a heading of Level 1 to 6; AutoID as on a section.
type Heading struct {
	Header
	Level    int
	AutoID   bool
	Children []Node
}

type Para struct {
	Header
	Children []Node
}

type ThematicBreak struct{ Header }

type CodeBlock struct {
	Header
	Lang string
	Text string
}

// RawBlock is a raw block in Format (written `=format`).
type RawBlock struct {
	Header
	Format string
	Text   string
}

type BlockQuote struct {
	Header
	Children []Node
}

type Div struct {
	Header
	Children []Node
}

// BulletList's Style is its bullet ("-", "*", "+").
type BulletList struct {
	Header
	Style    string
	Tight    bool
	Children []Node
}

// TaskList's Style is its bullet.
type TaskList struct {
	Header
	Style    string
	Tight    bool
	Children []Node
}

// OrderedList's Style is djot.js's: the style it settled on in its
// delimiter ("1.", "(a)", "I)"). Marker is its first marker's text ("3",
// "iv"), from which its start is read.
type OrderedList struct {
	Header
	Style    string
	Marker   string
	Tight    bool
	Children []Node
}

type DefinitionList struct {
	Header
	Children []Node
}

type ListItem struct {
	Header
	Children []Node
}

type TaskListItem struct {
	Header
	Checked  bool
	Children []Node
}

type DefinitionListItem struct {
	Header
	Children []Node
}

type Term struct {
	Header
	Children []Node
}

type Definition struct {
	Header
	Children []Node
}

// Table's children are its caption, if any, first, then its rows.
type Table struct {
	Header
	Children []Node
}

type Caption struct {
	Header
	Children []Node
}

type Row struct {
	Header
	Head     bool
	Children []Node
}

type Cell struct {
	Header
	Head     bool
	Align    CellAlign
	Children []Node
}

// Footnote is a footnote definition, standing after the document's
// blocks; the HTML renderer numbers them.
type Footnote struct {
	Header
	Label    string
	Children []Node
}

// Inlines.

// Str is text. Source is set only on a literal smart quote (Text one of
// ‘ ’ “ ” written as ' or "): the quote it was written as. Heading ids,
// reference labels, image alt text and the Djot writer read it, as djot.js
// reads smart punctuation as its source.
type Str struct {
	Header
	Text   string
	Source string
	// off is the offset of Text in the inline source it was sliced from,
	// plus one; 0 means Text is not a slice of it. Parse-time only.
	off int
}

type SoftBreak struct{ Header }

type HardBreak struct{ Header }

type NonBreakingSpace struct{ Header }

type Emph struct {
	Header
	Children []Node
}

type Strong struct {
	Header
	Children []Node
}

type Span struct {
	Header
	Children []Node
}

type Mark struct {
	Header
	Children []Node
}

type Superscript struct {
	Header
	Children []Node
}

type Subscript struct {
	Header
	Children []Node
}

type Insert struct {
	Header
	Children []Node
}

type Delete struct {
	Header
	Children []Node
}

type DoubleQuoted struct {
	Header
	Children []Node
}

type SingleQuoted struct {
	Header
	Children []Node
}

type Verbatim struct {
	Header
	Text string
}

type InlineMath struct {
	Header
	Text string
}

type DisplayMath struct {
	Header
	Text string
}

type RawInline struct {
	Header
	Format string
	Text   string
}

type URL struct {
	Header
	Text string
}

type Email struct {
	Header
	Text string
}

type FootnoteReference struct {
	Header
	Label string
}

type Symb struct {
	Header
	Alias string
}

// Link's Destination is nil when it has none, since an empty
// destination ("[a]()") renders href="". Reference is the label of a
// reference link.
type Link struct {
	Header
	Destination *string
	Reference   string
	Children    []Node
}

// Image's fields are a link's; its children are its alt text.
type Image struct {
	Header
	Destination *string
	Reference   string
	Children    []Node
}

type Ellipsis struct{ Header }

type EmDash struct{ Header }

type EnDash struct{ Header }

func (*Section) Tag() Tag            { return "section" }
func (*Heading) Tag() Tag            { return "heading" }
func (*Para) Tag() Tag               { return "para" }
func (*ThematicBreak) Tag() Tag      { return "thematic_break" }
func (*CodeBlock) Tag() Tag          { return "code_block" }
func (*RawBlock) Tag() Tag           { return "raw_block" }
func (*BlockQuote) Tag() Tag         { return "block_quote" }
func (*Div) Tag() Tag                { return "div" }
func (*BulletList) Tag() Tag         { return "bullet_list" }
func (*TaskList) Tag() Tag           { return "task_list" }
func (*OrderedList) Tag() Tag        { return "ordered_list" }
func (*DefinitionList) Tag() Tag     { return "definition_list" }
func (*ListItem) Tag() Tag           { return "list_item" }
func (*TaskListItem) Tag() Tag       { return "task_list_item" }
func (*DefinitionListItem) Tag() Tag { return "definition_list_item" }
func (*Term) Tag() Tag               { return "term" }
func (*Definition) Tag() Tag         { return "definition" }
func (*Table) Tag() Tag              { return "table" }
func (*Caption) Tag() Tag            { return "caption" }
func (*Row) Tag() Tag                { return "row" }
func (*Cell) Tag() Tag               { return "cell" }
func (*Footnote) Tag() Tag           { return "footnote" }
func (*Str) Tag() Tag                { return "str" }
func (*SoftBreak) Tag() Tag          { return "soft_break" }
func (*HardBreak) Tag() Tag          { return "hard_break" }
func (*NonBreakingSpace) Tag() Tag   { return "non_breaking_space" }
func (*Emph) Tag() Tag               { return "emph" }
func (*Strong) Tag() Tag             { return "strong" }
func (*Span) Tag() Tag               { return "span" }
func (*Mark) Tag() Tag               { return "mark" }
func (*Superscript) Tag() Tag        { return "superscript" }
func (*Subscript) Tag() Tag          { return "subscript" }
func (*Insert) Tag() Tag             { return "insert" }
func (*Delete) Tag() Tag             { return "delete" }
func (*DoubleQuoted) Tag() Tag       { return "double_quoted" }
func (*SingleQuoted) Tag() Tag       { return "single_quoted" }
func (*Verbatim) Tag() Tag           { return "verbatim" }
func (*InlineMath) Tag() Tag         { return "inline_math" }
func (*DisplayMath) Tag() Tag        { return "display_math" }
func (*RawInline) Tag() Tag          { return "raw_inline" }
func (*URL) Tag() Tag                { return "url" }
func (*Email) Tag() Tag              { return "email" }
func (*FootnoteReference) Tag() Tag  { return "footnote_reference" }
func (*Symb) Tag() Tag               { return "symb" }
func (*Link) Tag() Tag               { return "link" }
func (*Image) Tag() Tag              { return "image" }
func (*Ellipsis) Tag() Tag           { return "ellipsis" }
func (*EmDash) Tag() Tag             { return "em_dash" }
func (*EnDash) Tag() Tag             { return "en_dash" }

// nodeKinds is the inventory: every kind Parse produces, by its tag, with a
// constructor of its zero value. The JSON decoder reads it.
var nodeKinds = func() map[Tag]func() Node {
	kinds := map[Tag]func() Node{}
	for _, f := range []func() Node{
		func() Node { return &Section{} },
		func() Node { return &Heading{} },
		func() Node { return &Para{} },
		func() Node { return &ThematicBreak{} },
		func() Node { return &CodeBlock{} },
		func() Node { return &RawBlock{} },
		func() Node { return &BlockQuote{} },
		func() Node { return &Div{} },
		func() Node { return &BulletList{} },
		func() Node { return &TaskList{} },
		func() Node { return &OrderedList{} },
		func() Node { return &DefinitionList{} },
		func() Node { return &ListItem{} },
		func() Node { return &TaskListItem{} },
		func() Node { return &DefinitionListItem{} },
		func() Node { return &Term{} },
		func() Node { return &Definition{} },
		func() Node { return &Table{} },
		func() Node { return &Caption{} },
		func() Node { return &Row{} },
		func() Node { return &Cell{} },
		func() Node { return &Footnote{} },
		func() Node { return &Str{} },
		func() Node { return &SoftBreak{} },
		func() Node { return &HardBreak{} },
		func() Node { return &NonBreakingSpace{} },
		func() Node { return &Emph{} },
		func() Node { return &Strong{} },
		func() Node { return &Span{} },
		func() Node { return &Mark{} },
		func() Node { return &Superscript{} },
		func() Node { return &Subscript{} },
		func() Node { return &Insert{} },
		func() Node { return &Delete{} },
		func() Node { return &DoubleQuoted{} },
		func() Node { return &SingleQuoted{} },
		func() Node { return &Verbatim{} },
		func() Node { return &InlineMath{} },
		func() Node { return &DisplayMath{} },
		func() Node { return &RawInline{} },
		func() Node { return &URL{} },
		func() Node { return &Email{} },
		func() Node { return &FootnoteReference{} },
		func() Node { return &Symb{} },
		func() Node { return &Link{} },
		func() Node { return &Image{} },
		func() Node { return &Ellipsis{} },
		func() Node { return &EmDash{} },
		func() Node { return &EnDash{} },
	} {
		kinds[f().Tag()] = f
	}
	return kinds
}()

// linkFields is what a link and an image share: a pointer to the
// destination, to the reference and the children. ok is false for any other
// kind.
func linkFields(n Node) (dest **string, ref *string, children []Node, ok bool) {
	switch n := n.(type) {
	case *Link:
		return &n.Destination, &n.Reference, n.Children, true
	case *Image:
		return &n.Destination, &n.Reference, n.Children, true
	}
	return nil, nil, nil, false
}
