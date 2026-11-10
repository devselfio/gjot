package gjot

import "strings"

// This file is the go/ast-shaped tree: one struct per node kind behind an
// interface (swarm-gridapp#8). It is built beside the old Node struct until
// the parser builds it directly; until then every name carries a "tree"
// prefix, which the swap renames away (treeNode becomes Node, treePara
// becomes Para, and so on).

// treeNode is one element of the Djot AST. Only the pointer kinds below
// implement it, through the unexported header method.
type treeNode interface {
	// Tag is the djot.js name of the node's kind ("para", "str", ...), as
	// ASTText and the JSON form print it.
	Tag() Tag
	HasClass(class string) bool
	Attr(key string) string
	header() *treeHeader
}

// treeHeader is what every kind carries: the id the document gave the node
// (written or, on a heading and its section, generated), its attributes in
// source order, and its source range when parsed WithSourcePos.
type treeHeader struct {
	ID    string
	Attrs []Attr
	Pos   *Range
}

func (h *treeHeader) header() *treeHeader { return h }

// writtenID is the id written on the node and whether one was, an empty
// `id=""` included.
func (h *treeHeader) writtenID() (string, bool) {
	for _, a := range h.Attrs {
		if a.Key == "id" {
			return a.Value, true
		}
	}
	return "", false
}

// classList is the classes the node renders with.
func (h *treeHeader) classList() []string {
	return strings.Fields(valueOf(h.Attrs, "class"))
}

// HasClass reports whether class is among the node's rendered classes.
func (h *treeHeader) HasClass(class string) bool {
	for _, c := range h.classList() {
		if c == class {
			return true
		}
	}
	return false
}

// Attr returns the value of the key="value" attribute, or "" if absent.
func (h *treeHeader) Attr(key string) string {
	for _, a := range h.Attrs {
		if a.Key == key {
			return a.Value
		}
	}
	return ""
}

// Blocks.

// treeSection is a heading and the blocks up to the next heading of its
// level or higher; the document root is a section of Level 0. AutoID is set
// when ID was generated from the heading's text rather than written.
type treeSection struct {
	treeHeader
	Level    int
	AutoID   bool
	Children []treeNode
}

// treeHeading is a heading of Level 1 to 6; AutoID as on a section.
type treeHeading struct {
	treeHeader
	Level    int
	AutoID   bool
	Children []treeNode
}

type treePara struct {
	treeHeader
	Children []treeNode
}

type treeThematicBreak struct{ treeHeader }

type treeCodeBlock struct {
	treeHeader
	Lang string
	Text string
}

// treeRawBlock is a raw block in Format (written `=format`).
type treeRawBlock struct {
	treeHeader
	Format string
	Text   string
}

type treeBlockQuote struct {
	treeHeader
	Children []treeNode
}

type treeDiv struct {
	treeHeader
	Children []treeNode
}

// treeBulletList's Style is its bullet ("-", "*", "+").
type treeBulletList struct {
	treeHeader
	Style    string
	Tight    bool
	Children []treeNode
}

// treeTaskList's Style is its bullet.
type treeTaskList struct {
	treeHeader
	Style    string
	Tight    bool
	Children []treeNode
}

// treeOrderedList's Style is djot.js's: the style it settled on in its
// delimiter ("1.", "(a)", "I)"). Marker is its first marker's text ("3",
// "iv"), from which its start is read.
type treeOrderedList struct {
	treeHeader
	Style    string
	Marker   string
	Tight    bool
	Children []treeNode
}

type treeDefinitionList struct {
	treeHeader
	Children []treeNode
}

type treeListItem struct {
	treeHeader
	Children []treeNode
}

type treeTaskListItem struct {
	treeHeader
	Checked  bool
	Children []treeNode
}

type treeDefinitionListItem struct {
	treeHeader
	Children []treeNode
}

type treeTerm struct {
	treeHeader
	Children []treeNode
}

type treeDefinition struct {
	treeHeader
	Children []treeNode
}

// treeTable's children are its caption, if any, first, then its rows.
type treeTable struct {
	treeHeader
	Children []treeNode
}

type treeCaption struct {
	treeHeader
	Children []treeNode
}

type treeRow struct {
	treeHeader
	Head     bool
	Children []treeNode
}

type treeCell struct {
	treeHeader
	Head     bool
	Align    CellAlign
	Children []treeNode
}

// treeFootnote is a footnote definition, standing after the document's
// blocks; the HTML renderer numbers them.
type treeFootnote struct {
	treeHeader
	Label    string
	Children []treeNode
}

