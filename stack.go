package gjot

import "strings"

// stack.go is the block parser as a container stack, in the shape of zjot
// v2.0.0's Parser.zig (itself djot.js's block.js), built beside the window
// parser behind withStack() (farraway/gridapp#12). Each line is run against
// the open containers from the outermost in (continues), then against the
// block starts while the innermost matched container holds blocks (tryOpen);
// what remains is lazy text for an open paragraph or heading, a blank line,
// or text for the innermost container. A container closes when a line
// neither continues it nor is lazy text for it. Ids are registered where
// they stand (document.go); references, footnotes and sections are resolved
// after the pass.
//
// Not ported from Parser.zig: block arenas, the held table, done/pending,
// the id log and its counters, and the nesting limit. Lists, tables and
// captions are stubs in this slice (slices 3 and 4 fill them).

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
	aligns      []CellAlign
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

// leafStart is where a leaf line's content starts.
type leafStart struct{ pos SourcePos }

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

// parseStack parses input with the container stack into the same tree the
// window parser builds.
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
	case kindList, kindListItem:
		return false // slice 3
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
		return false // slice 4
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

// stackMarker is a list marker at the start of a line's content. Filled in
// by slice 3.
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

// listMarker reads a list marker at the start of rest. A stub until slice
// 3: no list opens yet.
func listMarker(rest string) (stackMarker, bool) { return stackMarker{}, false }

// tryOpen tries the block starts at cur, in djot.js's order; on one, it
// closes the containers above matched, pushes the new one and moves cur
// past its marker, or to the line's end for a start that takes the line.
func (p *stackParser) tryOpen(cur *cursor, matched int) bool {
	r := cur.rest()
	s := cur.stand()
	if matched > 0 && p.stack[matched-1].kind.content() == contentItems {
		return false // an item under a list: slice 3
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
	// A caption ('^ ' at a block start): slice 4.
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
	if _, ok := listMarker(r); ok {
		return false // slice 3
	}
	// A table row: slice 4.
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

// tableRow reads a table row at cur. A stub until slice 4.
func (p *stackParser) tableRow(c *container, cur *cursor) {}

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
// line. Trailing blanks stay: the inline parser trims them.
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
	p.leafStarts = append(p.leafStarts, leafStart{pos: start})
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
		m.segments = append(m.segments, lineSegment{joinedStart: joined, orig: p.leafStarts[i].pos})
		joined += len(s) + 1
	}
	return m
}

func (p *stackParser) dropLeaf() {
	p.leafSlices = p.leafSlices[:0]
	p.leafStarts = p.leafStarts[:0]
}

// leafInlines is the open leaf's inline content, and the text it was read
// from; it empties the leaf buffers. The ids it holds are registered: those
// of inline attributes left on no node first, then the tree's.
func (p *stackParser) leafInlines(attrsFrom int) ([]*Node, string) {
	defer p.dropLeaf()
	src := strings.Join(p.leafSlices, "\n")
	if src == "" {
		return nil, ""
	}
	nodes, dropped := parseLeafInlines(src, attrsFrom)
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
	children, src := p.leafInlines(attrsFrom)
	n := &Node{Tag: tag, Level: level, Children: children}
	if tag == Heading {
		n.idText = headingIDText(src, children)
	}
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
// space: its newline unless spaces trail. A trailing tab is content.
func afterContent(l srcLine) SourcePos {
	trimmed := len(strings.TrimRight(l.text, " "))
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
				if (*slot).id != "" && c.data.attrParser.writesID() {
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
		p.doc.defs.footnoteDefs[c.data.label] = def
		return
	case kindDiv:
		end := s.pos
		if c.data.end != nil {
			end = *c.data.end
		}
		n = p.spanned(&Node{Tag: Div, Children: c.children, Classes: c.data.class}, c.start, end)
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
	default:
		// Lists, items, tables and captions: slices 3 and 4.
		p.dropLeaf()
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

// tableBefore is the last sibling, if a table: the table a caption closing
// now joins (slice 4).
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
