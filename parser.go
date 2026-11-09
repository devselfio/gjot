package gjot

import (
	"strings"
)

// parser.go is the block parser as a container stack, in the shape of zjot
// v2.0.0's Parser.zig (itself djot.js's block.js). Each line is run against
// the open containers from the outermost in (continues), then against the
// block starts while the innermost matched container holds blocks (tryOpen);
// what remains is lazy text for an open paragraph or heading, a blank line,
// or text for the innermost container. A container closes when a line
// neither continues it nor is lazy text for it. Ids are registered where
// they stand (document.go); references, footnotes and sections are resolved
// after the pass.
//
// Not ported from Parser.zig: block arenas, the held table, done/pending,
// the id log and its counters, and the nesting limit.

// srcLine is one line of the input, without its line end (zjot Lines.zig):
// where it starts, where its newline is (eol: col 0 of the next line, or the
// input's end for a last line without one), whether it had one, and the
// offset of the newline before it.
type srcLine struct {
	text          string
	pos, eol      SourcePos
	newline       bool
	newlineBefore int
}

// splitLines splits input into lines: '\n' ends a line, one '\r' before it
// is dropped, and the newline after the last line yields no empty line.
func splitLines(input string) []srcLine {
	var out []srcLine
	line, offset := 0, 0
	for rest := input; rest != ""; {
		raw, newline := rest, false
		if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
			raw, rest, newline = rest[:nl], rest[nl+1:], true
		} else {
			rest = ""
		}
		text := raw
		if newline && strings.HasSuffix(raw, "\r") {
			text = raw[:len(raw)-1]
		}
		line++
		l := srcLine{text: text, pos: SourcePos{Line: line, Col: 1, Offset: offset}, newline: newline, newlineBefore: max(offset-1, 0)}
		offset += len(raw)
		if newline {
			offset++
			l.eol = SourcePos{Line: line + 1, Col: 0, Offset: offset - 1}
		} else {
			l.eol = SourcePos{Line: line, Col: len(text) + 1, Offset: offset}
		}
		out = append(out, l)
	}
	return out
}

// inputEnd is where the input ends, as zjot's Lines.end: past the last
// line's newline (col 0 of the line after), or after its last byte.
func inputEnd(lines []srcLine) SourcePos {
	if len(lines) == 0 {
		return SourcePos{Line: 1, Col: 0, Offset: 0}
	}
	return lines[len(lines)-1].eol
}

// cursor is a position on the current line.
type cursor struct {
	line srcLine
	i    int
	// indent is the columns before i after skipSpace, a tab one column.
	indent int
}

func (c *cursor) skipSpace() {
	t := c.line.text
	for c.i < len(t) && (t[c.i] == ' ' || t[c.i] == '\t') {
		c.i++
	}
	c.indent = c.i
}

func (c *cursor) rest() string { return c.line.text[c.i:] }
func (c *cursor) atEOL() bool  { return c.i >= len(c.line.text) }

func (c *cursor) pos() SourcePos {
	if c.atEOL() {
		return c.line.eol
	}
	return c.at(c.i)
}

// at is the position of byte i of the line.
func (c *cursor) at(i int) SourcePos {
	return SourcePos{Line: c.line.pos.Line, Col: i + 1, Offset: c.line.pos.Offset + i}
}

// before is one position before pos: the byte before it, or col 0 (the
// previous line's newline) at the line's start.
func (c *cursor) before() SourcePos {
	if c.i == 0 {
		return SourcePos{Line: c.line.pos.Line, Col: 0, Offset: c.line.newlineBefore}
	}
	return c.at(c.i - 1)
}

func (c *cursor) stand() stand {
	s := stand{pos: c.pos(), before: c.before(), lineFirst: c.line.pos}
	if c.line.text == "" {
		s.lineFirst = c.line.eol
	}
	return s
}

// stand is where the parser stands when a container closes.
type stand struct {
	// pos is the current position: the next block's content, or a blank
	// line's newline. before is one position before it.
	pos, before SourcePos
	// lineFirst is the current line's first byte (its newline when empty),
	// where a block ending in a hard break ends.
	lineFirst SourcePos
	eof       bool
	// inclusive: the block closing ended on the current line (a code
	// block's closing fence).
	inclusive bool
}

type kind int

const (
	kindPara kind = iota
	kindHeading
	kindThematicBreak
	kindAttributes
	kindBlockQuote
	kindList
	kindListItem
	kindFootnote
	kindDiv
	kindCodeBlock
	kindTable
	kindCaption
	kindReferenceDefinition
)

// content is what a container holds.
type content int

const (
	contentBlock content = iota
	contentItems
	contentInlines
	contentText
	contentCells
	contentNone
	contentAttributes
)

func (k kind) content() content {
	switch k {
	case kindPara, kindHeading, kindCaption:
		return contentInlines
	case kindThematicBreak, kindReferenceDefinition:
		return contentNone
	case kindAttributes:
		return contentAttributes
	case kindList:
		return contentItems
	case kindCodeBlock:
		return contentText
	case kindTable:
		return contentCells
	}
	return contentBlock
}