// Inlines.

// treeStr is text. Source is set only on a literal smart quote (Text one of
// ‘ ’ “ ” written as ' or "): the quote it was written as.
type treeStr struct {
	treeHeader
	Text   string
	Source string
	// off is the offset of Text in the inline source it was sliced from,
	// plus one; 0 means Text is not a slice of it. Parse-time only.
	off int
}

type treeSoftBreak struct{ treeHeader }

type treeHardBreak struct{ treeHeader }

type treeNonBreakingSpace struct{ treeHeader }

type treeEmph struct {
	treeHeader
	Children []treeNode
}

type treeStrong struct {
	treeHeader
	Children []treeNode
}

type treeSpan struct {
	treeHeader
	Children []treeNode
}

type treeMark struct {
	treeHeader
	Children []treeNode
}

type treeSuperscript struct {
	treeHeader
	Children []treeNode
}

type treeSubscript struct {
	treeHeader
	Children []treeNode
}

type treeInsert struct {
	treeHeader
	Children []treeNode
}

type treeDelete struct {
	treeHeader
	Children []treeNode
}

type treeDoubleQuoted struct {
	treeHeader
	Children []treeNode
}

type treeSingleQuoted struct {
	treeHeader
	Children []treeNode
}

type treeVerbatim struct {
	treeHeader
	Text string
}

type treeInlineMath struct {
	treeHeader
	Text string
}

type treeDisplayMath struct {
	treeHeader
	Text string
}

type treeRawInline struct {
	treeHeader
	Format string
	Text   string
}

type treeURL struct {
	treeHeader
	Text string
}

type treeEmail struct {
	treeHeader
	Text string
}

type treeFootnoteReference struct {
	treeHeader
	Label string
}

type treeSymb struct {
	treeHeader
	Alias string
}

// treeLink's Destination is nil when it has none, since an empty
// destination ("[a]()") renders href="". Reference is the label of a
// reference link.
type treeLink struct {
	treeHeader
	Destination *string
	Reference   string
	Children    []treeNode
}

// treeImage's fields are a link's; its children are its alt text.
type treeImage struct {
	treeHeader
	Destination *string
	Reference   string
	Children    []treeNode
}

type treeEllipsis struct{ treeHeader }

type treeEmDash struct{ treeHeader }

type treeEnDash struct{ treeHeader }

func (*treeSection) Tag() Tag            { return "section" }
func (*treeHeading) Tag() Tag            { return "heading" }
func (*treePara) Tag() Tag               { return "para" }
func (*treeThematicBreak) Tag() Tag      { return "thematic_break" }
func (*treeCodeBlock) Tag() Tag          { return "code_block" }
func (*treeRawBlock) Tag() Tag           { return "raw_block" }
func (*treeBlockQuote) Tag() Tag         { return "block_quote" }
func (*treeDiv) Tag() Tag                { return "div" }
func (*treeBulletList) Tag() Tag         { return "bullet_list" }
func (*treeTaskList) Tag() Tag           { return "task_list" }
func (*treeOrderedList) Tag() Tag        { return "ordered_list" }
func (*treeDefinitionList) Tag() Tag     { return "definition_list" }
func (*treeListItem) Tag() Tag           { return "list_item" }
func (*treeTaskListItem) Tag() Tag       { return "task_list_item" }
func (*treeDefinitionListItem) Tag() Tag { return "definition_list_item" }
func (*treeTerm) Tag() Tag               { return "term" }
func (*treeDefinition) Tag() Tag         { return "definition" }
func (*treeTable) Tag() Tag              { return "table" }
func (*treeCaption) Tag() Tag            { return "caption" }
func (*treeRow) Tag() Tag                { return "row" }
func (*treeCell) Tag() Tag               { return "cell" }
func (*treeFootnote) Tag() Tag           { return "footnote" }
func (*treeStr) Tag() Tag                { return "str" }
func (*treeSoftBreak) Tag() Tag          { return "soft_break" }
func (*treeHardBreak) Tag() Tag          { return "hard_break" }
func (*treeNonBreakingSpace) Tag() Tag   { return "non_breaking_space" }
func (*treeEmph) Tag() Tag               { return "emph" }
func (*treeStrong) Tag() Tag             { return "strong" }
func (*treeSpan) Tag() Tag               { return "span" }
func (*treeMark) Tag() Tag               { return "mark" }
func (*treeSuperscript) Tag() Tag        { return "superscript" }
func (*treeSubscript) Tag() Tag          { return "subscript" }
func (*treeInsert) Tag() Tag             { return "insert" }
func (*treeDelete) Tag() Tag             { return "delete" }
func (*treeDoubleQuoted) Tag() Tag       { return "double_quoted" }
func (*treeSingleQuoted) Tag() Tag       { return "single_quoted" }
func (*treeVerbatim) Tag() Tag           { return "verbatim" }
func (*treeInlineMath) Tag() Tag         { return "inline_math" }
func (*treeDisplayMath) Tag() Tag        { return "display_math" }
func (*treeRawInline) Tag() Tag          { return "raw_inline" }
func (*treeURL) Tag() Tag                { return "url" }
func (*treeEmail) Tag() Tag              { return "email" }
func (*treeFootnoteReference) Tag() Tag  { return "footnote_reference" }
func (*treeSymb) Tag() Tag               { return "symb" }
func (*treeLink) Tag() Tag               { return "link" }
func (*treeImage) Tag() Tag              { return "image" }
func (*treeEllipsis) Tag() Tag           { return "ellipsis" }
func (*treeEmDash) Tag() Tag             { return "em_dash" }
func (*treeEnDash) Tag() Tag             { return "en_dash" }

