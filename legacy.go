package gjot

// fromLegacy converts the old tree to the new: one node per node, the id,
// attributes and position into the header, each kind keeping only its own
// fields. A task list item's nil Checked reads as false; AutoID is kept on
// headings and sections only. This file goes when the parser builds the new
// tree directly.
func fromLegacy(n *Node) treeNode {
	var children []treeNode
	if len(n.Children) > 0 {
		children = make([]treeNode, len(n.Children))
		for i, c := range n.Children {
			children[i] = fromLegacy(c)
		}
	}
	h := treeHeader{ID: n.ID, Attrs: n.Attrs, Pos: n.Pos}
	switch n.Tag {
	case Section:
		return &treeSection{treeHeader: h, Level: n.Level, AutoID: n.AutoID, Children: children}
	case Heading:
		return &treeHeading{treeHeader: h, Level: n.Level, AutoID: n.AutoID, Children: children}
	case Para:
		return &treePara{treeHeader: h, Children: children}
	case ThematicBreak:
		return &treeThematicBreak{treeHeader: h}
	case CodeBlock:
		return &treeCodeBlock{treeHeader: h, Lang: n.Lang, Text: n.Text}
	case RawBlock:
		return &treeRawBlock{treeHeader: h, Format: n.Lang, Text: n.Text}
	case BlockQuote:
		return &treeBlockQuote{treeHeader: h, Children: children}
	case Div:
		return &treeDiv{treeHeader: h, Children: children}
	case BulletList:
		return &treeBulletList{treeHeader: h, Style: n.Style, Tight: n.Tight, Children: children}
	case TaskList:
		return &treeTaskList{treeHeader: h, Style: n.Style, Tight: n.Tight, Children: children}
	case OrderedList:
		return &treeOrderedList{treeHeader: h, Style: n.Style, Marker: n.Text, Tight: n.Tight, Children: children}
	case DefinitionList:
		return &treeDefinitionList{treeHeader: h, Children: children}
	case ListItem:
		return &treeListItem{treeHeader: h, Children: children}
	case TaskListItem:
		return &treeTaskListItem{treeHeader: h, Checked: n.Checked != nil && *n.Checked, Children: children}
	case DefinitionListItem:
		return &treeDefinitionListItem{treeHeader: h, Children: children}
	case Term:
		return &treeTerm{treeHeader: h, Children: children}
	case Definition:
		return &treeDefinition{treeHeader: h, Children: children}
	case Table:
		return &treeTable{treeHeader: h, Children: children}
	case Caption:
		return &treeCaption{treeHeader: h, Children: children}
	case Row:
		return &treeRow{treeHeader: h, Head: n.Head, Children: children}
	case Cell:
		return &treeCell{treeHeader: h, Head: n.Head, Align: n.Align, Children: children}
	case Footnote:
		return &treeFootnote{treeHeader: h, Label: n.Text, Children: children}
	case Str:
		return &treeStr{treeHeader: h, Text: n.Text, Source: n.Source, off: n.off}
	case SoftBreak:
		return &treeSoftBreak{treeHeader: h}
	case HardBreak:
		return &treeHardBreak{treeHeader: h}
	case NonBreakingSpace:
		return &treeNonBreakingSpace{treeHeader: h}
	case Emph:
		return &treeEmph{treeHeader: h, Children: children}
	case Strong:
		return &treeStrong{treeHeader: h, Children: children}
	case Span:
		return &treeSpan{treeHeader: h, Children: children}
	case Mark:
		return &treeMark{treeHeader: h, Children: children}
	case Superscript:
		return &treeSuperscript{treeHeader: h, Children: children}
	case Subscript:
		return &treeSubscript{treeHeader: h, Children: children}
	case Insert:
		return &treeInsert{treeHeader: h, Children: children}
	case Delete:
		return &treeDelete{treeHeader: h, Children: children}
	case DoubleQuoted:
		return &treeDoubleQuoted{treeHeader: h, Children: children}
	case SingleQuoted:
		return &treeSingleQuoted{treeHeader: h, Children: children}
	case Verbatim:
		return &treeVerbatim{treeHeader: h, Text: n.Text}
	case InlineMath:
		return &treeInlineMath{treeHeader: h, Text: n.Text}
	case DisplayMath:
		return &treeDisplayMath{treeHeader: h, Text: n.Text}
	case RawInline:
		return &treeRawInline{treeHeader: h, Format: n.Lang, Text: n.Text}
	case URL:
		return &treeURL{treeHeader: h, Text: n.Text}
	case Email:
		return &treeEmail{treeHeader: h, Text: n.Text}
	case FootnoteReference:
		return &treeFootnoteReference{treeHeader: h, Label: n.Text}
	case Symb:
		return &treeSymb{treeHeader: h, Alias: n.Text}
	case Link:
		return &treeLink{treeHeader: h, Destination: n.Destination, Reference: n.Reference, Children: children}
	case Image:
		return &treeImage{treeHeader: h, Destination: n.Destination, Reference: n.Reference, Children: children}
	case Ellipsis:
		return &treeEllipsis{treeHeader: h}
	case EmDash:
		return &treeEmDash{treeHeader: h}
	case EnDash:
		return &treeEnDash{treeHeader: h}
	}
	panic("gjot: fromLegacy: unknown tag " + string(n.Tag))
}