// makesNode reports whether the container becomes a node of its parent: a
// definition, an attribute block and a caption do not, so they do not take
// the id waiting for the next block as they open.
func (k kind) makesNode() bool {
	switch k {
	case kindAttributes, kindFootnote, kindCaption, kindReferenceDefinition:
		return false
	}
	return true
}

// attrFailure is how an attribute block's lines failed, if they did.
type attrFailure int

const (
	attrNotFailed attrFailure = iota
	// attrFailedEOL: right at a line end, where a blank line next makes the
	// paragraph and counts as a blank line for the list around.
	attrFailedEOL
	// attrFailedElsewhere: only a line not indented past the opener makes
	// the paragraph.
	attrFailedElsewhere
)

type listKind int

const (
	listBullet listKind = iota
	listTask
	listDefinition
	listOrdered
)

// listData is what a list's items share and parse.js's tightness state.
// Filled in by slice 3.
type listData struct {
	style      listKind
	bullet     byte
	ordered    orderedStyles
	delim      delim
	indent     int
	markerText string
	blanklines bool
	tight      bool
}

// containerData has the fields of every kind; the container's kind says
// which apply (farraway/gridapp#12, Decision 3).
type containerData struct {
	// heading
	level int
	// bareAt: the first bare '#' line after the heading's last content
	// line, where it ends when that content ends in a hard break.
	bareAt *SourcePos
	// para: no inline attribute is read before attrsFrom, the end of the
	// lines a failed block attribute took; converted says it was one.
	attrsFrom int
	converted bool
	// attributes, footnote, list item, reference definition, code block:
	// the opener's indent.
	indent     int
	attrs      *blockAttrs
	attrParser *attrParser
	attrFailed attrFailure
	list       listData
	checked    *bool
	// footnote and reference definition: the normalized label.
	label string
	fence fenceInfo
	// div and code block: the closing fence's last character, once met;
	// thematic break: its line's end.
	end         *SourcePos
	divFenceLen int
	class       string
	// table: the alignments the last separator row gave, and the row on
	// the line before, which a separator row makes a header row.
	aligns    []CellAlign
	rowBefore *Node
}

type container struct {
	kind     kind
	start    SourcePos
	children []*Node
	// pendingAttrs are block attributes waiting for this container's next
	// child block.
	pendingAttrs *blockAttrs
	// leafEnd: for a paragraph, heading or attribute block, one past the
	// last byte of its last line that is not a trailing space.
	leafEnd SourcePos
	data    containerData
}

// leafStart is where a leaf line's content starts and where its newline is.
type leafStart struct{ pos, eol SourcePos }

type stackParser struct {
	lines    []srcLine
	doc      *document
	trackPos bool
	stack    []*container
	// pendingAttrs are block attributes waiting for the next top-level
	// block.
	pendingAttrs *blockAttrs
	blocks       []*Node
	// The open leaf's lines; the leaf is always the tip.
	leafSlices      []string
	leafStarts      []leafStart
	leafLastEOL     SourcePos
	leafLastNewline bool
	// finishedLine is set by a line a code fence (opening or closing), a
	// div's opening fence or a table row took: nothing else on it is read.
	finishedLine bool
	// blankTaken: the blank line being parsed was taken by a block
	// attribute it failed, so it is no blank line for the list around.
	blankTaken    bool
	footnoteDepth int
	// lineCount is the lines consumed, lastBlockLine the line the last
	// top-level block ended on.
	lineCount, lastBlockLine int
}

// parseStack parses input with the container stack into the whole document
// tree: the root Section that Parse returns.
func parseStack(input string, trackPos bool) *Node {
	p := &stackParser{lines: splitLines(input), doc: newDocument(trackPos), trackPos: trackPos, leafLastNewline: true}
	blocks := p.parseAll()
	var end *SourcePos
	if trackPos {
		e := inputEnd(p.lines)
		if p.lineCount <= p.lastBlockLine && len(blocks) > 0 && blocks[len(blocks)-1].Pos != nil {
			e = blocks[len(blocks)-1].Pos.End
		}
		end = &e
	}
	return p.doc.finish(blocks, end)
}

// parseAll runs every line through parseLine and closes what is open at
// the end.
func (p *stackParser) parseAll() []*Node {
	for _, l := range p.lines {
		p.lineCount++
		p.parseLine(l)
	}
	end := inputEnd(p.lines)
	p.closeFrom(0, stand{pos: end, before: end, lineFirst: end, eof: true})
	return p.blocks
}

func (p *stackParser) tip() *container { return p.stack[len(p.stack)-1] }

// holdsBlocks reports whether the nth open container (none when n is 0)
// holds blocks or items, so starts are tried after it.
func (p *stackParser) holdsBlocks(n int) bool {
	if n == 0 {
		return true
	}
	switch p.stack[n-1].kind.content() {
	case contentBlock, contentItems:
		return true
	}
	return false
}