// nodeKinds is the inventory: every kind Parse produces, by its tag, with a
// constructor of its zero value. The JSON decoder reads it.
var nodeKinds = func() map[Tag]func() treeNode {
	kinds := map[Tag]func() treeNode{}
	for _, f := range []func() treeNode{
		func() treeNode { return &treeSection{} },
		func() treeNode { return &treeHeading{} },
		func() treeNode { return &treePara{} },
		func() treeNode { return &treeThematicBreak{} },
		func() treeNode { return &treeCodeBlock{} },
		func() treeNode { return &treeRawBlock{} },
		func() treeNode { return &treeBlockQuote{} },
		func() treeNode { return &treeDiv{} },
		func() treeNode { return &treeBulletList{} },
		func() treeNode { return &treeTaskList{} },
		func() treeNode { return &treeOrderedList{} },
		func() treeNode { return &treeDefinitionList{} },
		func() treeNode { return &treeListItem{} },
		func() treeNode { return &treeTaskListItem{} },
		func() treeNode { return &treeDefinitionListItem{} },
		func() treeNode { return &treeTerm{} },
		func() treeNode { return &treeDefinition{} },
		func() treeNode { return &treeTable{} },
		func() treeNode { return &treeCaption{} },
		func() treeNode { return &treeRow{} },
		func() treeNode { return &treeCell{} },
		func() treeNode { return &treeFootnote{} },
		func() treeNode { return &treeStr{} },
		func() treeNode { return &treeSoftBreak{} },
		func() treeNode { return &treeHardBreak{} },
		func() treeNode { return &treeNonBreakingSpace{} },
		func() treeNode { return &treeEmph{} },
		func() treeNode { return &treeStrong{} },
		func() treeNode { return &treeSpan{} },
		func() treeNode { return &treeMark{} },
		func() treeNode { return &treeSuperscript{} },
		func() treeNode { return &treeSubscript{} },
		func() treeNode { return &treeInsert{} },
		func() treeNode { return &treeDelete{} },
		func() treeNode { return &treeDoubleQuoted{} },
		func() treeNode { return &treeSingleQuoted{} },
		func() treeNode { return &treeVerbatim{} },
		func() treeNode { return &treeInlineMath{} },
		func() treeNode { return &treeDisplayMath{} },
		func() treeNode { return &treeRawInline{} },
		func() treeNode { return &treeURL{} },
		func() treeNode { return &treeEmail{} },
		func() treeNode { return &treeFootnoteReference{} },
		func() treeNode { return &treeSymb{} },
		func() treeNode { return &treeLink{} },
		func() treeNode { return &treeImage{} },
		func() treeNode { return &treeEllipsis{} },
		func() treeNode { return &treeEmDash{} },
		func() treeNode { return &treeEnDash{} },
	} {
		kinds[f().Tag()] = f
	}
	return kinds
}()

// linkFields is what a link and an image share: a pointer to the
// destination, to the reference and the children. ok is false for any other
// kind.
func linkFields(n treeNode) (dest **string, ref *string, children []treeNode, ok bool) {
	switch n := n.(type) {
	case *treeLink:
		return &n.Destination, &n.Reference, n.Children, true
	case *treeImage:
		return &n.Destination, &n.Reference, n.Children, true
	}
	return nil, nil, nil, false
}
