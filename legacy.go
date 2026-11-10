package gjot

// fromLegacy converts the old tree to the new: one node per node, the id,
// attributes and position into the header, each kind keeping only its own
// fields. A task list item's nil Checked reads as false; AutoID is kept on
// headings and sections only. This file goes when the parser builds the new
// tree directly.
func fromLegacy(n *Node) treeNode { return fromLegacyLinks(n, nil) }

// linkTexts holds the Text of each childless link that had one in the old
// tree, which the new link has no field for. HTML renders it in place of
// the children, as it did before the renderers read the new tree, and
// toLegacy hands it back to the hooks; ASTText and Djot never read a link's
// Text. Parse never sets it, so the map stays nil for a parsed document.
// It goes with this file.
type linkTexts map[*treeLink]string

// fromLegacyLinks is fromLegacy, recording into *texts, when texts is not
// nil, the Text of every childless link that has one.
func fromLegacyLinks(n *Node, texts *linkTexts) treeNode {
	var children []treeNode
	if len(n.Children) > 0 {
		children = make([]treeNode, len(n.Children))
		for i, c := range n.Children {
			children[i] = fromLegacyLinks(c, texts)
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
		l := &treeLink{treeHeader: h, Destination: n.Destination, Reference: n.Reference, Children: children}
		if texts != nil && len(children) == 0 && n.Text != "" {
			if *texts == nil {
				*texts = linkTexts{}
			}
			(*texts)[l] = n.Text
		}
		return l
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

// toLegacy converts the new tree to the old, the mirror of fromLegacy, for
// the hooks, which take the old node until the API swaps. A task list item's
// Checked is always set, as the parser sets it.
func toLegacy(n treeNode) *Node { return toLegacyLinks(n, nil) }

// toLegacyLinks is toLegacy, setting the Text of each link texts holds.
func toLegacyLinks(n treeNode, texts linkTexts) *Node {
	h := n.header()
	l := &Node{Tag: n.Tag(), ID: h.ID, Attrs: h.Attrs, Pos: h.Pos}
	if cs := childrenOf(n); len(cs) > 0 {
		l.Children = make([]*Node, len(cs))
		for i, c := range cs {
			l.Children[i] = toLegacyLinks(c, texts)
		}
	}
	switch n := n.(type) {
	case *treeSection:
		l.Level, l.AutoID = n.Level, n.AutoID
	case *treeHeading:
		l.Level, l.AutoID = n.Level, n.AutoID
	case *treeCodeBlock:
		l.Lang, l.Text = n.Lang, n.Text
	case *treeRawBlock:
		l.Lang, l.Text = n.Format, n.Text
	case *treeBulletList:
		l.Style, l.Tight = n.Style, n.Tight
	case *treeTaskList:
		l.Style, l.Tight = n.Style, n.Tight
	case *treeOrderedList:
		l.Style, l.Text, l.Tight = n.Style, n.Marker, n.Tight
	case *treeTaskListItem:
		checked := n.Checked
		l.Checked = &checked
	case *treeRow:
		l.Head = n.Head
	case *treeCell:
		l.Head, l.Align = n.Head, n.Align
	case *treeFootnote:
		l.Text = n.Label
	case *treeStr:
		l.Text, l.Source, l.off = n.Text, n.Source, n.off
	case *treeVerbatim:
		l.Text = n.Text
	case *treeInlineMath:
		l.Text = n.Text
	case *treeDisplayMath:
		l.Text = n.Text
	case *treeRawInline:
		l.Lang, l.Text = n.Format, n.Text
	case *treeURL:
		l.Text = n.Text
	case *treeEmail:
		l.Text = n.Text
	case *treeFootnoteReference:
		l.Text = n.Label
	case *treeSymb:
		l.Text = n.Alias
	case *treeLink:
		l.Destination, l.Reference = n.Destination, n.Reference
		l.Text = texts[n]
	case *treeImage:
		l.Destination, l.Reference = n.Destination, n.Reference
	}
	return l
}