func (p *stackParser) parseLine(l srcLine) {
	cur := &cursor{line: l}
	p.finishedLine = false
	p.blankTaken = false
	// Open containers continue from the outermost in; the first that does
	// not ends the run.
	matched := 0
	for _, c := range p.stack {
		cur.skipSpace()
		if !p.continues(c, cur) {
			break
		}
		matched++
	}
	if p.finishedLine {
		// A closing code fence or a table row took the line: a fence closes
		// its code block, which ends on this line.
		s := cur.stand()
		s.inclusive = true
		p.closeFrom(matched, s)
		return
	}
	// New containers open while the innermost matched one holds blocks.
	cur.skipSpace()
	newStarts := false
	for !cur.atEOL() && p.holdsBlocks(matched) {
		if !p.tryOpen(cur, matched) {
			break
		}
		matched = len(p.stack)
		newStarts = true
		if p.finishedLine {
			return
		}
		cur.skipSpace()
	}
	// The rest of the line: lazy text continues an open paragraph or
	// heading that no longer matched, when nothing opened and the line is
	// not blank; otherwise the containers that did not match close here.
	cur.skipSpace()
	blank := cur.atEOL()
	lazy := !blank && !newStarts && matched < len(p.stack) && p.tip().kind.content() == contentInlines
	if !lazy {
		p.closeFrom(matched, cur.stand())
	}
	if p.holdsBlocks(len(p.stack)) {
		if blank {
			if !newStarts && !p.blankTaken {
				p.blankLine()
			}
		} else {
			p.push(kindPara, containerData{}, cur.pos())
		}
	}
	if len(p.stack) > 0 {
		switch p.tip().kind.content() {
		case contentInlines:
			if !blank {
				p.addLeafLine(cur)
			}
		case contentText:
			p.addCodeLine(cur)
		}
	}
}

// continues reports whether the line at cur continues c, moving cur past
// the prefix it consumes.
func (p *stackParser) continues(c *container, cur *cursor) bool {
	switch c.kind {
	case kindPara, kindCaption:
		return !cur.atEOL()
	case kindHeading:
		r := cur.rest()
		hashes := countLeadingChar(r, '#')
		if hashes != c.data.level || (hashes < len(r) && r[hashes] != ' ' && r[hashes] != '\t') {
			return false
		}
		// A bare marker line adds no text.
		if c.data.bareAt == nil && isBlank(r[hashes:]) {
			at := cur.line.pos
			c.data.bareAt = &at
		}
		cur.i += hashes
		return true
	case kindThematicBreak:
		return false
	case kindAttributes:
		return p.continuesAttributes(c, cur)
	case kindBlockQuote:
		if !startsBlockQuote(cur.rest()) {
			return false
		}
		cur.i++
		return true
	case kindList:
		// A blank line or a deeper indent continues a list; at or left of
		// its indent, only a marker it accepts does (its item then opens).
		if cur.atEOL() || cur.indent > c.data.list.indent {
			return true
		}
		sm, ok := listMarker(cur.rest())
		return ok && c.data.list.accepts(sm)
	case kindListItem:
		return cur.atEOL() || cur.indent > c.data.indent
	case kindFootnote:
		return cur.atEOL() || cur.indent > c.data.indent
	case kindDiv:
		// A code block's lines are its own, whatever they look like.
		if p.tip().kind == kindCodeBlock {
			return true
		}
		r := cur.rest()
		if closesDiv(r, c.data.divFenceLen) {
			end := cur.at(cur.i + countLeadingChar(r, ':') - 1)
			c.data.end = &end
			cur.i = len(cur.line.text)
			return false
		}
		return true
	case kindCodeBlock:
		r := cur.rest()
		if isClosingCodeFence(r, c.data.fence) {
			end := cur.at(cur.i + countLeadingChar(r, c.data.fence.char) - 1)
			c.data.end = &end
			cur.i = len(cur.line.text)
			p.finishedLine = true
			return false
		}
		return true
	case kindTable:
		// A row continues the table; any other line ends it.
		if !isTableRow(cur.rest()) {
			return false
		}
		p.tableRow(c, cur)
		return true
	case kindReferenceDefinition:
		// A deeper-indented line holding one word continues the destination.
		r := cur.rest()
		if cur.atEOL() || cur.indent <= c.data.indent || strings.ContainsAny(r, " \t") {
			return false
		}
		p.addLeafSlice(r, cur.pos(), cur.line)
		cur.i = len(cur.line.text)
		return true
	}
	return false
}

// continuesAttributes is continues for a block attribute: lines indented
// past the opener keep it going until it parses; one that fails makes it a
// paragraph (zjot Parser.zig, djot.js).
func (p *stackParser) continuesAttributes(c *container, cur *cursor) bool {
	d := &c.data
	if d.attrs != nil {
		return false
	}
	// Failed at its line end, a blank line makes it a paragraph that ends
	// here, and the line is the list's blank line too.
	if d.attrFailed == attrFailedEOL && cur.atEOL() {
		p.convertToPara(c)
		return false
	}
	if cur.indent > d.indent {
		line := strings.TrimLeft(cur.rest(), " \t")
		p.addLeafSlice(line, cur.pos(), cur.line)
		if cur.atEOL() {
			c.leafEnd = cur.line.pos
		} else {
			c.leafEnd = afterContent(cur.line)
		}
		switch d.attrFailed {
		case attrFailedElsewhere:
		case attrFailedEOL:
			d.attrFailed = attrFailedElsewhere
		case attrNotFailed:
			switch status, at := d.attrParser.feedLine(line); status {
			case attrDone:
				a := d.attrParser.finish()
				d.attrs = &a
			case attrFail:
				switch {
				case at >= len(line):
					d.attrFailed = attrFailedEOL
				case isBlank(line[at+1:]):
					p.convertToPara(c)
				default:
					d.attrFailed = attrFailedElsewhere
				}
			}
		}
		cur.i = len(cur.line.text)
		return true
	}
	// It never parsed: its lines are a paragraph, which this line continues
	// or not as a paragraph's would.
	p.convertToPara(c)
	p.blankTaken = cur.atEOL()
	return !cur.atEOL()
}

