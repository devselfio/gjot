package ast

// WalkStatus is what a Walk visitor, or a gjot.Hook's entering call,
// returns to control the visit of the node's children.
type WalkStatus int

const (
	// WalkContinue visits (or renders) the children.
	WalkContinue WalkStatus = iota
	// WalkSkipChildren leaves the children unvisited (or unrendered).
	WalkSkipChildren
)

// Walk visits n and then its descendants in document order: a node
// before its children, children in order, the footnote definitions after the
// body as the tree holds them. When visit returns WalkSkipChildren for a
// node, its children are not visited.
func Walk(n Node, visit func(n Node) WalkStatus) {
	if visit(n) == WalkSkipChildren {
		return
	}
	for _, c := range Children(n) {
		Walk(c, visit)
	}
}

// Children returns n's children in order: the slice the node holds, not
// a copy, or nil for a kind that has none (a leaf such as *Str).
func Children(n Node) []Node {
	switch n := n.(type) {
	case *Section:
		return n.Children
	case *Heading:
		return n.Children
	case *Para:
		return n.Children
	case *BlockQuote:
		return n.Children
	case *Div:
		return n.Children
	case *BulletList:
		return n.Children
	case *TaskList:
		return n.Children
	case *OrderedList:
		return n.Children
	case *DefinitionList:
		return n.Children
	case *ListItem:
		return n.Children
	case *TaskListItem:
		return n.Children
	case *DefinitionListItem:
		return n.Children
	case *Term:
		return n.Children
	case *Definition:
		return n.Children
	case *Table:
		return n.Children
	case *Caption:
		return n.Children
	case *Row:
		return n.Children
	case *Cell:
		return n.Children
	case *Footnote:
		return n.Children
	case *Emph:
		return n.Children
	case *Strong:
		return n.Children
	case *Span:
		return n.Children
	case *Mark:
		return n.Children
	case *Superscript:
		return n.Children
	case *Subscript:
		return n.Children
	case *Insert:
		return n.Children
	case *Delete:
		return n.Children
	case *DoubleQuoted:
		return n.Children
	case *SingleQuoted:
		return n.Children
	case *Link:
		return n.Children
	case *Image:
		return n.Children
	}
	return nil
}
