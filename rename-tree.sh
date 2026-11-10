#!/bin/sh
# rename-tree.sh: the swap's mechanical rename (swarm-gridapp#8 slice 3).
# The per-kind types were built beside the old Node struct under a "tree"
# prefix, since their names collided with it and with the Tag constants;
# both are gone, so each takes its final name. Code is rewritten with
# gofmt -r, one rule per name; comments, which gofmt -r does not touch,
# with one sed over the same names. Run from the repository root; this
# script is removed in the commit after the one that runs it.
set -eu
files=$(ls *.go)
rename() {
	gofmt -l -w -r "$1 -> $2" $files >/dev/null
	sed -i "s/\b$1\b/$2/g" $files
}
rename treeNode Node
rename treeHeader Header
rename treeWalk Walk
rename treeUnmarshalNode UnmarshalNode
rename treeBlockQuote BlockQuote
rename treeBulletList BulletList
rename treeCaption Caption
rename treeCell Cell
rename treeCodeBlock CodeBlock
rename treeDefinition Definition
rename treeDefinitionList DefinitionList
rename treeDefinitionListItem DefinitionListItem
rename treeDelete Delete
rename treeDisplayMath DisplayMath
rename treeDiv Div
rename treeDoubleQuoted DoubleQuoted
rename treeEllipsis Ellipsis
rename treeEmDash EmDash
rename treeEmail Email
rename treeEmph Emph
rename treeEnDash EnDash
rename treeFootnote Footnote
rename treeFootnoteReference FootnoteReference
rename treeHardBreak HardBreak
rename treeHeading Heading
rename treeImage Image
rename treeInlineMath InlineMath
rename treeInsert Insert
rename treeLink Link
rename treeListItem ListItem
rename treeMark Mark
rename treeNonBreakingSpace NonBreakingSpace
rename treeOrderedList OrderedList
rename treePara Para
rename treeRawBlock RawBlock
rename treeRawInline RawInline
rename treeRow Row
rename treeSection Section
rename treeSingleQuoted SingleQuoted
rename treeSoftBreak SoftBreak
rename treeSpan Span
rename treeStr Str
rename treeStrong Strong
rename treeSubscript Subscript
rename treeSuperscript Superscript
rename treeSymb Symb
rename treeTable Table
rename treeTaskList TaskList
rename treeTaskListItem TaskListItem
rename treeTerm Term
rename treeThematicBreak ThematicBreak
rename treeURL URL
rename treeVerbatim Verbatim