// stackMarker is a list marker at the start of a line's content: the kind
// of list it opens or continues, what its items share, how many bytes the
// cursor moves over it (the marker byte alone for a bullet or definition
// marker; for an ordered marker, up to the rest its parser leaves, and for
// a task marker that and the box; spaces left after are skipped by the
// cursor either way), and a task item's box.
type stackMarker struct {
	kind    listKind
	bullet  byte
	ordered orderedStyles
	delim   delim
	advance int
	checked *bool
	text    string
	m       marker
}

// listMarker reads a list marker at the start of rest, as djot.js's
// getListStyles: a bullet followed by a task box is the task style whatever
// the bullet, a bare bullet its own style, ':' a definition, and an ordered
// marker the styles it may be with its delimiter.
func listMarker(rest string) (stackMarker, bool) {
	if m, ok := parseBulletMarker(rest); ok {
		width := len(rest) - len(m.rest)
		if isTaskItem(m.rest) {
			checked := m.rest[1] != ' '
			return stackMarker{kind: listTask, bullet: m.bullet, advance: width + 3, checked: &checked, text: m.markerText, m: m}, true
		}
		return stackMarker{kind: listBullet, bullet: m.bullet, advance: 1, text: m.markerText, m: m}, true
	}
	if rest != "" && rest[0] == ':' && (len(rest) == 1 || isSpaceOrTab(rest[1])) {
		return stackMarker{kind: listDefinition, bullet: ':', advance: 1, text: ":"}, true
	}
	if m, ok := parseOrderedMarker(rest); ok {
		return stackMarker{kind: listOrdered, ordered: orderedStyles{possible: m.styles}, delim: m.delim, advance: len(rest) - len(m.rest), text: m.markerText, m: m}, true
	}
	return stackMarker{}, false
}

// accepts reports whether marker sm continues list l: the same kind, the
// same bullet for a bullet list, and for an ordered list the same delimiter
// and a style the list may still be.
func (l *listData) accepts(sm stackMarker) bool {
	if sm.kind != l.style {
		return false
	}
	switch l.style {
	case listBullet:
		return sm.bullet == l.bullet
	case listOrdered:
		return sm.delim == l.delim && l.ordered.matches(sm.m)
	}
	return true
}

