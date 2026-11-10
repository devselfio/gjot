package gjot

// treeWalk visits n and then its descendants in document order: a node
// before its children, children in order, the footnote definitions after the
// body as the tree holds them. When visit returns WalkSkipChildren for a
// node, its children are not visited.
func treeWalk(n treeNode, visit func(n treeNode) WalkStatus) {
	if visit(n) == WalkSkipChildren {
		return
	}
	for _, c := range childrenOf(n) {
		treeWalk(c, visit)
	}
}

// childrenOf is n's children, or nil for a kind that has none.
func childrenOf(n treeNode) []treeNode {
	switch n := n.(type) {
	case *treeSection:
		return n.Children
	case *treeHeading:
		return n.Children
	case *treePara:
		return n.Children
	case *treeBlockQuote:
		return n.Children
	case *treeDiv:
		return n.Children
	case *treeBulletList:
		return n.Children
	case *treeTaskList:
		return n.Children
	case *treeOrderedList:
		return n.Children
	case *treeDefinitionList:
		return n.Children
	case *treeListItem:
		return n.Children
	case *treeTaskListItem:
		return n.Children
	case *treeDefinitionListItem:
		return n.Children
	case *treeTerm:
		return n.Children
	case *treeDefinition:
		return n.Children
	case *treeTable:
		return n.Children
	case *treeCaption:
		return n.Children
	case *treeRow:
		return n.Children
	case *treeCell:
		return n.Children
	case *treeFootnote:
		return n.Children
	case *treeEmph:
		return n.Children
	case *treeStrong:
		return n.Children
	case *treeSpan:
		return n.Children
	case *treeMark:
		return n.Children
	case *treeSuperscript:
		return n.Children
	case *treeSubscript:
		return n.Children
	case *treeInsert:
		return n.Children
	case *treeDelete:
		return n.Children
	case *treeDoubleQuoted:
		return n.Children
	case *treeSingleQuoted:
		return n.Children
	case *treeLink:
		return n.Children
	case *treeImage:
		return n.Children
	}
	return nil
}