// tryOpen tries the block starts at cur, in djot.js's order; on one, it
// closes the containers above matched, pushes the new one and moves cur
// past its marker, or to the line's end for a start that takes the line.
func (p *stackParser) tryOpen(cur *cursor, matched int) bool {
	r := cur.rest()
	s := cur.stand()
	if matched > 0 && p.stack[matched-1].kind.content() == contentItems {
		// Under a list only its next item opens: the item before closes
		// here, and an ordered list narrows to the styles both share.
		list := &p.stack[matched-1].data.list
		sm, ok := listMarker(r)
		if !ok || !list.accepts(sm) {
			return false
		}
		p.closeFrom(matched, s)
		if list.style == listOrdered {
			list.ordered.accept(sm.m)
		}
		p.push(kindListItem, containerData{indent: cur.indent, checked: sm.checked}, cur.pos())
		cur.i += sm.advance
		return true
	}
	if startsBlockQuote(r) {
		p.closeFrom(matched, s)
		p.push(kindBlockQuote, containerData{}, cur.pos())
		cur.i++
		return true
	}
	if level := headingLevel(r); level > 0 {
		p.closeFrom(matched, s)
		p.push(kindHeading, containerData{level: level}, cur.pos())
		p.tip().leafEnd = afterContent(cur.line)
		cur.i += level
		return true
	}
	// A caption ('^ ' at a block start): what is open above closes where
	// its text starts; it joins the table before it as it closes.
	if len(r) > 1 && r[0] == '^' && isSpaceOrTab(r[1]) {
		cur.i++
		cur.skipSpace()
		p.closeFrom(matched, cur.stand())
		p.push(kindCaption, containerData{}, cur.pos())
		p.tip().leafEnd = cur.pos()
		return true
	}
	if def, ok := parseDefinitionLabel(r); ok {
		if def.footnote() {
			p.closeFrom(matched, s)
			// Its own id, in the block attributes waiting for it, is
			// registered where it is written, before its content's.
			if ba := *p.attrsSlot(); ba != nil {
				p.doc.ids.register(ba.id)
			}
			p.push(kindFootnote, containerData{label: normalizeLabel(def.label), indent: cur.indent}, cur.pos())
			p.footnoteDepth++
			cur.i += def.restCol
			return true
		}
		// A reference definition's destination is one word, to the end of
		// the line.
		dest := strings.TrimRight(def.rest, " \t")
		if !strings.ContainsAny(dest, " \t") && def.restCol+len(dest) == len(r) {
			p.closeFrom(matched, s)
			p.push(kindReferenceDefinition, containerData{label: normalizeLabel(def.refLabel()), indent: cur.indent}, cur.pos())
			if dest != "" {
				p.addLeafSlice(dest, cur.at(cur.i+def.restCol), cur.line)
			}
			cur.i = len(cur.line.text)
			return true
		}
	}
	if isThematicBreak(r) {
		p.closeFrom(matched, s)
		end := cur.line.eol
		p.push(kindThematicBreak, containerData{end: &end}, cur.pos())
		cur.i = len(cur.line.text)
		return true
	}
	if sm, ok := listMarker(r); ok {
		// The list opens on its first marker and leaves cur there, so the
		// item opens under it on the next try.
		p.closeFrom(matched, s)
		p.push(kindList, containerData{list: listData{
			style: sm.kind, bullet: sm.bullet, ordered: sm.ordered, delim: sm.delim,
			indent: cur.indent, markerText: sm.text, tight: true,
		}}, cur.pos())
		return true
	}
	if isTableRow(r) {
		p.closeFrom(matched, s)
		p.push(kindTable, containerData{}, cur.pos())
		p.tableRow(p.tip(), cur)
		return true
	}
	// A block attribute opens only when its first line parses so far or
	// completes at its '}'; a line that fails is a paragraph.
	if r[0] == '{' && opensAttrBlock(r) {
		p.closeFrom(matched, s)
		ap := newAttrParser("")
		status, _ := ap.feedLine(r)
		p.push(kindAttributes, containerData{indent: cur.indent, attrParser: ap}, cur.pos())
		p.addLeafSlice(r, cur.pos(), cur.line)
		p.tip().leafEnd = afterContent(cur.line)
		if status == attrDone {
			a := ap.finish()
			p.tip().data.attrs = &a
		}
		cur.i = len(cur.line.text)
		return true
	}
	if dv, ok := isFencedDivStart(r); ok && divClass(dv.class) {
		p.closeFrom(matched, s)
		p.push(kindDiv, containerData{divFenceLen: len(dv.fence), class: dv.class}, cur.pos())
		cur.i = len(cur.line.text)
		p.finishedLine = true
		return true
	}
	if fence, ok := isCodeFence(r); ok {
		p.closeFrom(matched, s)
		p.push(kindCodeBlock, containerData{fence: fence, indent: cur.indent}, cur.pos())
		cur.i = len(cur.line.text)
		p.finishedLine = true
		return true
	}
	return false
}

// tableRow reads the table row at cur into table c and takes the line. A
// separator row sets the alignments of the rows after it and makes the row
// on the line before it, if any, a header row with those alignments; any
// other row becomes a Row of Cells aligned as the last separator said.
func (p *stackParser) tableRow(c *container, cur *cursor) {
	r := cur.rest()
	p.finishedLine = true
	defer func() { cur.i = len(cur.line.text) }()
	if isTableSep(r) {
		aligns := parseSepAligns(r)
		c.data.aligns = aligns
		if row := c.data.rowBefore; row != nil {
			row.Head = true
			for i, cell := range row.Children {
				cell.Head = true
				cell.Align = alignAt(aligns, i)
			}
		}
		c.data.rowBefore = nil
		return
	}
	row := &Node{Tag: Row}
	// Each cell's text starts after the pipe before it; the cells are
	// contiguous, one pipe apart.
	off := cur.i + 1
	for i, text := range rowCells(r) {
		lead := len(text) - len(strings.TrimLeft(text, " \t"))
		content := trimCellEnd(text[lead:])
		if content != "" {
			p.addLeafSlice(content, cur.at(off+lead), cur.line)
		}
		// No line end follows a cell: its trailing tab is content.
		children, _ := p.leafInlines(0, false)
		cell := &Node{Tag: Cell, Children: children, Align: alignAt(c.data.aligns, i)}
		row.Children = append(row.Children, p.spanned(cell, cur.at(off-1), cur.at(off+len(text))))
		off += len(text) + 1
	}
	// A row ends one past its last pipe: a trailing tab is not content.
	p.spanned(row, cur.pos(), afterBlanks(cur.line, " \t"))
	c.children = append(c.children, row)
	c.data.rowBefore = row
}

// alignAt is column i's alignment, the default past the separator's cells.
func alignAt(aligns []CellAlign, i int) CellAlign {
	if i < len(aligns) {
		return aligns[i]
	}
	return AlignDefault
}

// trimCellEnd trims a cell's trailing spaces, but keeps one that a
// backslash escapes.
func trimCellEnd(text string) string {
	t := strings.TrimRight(text, " ")
	if len(t) < len(text) {
		bs := len(t) - len(strings.TrimRight(t, "\\"))
		if bs%2 == 1 {
			return text[:len(t)+1]
		}
	}
	return t
}

// push opens a container. One that makes a node registers the id waiting
// for it now, where djot.js reads it.
func (p *stackParser) push(k kind, data containerData, start SourcePos) {
	p.listEvent(k == kindList, k == kindListItem)
	if k.makesNode() {
		if ba := *p.attrsSlot(); ba != nil {
			p.doc.ids.register(ba.id)
		}
	}
	p.stack = append(p.stack, &container{kind: k, start: start, data: data})
}

// attrsSlot is where block attributes wait: with the innermost open
// container, or with the parser for the next top-level block.
func (p *stackParser) attrsSlot() **blockAttrs {
	if len(p.stack) > 0 {
		return &p.tip().pendingAttrs
	}
	return &p.pendingAttrs
}

// blankLine marks the innermost list for tightness, and drops the waiting
// block attribute: block attributes do not survive a blank line (zjot.test
// difference 1).
func (p *stackParser) blankLine() {
	if ln := p.listNode(); ln != nil {
		ln.data.list.blanklines = true
	}
	*p.attrsSlot() = nil
}

// listEvent is parse.js's tightness rule on every event but a blank line:
// after blank lines, anything but a list's or item's start or end makes the
// list loose, and anything but an item's start or end clears the mark.
func (p *stackParser) listEvent(isList, isItem bool) {
	ln := p.listNode()
	if ln == nil {
		return
	}
	l := &ln.data.list
	if !isList && !isItem && l.blanklines {
		l.tight = false
	}
	if !isItem {
		l.blanklines = false
	}
}

// listNode is the innermost list a tightness event reaches: the tip, or the
// list the tip's item is in.
func (p *stackParser) listNode() *container {
	n := len(p.stack)
	if n > 0 && p.stack[n-1].kind == kindList {
		return p.stack[n-1]
	}
	if n > 1 && p.stack[n-2].kind == kindList {
		return p.stack[n-2]
	}
	return nil
}

// convertToPara turns a failed attribute block into a paragraph where it
// stands.
func (p *stackParser) convertToPara(c *container) {
	c.data.attrsFrom = p.failedAttrsFrom()
	c.data.converted = true
	c.kind = kindPara
}

// failedAttrsFrom is where inline attributes may start in a failed block
// attribute's paragraph: after every line it fed the attribute parser when
// the first left it open, as djot.js reparses them (reparseAttributes).
func (p *stackParser) failedAttrsFrom() int {
	if len(p.leafSlices) == 0 || !opensAttr(p.leafSlices[0]) {
		return 0
	}
	joined := len(p.leafSlices) - 1
	for _, s := range p.leafSlices {
		joined += len(s)
	}
	return joined
}

// addLeafLine adds the rest of the line at cur as the open leaf's next
// line. Trailing blanks stay: before a soft break they are content, and at
// the leaf's end the inline parser trims the spaces, as djot.js.
func (p *stackParser) addLeafLine(cur *cursor) {
	p.addLeafSlice(cur.rest(), cur.pos(), cur.line)
	t := p.tip()
	t.leafEnd = afterContent(cur.line)
	if t.kind == kindHeading {
		t.data.bareAt = nil
	}
}

func (p *stackParser) addLeafSlice(text string, start SourcePos, l srcLine) {
	p.leafSlices = append(p.leafSlices, text)
	p.leafStarts = append(p.leafStarts, leafStart{pos: start, eol: l.eol})
	p.leafLastEOL = l.eol
	p.leafLastNewline = l.newline
}

// addCodeLine adds the line at cur as the open code block's next line: it
// loses the fence's indentation and keeps the rest, blank lines included.
func (p *stackParser) addCodeLine(cur *cursor) {
	keep := max(cur.indent-p.tip().data.indent, 0)
	p.addLeafSlice(cur.line.text[cur.i-keep:], cur.at(cur.i-keep), cur.line)
}

// leafLineMap maps the joined leaf text back to the source.
func (p *stackParser) leafLineMap() lineMap {
	var m lineMap
	joined := 0
	for i, s := range p.leafSlices {
		m.segments = append(m.segments, lineSegment{joinedStart: joined, orig: p.leafStarts[i].pos, eol: p.leafStarts[i].eol})
		joined += len(s) + 1
	}
	m.total = joined - 1
	return m
}

func (p *stackParser) dropLeaf() {
	p.leafSlices = p.leafSlices[:0]
	p.leafStarts = p.leafStarts[:0]
}

// leafInlines is the open leaf's inline content, and the text it was read
// from; it empties the leaf buffers. endsLine says whether a line end
// follows the text (a cell's does not). The ids it holds are registered:
// those of inline attributes left on no node first, then the tree's.
func (p *stackParser) leafInlines(attrsFrom int, endsLine bool) ([]*Node, string) {
	defer p.dropLeaf()
	src := strings.Join(p.leafSlices, "\n")
	if src == "" {
		return nil, ""
	}
	nodes, dropped := parseLeafInlines(src, attrsFrom, endsLine)
	for _, id := range dropped {
		p.doc.ids.register(id)
	}
	p.doc.ids.registerTree(nodes)
	if p.trackPos {
		applyInlinePositions(nodes, p.leafLineMap())
	}
	return nodes, src
}

// leafText is the open code block's text: its lines, each with its
// newline, but none after the input's last line when it had none.
func (p *stackParser) leafText() string {
	defer p.dropLeaf()
	var b strings.Builder
	for _, s := range p.leafSlices {
		b.WriteString(s)
		b.WriteByte('\n')
	}
	text := b.String()
	if !p.leafLastNewline && text != "" {
		text = text[:len(text)-1]
	}
	return text
}

// leafNode is a paragraph, heading or caption over the open leaf.
func (p *stackParser) leafNode(c *container, tag Tag, level, attrsFrom int, s stand) *Node {
	children, _ := p.leafInlines(attrsFrom, true)
	n := &Node{Tag: tag, Level: level, Children: children}
	if p.trackPos {
		n.Pos = &Range{Start: c.start, End: leafEnd(c, children, s)}
	}
	return n
}

// leafEnd is where a paragraph, heading or caption ends: a caption one
// before where the parser stands; the others after their last byte that is
// not a trailing space or, ending in a hard break, at the next line's first
// byte (a heading's first bare '#' line).
func leafEnd(c *container, children []*Node, s stand) SourcePos {
	if c.kind == kindCaption {
		return s.before
	}
	if n := len(children); n > 0 && children[n-1].Tag == HardBreak {
		if c.kind == kindHeading && c.data.bareAt != nil {
			return *c.data.bareAt
		}
		return s.lineFirst
	}
	return c.leafEnd
}

// afterContent is one past the last byte of l that is not a trailing
// space: its newline unless spaces trail. A trailing tab is content, as
// djot.js.
func afterContent(l srcLine) SourcePos {
	return afterBlanks(l, " ")
}

// afterBlanks is where l's content ends with blanks stripped from its end:
// its newline when none trail, else one past the last other byte.
func afterBlanks(l srcLine, blanks string) SourcePos {
	trimmed := len(strings.TrimRight(l.text, blanks))
	if trimmed == len(l.text) {
		return l.eol
	}
	return SourcePos{Line: l.pos.Line, Col: trimmed + 1, Offset: l.pos.Offset + trimmed}
}

// closeFrom closes the open containers past the first from, innermost
// first.
func (p *stackParser) closeFrom(from int, s stand) {
	for len(p.stack) > from {
		p.close(s)
	}
}

// spanned sets n's range when positions are tracked.
func (p *stackParser) spanned(n *Node, start, end SourcePos) *Node {
	if p.trackPos {
		n.Pos = &Range{Start: start, End: end}
	}
	return n
}

// close closes the tip. Its node joins its parent's children or the
// top-level blocks; a definition is recorded instead; a complete attribute
// block waits for the next block, one never completed is a paragraph.
func (p *stackParser) close(s stand) {
	t := p.tip()
	p.listEvent(t.kind == kindList, t.kind == kindListItem)
	p.stack = p.stack[:len(p.stack)-1]
	c := t
	var n *Node
	switch c.kind {
	case kindPara:
		// A failed attribute block that a line turned into a paragraph takes
		// the attributes waiting before it, and registers their id now, as
		// it opened as no node.
		if c.data.converted {
			if ba := *p.attrsSlot(); ba != nil {
				p.doc.ids.register(ba.id)
			}
		}
		n = p.leafNode(c, Para, 0, c.data.attrsFrom, s)
	case kindHeading:
		// Its inline ids are registered first, then the heading's own.
		n = p.leafNode(c, Heading, c.data.level, 0, s)
		explicit := ""
		if ba := *p.attrsSlot(); ba != nil {
			explicit = ba.id
		}
		p.doc.ids.heading(n, explicit)
	case kindThematicBreak:
		n = p.spanned(&Node{Tag: ThematicBreak}, c.start, *c.data.end)
	case kindAttributes:
		slot := p.attrsSlot()
		if a := c.data.attrs; a != nil {
			// Waits for the next block in the container it was written in.
			// An id a later one overrides was still read.
			if *slot != nil {
				if (*slot).id != "" && a.writesID() {
					p.doc.ids.register((*slot).id)
				}
				merged := (*slot).merge(*a)
				*slot = &merged
			} else {
				*slot = a
			}
			p.dropLeaf()
			return
		}
		// Never completed: its lines are a paragraph, which takes the
		// attributes waiting before it.
		if ba := *slot; ba != nil {
			p.doc.ids.register(ba.id)
		}
		n = p.leafNode(c, Para, 0, p.failedAttrsFrom(), s)
	case kindBlockQuote:
		n = p.spanned(&Node{Tag: BlockQuote, Children: c.children}, c.start, s.pos)
	case kindFootnote:
		// Recorded for the document, with the block attributes waiting
		// before it.
		p.footnoteDepth--
		slot := p.attrsSlot()
		def := &footnoteDef{blocks: c.children, attrs: *slot}
		if p.trackPos {
			def.pos = &Range{Start: c.start, End: s.pos}
		}
		*slot = nil
		p.doc.defs.defineFootnote(c.data.label, def)
		return
	case kindDiv:
		end := s.pos
		if c.data.end != nil {
			end = *c.data.end
		}
		div := &Node{Tag: Div, Children: c.children}
		if c.data.class != "" {
			div.Attrs = []Attr{{"class", c.data.class}}
		}
		n = p.spanned(div, c.start, end)
	case kindCodeBlock:
		text := p.leafText()
		end := s.pos
		if c.data.end != nil {
			end = *c.data.end
		}
		lang := c.data.fence.lang
		if strings.HasPrefix(lang, "=") {
			n = &Node{Tag: RawBlock, Text: text, Lang: lang[1:]}
		} else {
			n = &Node{Tag: CodeBlock, Text: text, Lang: lang}
		}
		p.spanned(n, c.start, end)
	case kindReferenceDefinition:
		url := strings.Join(p.leafSlices, "")
		p.dropLeaf()
		slot := p.attrsSlot()
		// An empty label defines nothing.
		if c.data.label != "" {
			p.doc.defs.refDefs[c.data.label] = refDefinition{url: url, attrs: *slot}
		}
		// Its id is registered where it is written, off any node.
		if ba := *slot; ba != nil {
			p.doc.ids.register(ba.id)
		}
		*slot = nil
		return
	case kindList:
		n = p.spanned(p.listNodeOf(c), c.start, s.pos)
	case kindListItem:
		n = p.spanned(p.itemNode(c), c.start, s.before)
	case kindTable:
		// djot.js gives every table a caption, empty until one follows it.
		children := append([]*Node{{Tag: Caption}}, c.children...)
		n = p.spanned(&Node{Tag: Table, Children: children}, c.start, s.pos)
	case kindCaption:
		p.closeCaption(c, s)
		return
	}
	p.closeAs(n, s)
}

// closeAs appends n, with the block attributes waiting for it, to its
// parent's children or the top-level blocks.
func (p *stackParser) closeAs(n *Node, s stand) {
	slot := p.attrsSlot()
	if ba := *slot; ba != nil {
		applyBlockAttrs(n, *ba)
		*slot = nil
	}
	if len(p.stack) > 0 {
		t := p.tip()
		t.children = append(t.children, n)
		return
	}
	p.blocks = append(p.blocks, n)
	// The block ended on the line before the one that closed it, unless
	// that line was its own (a code block's closing fence) or the last.
	if s.eof || s.inclusive {
		p.lastBlockLine = p.lineCount
	} else {
		p.lastBlockLine = p.lineCount - 1
	}
}

// closeCaption closes a caption: with the block attributes waiting before
// it, it replaces the caption the table before it holds (the empty one it
// was made with, or an earlier caption's); with no table before it, it is
// dropped. Its ids, and the waiting attributes' id, were registered either
// way.
func (p *stackParser) closeCaption(c *container, s stand) {
	n := p.leafNode(c, Caption, 0, 0, s)
	slot := p.attrsSlot()
	ba := *slot
	*slot = nil
	if ba != nil {
		p.doc.ids.register(ba.id)
	}
	table := p.tableBefore()
	if table == nil {
		return
	}
	if ba != nil {
		applyBlockAttrs(n, *ba)
	}
	table.Children[0] = n
}

// tableBefore is the last sibling, if a table: the table a caption closing
// now joins.
func (p *stackParser) tableBefore() *Node {
	siblings := p.blocks
	if len(p.stack) > 0 {
		siblings = p.tip().children
	}
	if n := len(siblings); n > 0 && siblings[n-1].Tag == Table {
		return siblings[n-1]
	}
	return nil
}

// listNodeOf is the node a closing list makes: a bullet or task list with
// its tightness and first bullet, an ordered list with djot.js's style
// (the style it settled on, in its delimiter) and its first marker's text,
// from which its start is read, or a definition list.
func (p *stackParser) listNodeOf(c *container) *Node {
	l := c.data.list
	switch l.style {
	case listTask:
		return &Node{Tag: TaskList, Children: c.children, Tight: l.tight, Style: string(rune(l.bullet))}
	case listDefinition:
		return &Node{Tag: DefinitionList, Children: c.children}
	case listOrdered:
		style := styleDecimal
		if len(l.ordered.possible) > 0 {
			style = l.ordered.possible[0]
		}
		return &Node{Tag: OrderedList, Children: c.children, Tight: l.tight, Style: styleString(style, l.delim), Text: l.markerText}
	}
	return &Node{Tag: BulletList, Children: c.children, Tight: l.tight, Style: string(rune(l.bullet))}
}

// itemNode is the node a closing item makes; its list is the tip. A
// definition list's item takes its first paragraph's inlines as the term
// and the rest as the definition. Only the paragraph's inlines move, so a
// block attribute on it (its id already registered) is not on the Term:
// today's tree, which the corpus does not cover.
func (p *stackParser) itemNode(c *container) *Node {
	switch p.tip().data.list.style {
	case listTask:
		return &Node{Tag: TaskListItem, Children: c.children, Checked: c.data.checked}
	case listDefinition:
		term, blocks := &Node{Tag: Term}, c.children
		if len(blocks) > 0 && blocks[0].Tag == Para {
			term.Children = blocks[0].Children
			blocks = blocks[1:]
		}
		return &Node{Tag: DefinitionListItem, Children: []*Node{term, {Tag: Definition, Children: blocks}}}
	}
	return &Node{Tag: ListItem, Children: c.children}
}
