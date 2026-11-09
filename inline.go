package gjot

import (
	"strings"
	"unicode/utf8"
)

// Inline parsing with an opener stack. Text nodes sliced from the inline
// source carry their offset in Node.off so adjacent slices merge, heading
// ids can be derived from the raw source, and source positions can be
// applied after the fact.

type itemKind int

const (
	itemNode itemKind = iota
	itemOpener
	itemAttrs
)

type openerInfo struct {
	char    byte
	itemIdx int
	// srcPos is where the opener starts in the inline source: the "{" of
	// a braced opener, else the delimiter itself.
	srcPos int
	marked bool
	// link is set on a bracket whose "](" or "][" has come: djot.js's
	// explicit_link or reference_link, closed by the ")" or "]" the
	// forward pass comes to.
	link *linkOpen
}

// linkOpen is a bracket's "](" or "][": the link's text is the items before
// it, and the items from it on are the destination or label, which is read
// off the source instead.
type linkOpen struct {
	reference bool
	// itemIdx is the item of the "](" or "][".
	itemIdx int
	// srcPos is where its "]" is in the source.
	srcPos int
	// droppedLen is len(p.dropped) as it was then: an id read in the
	// destination or label is text once the link closes, which djot.js
	// does not register.
	droppedLen int
}

// touches reports whether a closer at pos would make an empty pair: the
// delimiters are adjacent in the source, whatever stood between them made
// no node (djot.js's opener.endpos === pos - 1; {_{}_} is an empty
// emphasis, {__} none). Such a closer closes nothing.
func (op openerInfo) touches(pos int) bool {
	return op.srcPos+openerLen(op) == pos
}

type inlineItem struct {
	kind   itemKind
	node   *Node
	opener openerInfo
	attrs  blockAttrs
	// An attribute item's source extent [attrsStart, attrsEnd); attrsDeep
	// says MaxNesting openers were open when it was read, so a span it
	// would make is one level too many (mayOpen) and it is text instead.
	attrsStart, attrsEnd int
	attrsDeep            bool
}

type inlineParser struct {
	src string
	// attrsFrom: no inline attribute is read before this offset (the text a
	// failed block attribute took, which djot.js reparses without them).
	attrsFrom int
	// endsLine: whether a line end follows src (a table cell's does not).
	// Where one does, a backslash ending src is a hard break, and the last
	// text's or an unclosed verbatim's trailing spaces go, as djot.js trims
	// them before the newline; a trailing tab is content.
	endsLine bool
	// dropped collects the ids of inline attributes left on no node, which
	// were registered all the same (djot.js).
	dropped   []string
	items     []inlineItem
	openers   []openerInfo
	pos       int
	textStart int
	// destination is set at a "](" and cleared when a link closes on its
	// ")", as djot.js's, which a destination never closed leaves set:
	// while set, "(" and ")" pair up, and a ")" with no "(" to pair
	// closes the nearest bracket's link when that bracket is an explicit
	// link.
	destination bool
	// parens is where each "(" read in the destination stands, unpaired
	// so far: djot.js's "(" openers, which a match over them clears as
	// it clears every opener inside it.
	parens []int
	// noFootnoteBefore: no footnote reference opens before here
	// (footnoteRefEnd).
	noFootnoteBefore int
	// tooDeep: an opener was refused at MaxNesting (mayOpen).
	tooDeep bool
}

// parseLeafInlines parses the inline source for the container stack: no
// inline attribute is read at an offset before attrsFrom, endsLine says
// whether a line end follows src, and the ids of inline attributes that
// attached to no node (or that a later id replaced) are returned rather than
// lost. tooDeep says an opener was refused at MaxNesting (mayOpen).
func parseLeafInlines(src string, attrsFrom int, endsLine bool) (nodes []*Node, droppedIDs []string, tooDeep bool) {
	p := &inlineParser{src: src, attrsFrom: attrsFrom, endsLine: endsLine}
	nodes = p.parse()
	return nodes, p.dropped, p.tooDeep
}

// strNode is a text node sliced from the inline source at off.
func strNode(text string, off int) *Node {
	return &Node{Tag: Str, Text: text, off: off + 1}
}

// quoteLit is the text node a smart quote that pairs with nothing leaves:
// the glyph it renders as, and the quote it was written as, which is what a
// heading id or reference label reads (djot.js smart_punctuation text).
func quoteLit(glyph, source string) *Node {
	return &Node{Tag: Str, Text: glyph, Source: source}
}

// at records n's extent [start, end) in the inline source and returns n.
func at(n *Node, start, end int) *Node {
	n.src = srcSpan{start, end}
	return n
}

// tail returns a text node for n.Text[start:], keeping the source offset of
// a slice or the extent of a literal. A literal whose text is not its source
// bytes (a smart quote) keeps its whole extent, since its text cannot be
// mapped byte for byte.
func tail(n *Node, start int) *Node {
	t := &Node{Tag: Str, Text: n.Text[start:]}
	if start == 0 {
		t.Source = n.Source // the whole literal: a smart quote keeps its quote
	}
	switch {
	case n.off > 0:
		t.off = n.off + start
	case n.src.end > 0 && start > 0 && n.src.end-n.src.start == len(n.Text):
		t.src = srcSpan{n.src.start + start, n.src.end}
	default:
		t.src = n.src
	}
	return t
}

func (p *inlineParser) pushNode(n *Node) {
	p.items = append(p.items, inlineItem{kind: itemNode, node: n})
}

func (p *inlineParser) pushOpener(op openerInfo) {
	p.openers = append(p.openers, op)
	p.items = append(p.items, inlineItem{kind: itemOpener, opener: op})
}

// mayOpen reports whether another opener may be pushed: MaxNesting are
// open at most, so the inlines of a leaf nest at most MaxNesting deep. A
// delimiter refused here is what it is when it cannot open.
func (p *inlineParser) mayOpen() bool {
	if len(p.openers) >= MaxNesting {
		p.tooDeep = true
		return false
	}
	return true
}

// addText appends the source slice text at off, extending the previous node
// when both are adjacent slices of the source. An open bracket is left
// alone: its opener points at it, and the link's text is the items after it.
func (p *inlineParser) addText(text string, off int) {
	n := len(p.items)
	openBracket := len(p.openers) > 0 && p.openers[len(p.openers)-1].itemIdx == n-1
	if n > 0 && !openBracket && p.items[n-1].kind == itemNode && p.joinText(p.items[n-1].node, strNode(text, off)) {
		return
	}
	p.pushNode(strNode(text, off))
}

// joinText extends the text node last by n when both are adjacent slices of
// the source: djot.js reports such text as one str. A node that is not a
// slice (a smart quote's glyph) joins nothing.
func (p *inlineParser) joinText(last, n *Node) bool {
	if last.Tag != Str || n.Tag != Str || last.off == 0 || n.off == 0 || last.off-1+len(last.Text) != n.off-1 {
		return false
	}
	last.Text = p.src[last.off-1 : n.off-1+len(n.Text)]
	return true
}

// appendText appends the text node n to nodes, joining it to the last node
// when it can: an opener that did not close leaves text djot.js reports as
// one str with its neighbours.
func (p *inlineParser) appendText(nodes []*Node, n *Node) []*Node {
	if len(nodes) > 0 && p.joinText(nodes[len(nodes)-1], n) {
		return nodes
	}
	return append(nodes, n)
}

func (p *inlineParser) flushText() {
	if p.pos > p.textStart {
		p.addText(p.src[p.textStart:p.pos], p.textStart)
	}
}

// closeAt wraps the items after opener i in a node of tag, spanning from the
// opener to end (just past the closing delimiter), and drops openers from
// index i up.
func (p *inlineParser) closeAt(i int, tag Tag, end int) {
	op := p.openers[i]
	children := p.collectChildren(op.itemIdx)
	p.pushNode(at(&Node{Tag: tag, Children: children}, op.srcPos, end))
	p.truncateOpeners(i)
}

// truncateOpeners drops the openers from n up, and the parens from where
// the lowest of them starts.
func (p *inlineParser) truncateOpeners(n int) {
	if n < len(p.openers) {
		p.dropParens(p.openers[n].srcPos)
	}
	p.openers = p.openers[:n]
}

// dropParens drops the parens read at or after from.
func (p *inlineParser) dropParens(from int) {
	for len(p.parens) > 0 && p.parens[len(p.parens)-1] >= from {
		p.parens = p.parens[:len(p.parens)-1]
	}
}

func (p *inlineParser) parse() []*Node {
	src := p.src
	for p.pos < len(src) {
		c := src[p.pos]
		switch c {
		case '\\':
			p.flushText()
			p.handleBackslash()
			p.textStart = p.pos
		case '`':
			p.flushText()
			p.handleBackticks()
			p.textStart = p.pos
		case '\n':
			p.flushText()
			p.pushNode(at(&Node{Tag: SoftBreak}, p.pos, p.pos+1))
			p.pos++
			p.textStart = p.pos
		// One rule for the four matched delimiters (djot.js's betweenMatched).
		case '*', '_', '~', '^':
			p.flushText()
			p.handleEmphDelimiter(c, false)
			p.textStart = p.pos
		case '{':
			if p.pos+1 < len(src) {
				switch next := src[p.pos+1]; next {
				case '_', '*', '~', '^':
					p.flushText()
					p.pos++
					p.handleEmphDelimiter(next, true)
					p.textStart = p.pos
					continue
				case '+', '-', '=':
					p.flushText()
					if p.mayOpen() {
						p.pushOpener(openerInfo{char: next, itemIdx: len(p.items), srcPos: p.pos, marked: true})
					} else {
						p.addText(src[p.pos:p.pos+2], p.pos)
					}
					p.pos += 2
					p.textStart = p.pos
					continue
				case '\'', '"':
					p.flushText()
					p.pos++
					p.handleSmartQuote(next, true)
					p.textStart = p.pos
					continue
				}
			}
			p.flushText()
			if ba, end, ok := p.inlineAttrs(p.pos); ok {
				if !ba.empty() {
					p.items = append(p.items, inlineItem{kind: itemAttrs, attrs: ba, attrsStart: p.pos, attrsEnd: end, attrsDeep: len(p.openers) >= MaxNesting})
				}
				p.pos = end
			} else {
				p.addText(src[p.pos:p.pos+1], p.pos)
				p.pos++
			}
			p.textStart = p.pos
		case '[':
			p.flushText()
			if close := p.footnoteRefEnd(p.pos); close >= 0 {
				p.pushNode(at(&Node{Tag: FootnoteReference, Text: normalizeLabel(src[p.pos+2 : close])}, p.pos, close+1))
				p.pos = close + 1
				p.textStart = p.pos
				continue
			}
			// The bracket is an item of its own: the opener's index points
			// at it. Refused at MaxNesting it is a bracket with no opener.
			if p.mayOpen() {
				p.openers = append(p.openers, openerInfo{char: '[', itemIdx: len(p.items), srcPos: p.pos})
			}
			p.pushNode(strNode(src[p.pos:p.pos+1], p.pos))
			p.pos++
			p.textStart = p.pos
		case '!':
			// ![^a] is "!" and a footnote reference: djot.js decides an
			// image at the closing bracket, where the reference has
			// already matched.
			if p.pos+1 < len(src) && src[p.pos+1] == '[' && p.footnoteRefEnd(p.pos+1) < 0 {
				p.flushText()
				if p.mayOpen() {
					p.openers = append(p.openers, openerInfo{char: '!', itemIdx: len(p.items), srcPos: p.pos})
				}
				p.pushNode(strNode(src[p.pos:p.pos+2], p.pos))
				p.pos += 2
				p.textStart = p.pos
			} else {
				p.pos++
			}
		case ']':
			p.flushText()
			p.handleCloseBracket()
			p.textStart = p.pos
		case '(':
			if p.destination {
				p.parens = append(p.parens, p.pos)
			}
			p.pos++
		case ')':
			if p.destination && !p.popParen() {
				p.flushText()
				p.handleCloseParen()
				p.textStart = p.pos
			} else {
				p.pos++
			}
		case '<':
			p.flushText()
			if !p.handleAutolink() {
				p.addText(src[p.pos:p.pos+1], p.pos)
				p.pos++
			}
			p.textStart = p.pos
		case '$':
			p.flushText()
			p.handleMath()
			p.textStart = p.pos
		case ':':
			p.flushText()
			if !p.handleSymbol() {
				p.pos++
				p.textStart = p.pos - 1
				continue
			}
			p.textStart = p.pos
		case '"', '\'':
			p.flushText()
			p.handleSmartQuote(c, false)
			p.textStart = p.pos
		case '-', '+', '=':
			if p.pos+1 < len(src) && src[p.pos+1] == '}' {
				p.flushText()
				if !p.handleBracedClose(c) {
					p.addText(src[p.pos:p.pos+2], p.pos)
				}
				p.pos += 2
				p.textStart = p.pos
			} else if c == '-' && p.pos+1 < len(src) && src[p.pos+1] == '-' {
				p.flushText()
				dp := p.pos
				for dp < len(src) && src[dp] == '-' {
					dp++
				}
				count := dp - p.pos
				if dp < len(src) && src[dp] == '}' && count >= 2 {
					count--
					dp--
				}
				p.emitDashes(p.pos, count)
				p.pos = dp
				p.textStart = p.pos
			} else {
				p.pos++
			}
		case '.':
			if p.pos+2 < len(src) && src[p.pos+1] == '.' && src[p.pos+2] == '.' {
				p.flushText()
				p.pushNode(at(&Node{Tag: Ellipsis}, p.pos, p.pos+3))
				p.pos += 3
				p.textStart = p.pos
			} else {
				p.pos++
			}
		default:
			p.pos++
		}
	}
	p.flushText()
	if p.endsLine {
		p.trimLastText(len(src), " ")
	}
	return p.resolveItems()
}

func (p *inlineParser) handleBackslash() {
	src, pos := p.src, p.pos
	if pos+1 >= len(src) {
		// A backslash ending the block is a hard break on its newline, as
		// djot.js; ending a cell, which has none, it is text.
		if p.endsLine {
			p.hardBreak(pos, pos+1)
		} else {
			p.addText(src[pos:pos+1], pos)
		}
		p.pos++
		return
	}
	next := src[pos+1]
	switch {
	case next == '\n':
		p.hardBreak(pos, pos+1)
		p.pos += 2
	case next == ' ' || next == '\t':
		skip := pos + 1
		for skip < len(src) && (src[skip] == ' ' || src[skip] == '\t') {
			skip++
		}
		if skip < len(src) && src[skip] == '\n' || skip == len(src) && p.endsLine {
			// A hard break, at the end of src too when a line end follows
			// it; as djot.js's, it spans from the blank after the backslash
			// to the newline.
			p.hardBreak(pos, skip)
			p.pos = min(skip+1, len(src))
		} else if next == ' ' {
			p.pushNode(at(&Node{Tag: NonBreakingSpace}, pos+1, pos+2))
			p.pos += 2
		} else {
			p.addText(src[pos:pos+1], pos)
			p.pos++
		}
	case isEscapable(next):
		// The escaped byte alone: a text node of its own, which the text
		// after it joins and the text before it does not.
		p.pushNode(strNode(src[pos+1:pos+2], pos+1))
		p.pos += 2
	default:
		p.addText(src[pos:pos+1], pos)
		p.pos++
	}
}

func (p *inlineParser) handleBackticks() {
	src, pos := p.src, p.pos
	tickLen := countRunAt(src, pos, '`')
	after := pos + tickLen
	closePos, ok := findClosingTicks(src, after, tickLen)
	if !ok {
		rest := p.unclosedRest(after)
		p.pushNode(at(&Node{Tag: Verbatim, Text: trimVerbatimContent(rest)}, pos, after+len(rest)))
		p.pos = len(src)
		return
	}
	content := trimVerbatimContent(src[after:closePos])
	afterClose := closePos + tickLen
	p.pos = afterClose
	if fe, ok := rawFormatEnd(src, afterClose); ok {
		p.pushNode(at(&Node{Tag: RawInline, Text: content, Lang: src[afterClose+2 : fe]}, pos, fe+1))
		p.pos = fe + 1
		return
	}
	p.pushNode(at(&Node{Tag: Verbatim, Text: content}, pos, afterClose))
}

// rawFormatEnd is where a raw format opening at pos (`{=`) ends: its `}`,
// with one or more bytes between other than blanks (JavaScript's \s),
// braces and backticks, as djot.js's `\{=[^\s{}`]+\}`.
func rawFormatEnd(src string, pos int) (int, bool) {
	if pos+1 >= len(src) || src[pos] != '{' || src[pos+1] != '=' {
		return 0, false
	}
	for i := pos + 2; i < len(src); {
		r, n := utf8.DecodeRuneInString(src[i:])
		switch {
		case r == '}':
			return i, i > pos+2
		case r == '{' || r == '`' || isJSSpace(r):
			return 0, false
		}
		i += n
	}
	return 0, false
}

// handleMath reads math as a verbatim with a dollar prefix: closed, its
// content between the backtick runs; unclosed, to the leaf's end as an
// unclosed verbatim (djot.js). Either way one space next to a backtick at
// an end goes. A dollar without a backtick is text.
func (p *inlineParser) handleMath() {
	src, pos := p.src, p.pos
	dollars := 1
	if pos+1 < len(src) && src[pos+1] == '$' {
		dollars = 2
	}
	tickStart := pos + dollars
	if tickStart >= len(src) || src[tickStart] != '`' {
		p.addText(src[pos:pos+dollars], pos)
		p.pos += dollars
		return
	}
	tag := InlineMath
	if dollars == 2 {
		tag = DisplayMath
	}
	tickLen := countRunAt(src, tickStart, '`')
	contentStart := tickStart + tickLen
	if closePos, ok := findClosingTicks(src, contentStart, tickLen); ok {
		p.pushNode(at(&Node{Tag: tag, Text: trimVerbatimContent(src[contentStart:closePos])}, pos, closePos+tickLen))
		p.pos = closePos + tickLen
		return
	}
	rest := p.unclosedRest(contentStart)
	p.pushNode(at(&Node{Tag: tag, Text: trimVerbatimContent(rest)}, pos, contentStart+len(rest)))
	p.pos = len(src)
}

// hardBreak pushes the hard break the backslash at pos makes, spanning the
// joined text after the backslash through the newline at nl (one past the
// text when the line end follows it), after dropping the blanks of the text
// that ends at the backslash.
func (p *inlineParser) hardBreak(pos, nl int) {
	p.trimLastText(pos, " \t")
	p.pushNode(at(&Node{Tag: HardBreak}, pos+1, nl+1))
}

// trimLastText drops blanks from the end of the last text node when it is a
// slice of the source ending at end: before a hard break (spaces and tabs)
// or at the leaf's end (spaces only, a trailing tab being content there);
// before a soft break they stay. djot.js trims its last match only when
// that match is text, so the spaces before an empty attribute or a
// comment, which make no node, stay.
func (p *inlineParser) trimLastText(end int, blanks string) {
	n := len(p.items)
	if n == 0 || p.items[n-1].kind != itemNode {
		return
	}
	last := p.items[n-1].node
	if last.Tag != Str || last.off == 0 || last.off-1+len(last.Text) != end {
		return
	}
	last.Text = strings.TrimRight(last.Text, blanks)
	if last.Text == "" {
		p.items = p.items[:n-1]
	}
}

// unclosedRest is the text from from to the end, the span an unclosed
// verbatim or math takes: at a leaf's end its trailing spaces go, as text's
// would (djot.js trims the last match before the closing newline; a cell
// has none).
func (p *inlineParser) unclosedRest(from int) string {
	rest := p.src[from:]
	if p.endsLine {
		return strings.TrimRight(rest, " ")
	}
	return rest
}

func bracedTag(char byte) Tag {
	switch char {
	case '-':
		return Delete
	case '+':
		return Insert
	case '=':
		return Mark
	case '~':
		return Subscript
	case '^':
		return Superscript
	}
	return Span
}

// topOpener is the index of the nearest opener of char, marked or not, or
// -1.
func (p *inlineParser) topOpener(char byte, marked bool) int {
	for i := len(p.openers) - 1; i >= 0; i-- {
		if op := p.openers[i]; op.char == char && op.marked == marked {
			return i
		}
	}
	return -1
}

// closeMarked closes the marked opener {X with the X} at p.pos as a node of
// tag spanning both braces; false when no opener is there to close, its
// pair would be empty, or it stands before the link whose destination is
// open (outsideLink).
func (p *inlineParser) closeMarked(char byte, tag Tag) bool {
	i := p.topOpener(char, true)
	if i < 0 || p.openers[i].touches(p.pos) || p.outsideLink(i) {
		return false
	}
	p.closeAt(i, tag, p.pos+2)
	return true
}

func (p *inlineParser) handleBracedClose(char byte) bool {
	return p.closeMarked(char, bracedTag(char))
}

// matchedTag is the node a matched delimiter makes: djot.js's betweenMatched
// annotations.
func matchedTag(char byte) Tag {
	switch char {
	case '*':
		return Strong
	case '_':
		return Emph
	case '~':
		return Subscript
	}
	return Superscript
}

func (p *inlineParser) handleEmphDelimiter(char byte, marked bool) {
	src, pos := p.src, p.pos

	if !marked && pos+1 < len(src) && src[pos+1] == '}' {
		if !p.closeMarked(char, matchedTag(char)) {
			p.addText(src[pos:pos+2], pos)
		}
		p.pos = pos + 2
		return
	}

	if canCloseDelim(src, pos) && !marked {
		if i := p.topOpener(char, false); i >= 0 && !p.openers[i].touches(pos) {
			if p.outsideLink(i) {
				p.addText(src[pos:pos+1], pos)
			} else {
				p.closeAt(i, matchedTag(char), pos+1)
			}
			p.pos = pos + 1
			return
		}
	}

	if marked || canOpenDelim(src, pos) {
		start := openerStart(pos, marked)
		if p.mayOpen() {
			p.pushOpener(openerInfo{char: char, itemIdx: len(p.items), srcPos: start, marked: marked})
		} else {
			p.addText(src[start:pos+1], start)
		}
		p.pos = pos + 1
		return
	}

	p.addText(src[pos:pos+1], pos)
	p.pos = pos + 1
}

// footnoteRefEnd is where a footnote reference opening at pos ("[^") ends:
// its first "]", with a label of at least one byte on the line between, as
// djot.js's [^\]]+ over the line; -1 otherwise ("[^]" is text). A scan that
// meets the line's end or the input's first has shown every "[^" before
// that point to fail too, which is kept, so a line of them is linear.
func (p *inlineParser) footnoteRefEnd(pos int) int {
	src := p.src
	if pos+1 >= len(src) || src[pos+1] != '^' || pos < p.noFootnoteBefore {
		return -1
	}
	close := pos + 2
	for close < len(src) && src[close] != ']' && src[close] != '\n' {
		close++
	}
	if close >= len(src) || src[close] == '\n' {
		p.noFootnoteBefore = close
		return -1
	}
	if close == pos+2 {
		return -1
	}
	return close
}

// nearestBracket is the index of the nearest bracket opener of either kind,
// or -1.
func (p *inlineParser) nearestBracket() int {
	for i := len(p.openers) - 1; i >= 0; i-- {
		if c := p.openers[i].char; c == '[' || c == '!' {
			return i
		}
	}
	return -1
}

// outsideLink reports whether a closer is kept from the opener at i: inside
// an open destination, an opener before the link's bracket, which djot.js
// after 0.3.2 does not match there and its suite expects (*[a](b*) is "*"
// and a link). The closer is then text, and opens nothing.
func (p *inlineParser) outsideLink(i int) bool {
	if !p.destination {
		return false
	}
	b := p.nearestBracket()
	if b < 0 {
		return false
	}
	open := p.openers[b].link
	return open != nil && !open.reference && i < b
}

// popParen pairs a ")" with the last "(" read in the destination; false
// when none is unpaired.
func (p *inlineParser) popParen() bool {
	if len(p.parens) == 0 {
		return false
	}
	p.parens = p.parens[:len(p.parens)-1]
	return true
}

// closeLink closes the link or image the bracket opener i began, on the ")"
// or "]" ending at end: its text is the items before its "](" or "][", and
// those from it on go.
func (p *inlineParser) closeLink(i int, link *Node, end int) {
	op := p.openers[i]
	open := op.link
	p.items = p.items[:open.itemIdx]
	p.dropped = p.dropped[:open.droppedLen]
	link.Children = p.collectChildren(op.itemIdx)
	link.Tag = Link
	start := op.srcPos
	if op.char == '!' {
		// djot.js starts an image on its bracket, not its "!".
		link.Tag, start = Image, op.srcPos+1
	}
	p.pushNode(at(link, start, end))
	p.truncateOpeners(i)
}

func (p *inlineParser) handleCloseBracket() {
	src, pos := p.src, p.pos
	i := p.nearestBracket()
	if i < 0 {
		p.addText(src[pos:pos+1], pos)
		p.pos = pos + 1
		return
	}
	op := p.openers[i]
	next := pos + 1

	if open := op.link; open != nil && open.reference {
		// The label is stored keyed as djot.js keys it (escapes dropped,
		// then normalized), as its AST prints it; a label of blanks only
		// keeps its blanks, since "" would read as the text-as-label form
		// [a][] (zjot.test L9266: [a][ ] resolves nothing).
		ref := unescapeLabel(src[open.srcPos+2 : pos])
		if norm := normalizeLabel(ref); norm != "" {
			ref = norm
		}
		p.closeLink(i, &Node{Reference: ref}, pos+1)
		p.pos = pos + 1
		return
	}

	if next < len(src) && (src[next] == '(' || src[next] == '[') {
		// The link closes where the destination or label does, as the
		// pass comes to it; what follows until then is read as any text.
		// The openers inside the link's text are done with ([x_y](x_ has
		// no emphasis), parens among them.
		p.truncateOpeners(i + 1)
		p.dropParens(op.srcPos)
		p.openers[i].link = &linkOpen{reference: src[next] == '[', itemIdx: len(p.items), srcPos: pos, droppedLen: len(p.dropped)}
		p.pushNode(strNode(src[pos:pos+2], pos))
		if src[next] == '(' {
			p.destination = true
			p.parens = p.parens[:0]
		}
		p.pos = pos + 2
		return
	}

	if next < len(src) && src[next] == '{' {
		// A span, with the attributes when they parse; else the brace is
		// text (djot.js). An image's "!" is text before it.
		children := p.collectChildren(op.itemIdx)
		start := op.srcPos
		if op.char == '!' {
			p.pushNode(strNode(src[op.srcPos:op.srcPos+1], op.srcPos))
			start++
		}
		// The span ends at its "]"; the attributes are not part of it.
		span := at(&Node{Tag: Span, Children: children}, start, pos+1)
		if ba, end, ok := p.inlineAttrs(next); ok {
			applyBlockAttrs(span, ba)
			p.pos = end
		} else {
			p.pos = next
		}
		p.pushNode(span)
		p.truncateOpeners(i)
		return
	}

	// A bare "]" is text, and the openers inside stay ([_a]_ has
	// emphasis), as djot.js.
	p.addText(src[pos:pos+1], pos)
	p.pos = pos + 1
}

// handleCloseParen handles a ")" in a destination with no "(" of its own to
// pair: it closes the link of the nearest bracket when that bracket is an
// explicit link, as djot.js, and is text otherwise.
func (p *inlineParser) handleCloseParen() {
	pos := p.pos
	if i := p.nearestBracket(); i >= 0 {
		if open := p.openers[i].link; open != nil && !open.reference {
			dest := processUrl(p.src[open.srcPos+2 : pos])
			p.closeLink(i, &Node{Destination: &dest}, pos+1)
			p.destination = false
			p.pos = pos + 1
			return
		}
	}
	p.addText(p.src[pos:pos+1], pos)
	p.pos = pos + 1
}

func (p *inlineParser) handleAutolink() bool {
	close, email, ok := autolinkAt(p.src, p.pos)
	if !ok {
		return false
	}
	tag := URL
	if email {
		tag = Email
	}
	p.pushNode(at(&Node{Tag: tag, Text: p.src[p.pos+1 : close]}, p.pos, close+1))
	p.pos = close + 1
	return true
}

// autolinkAt reads an autolink opening at pos, djot.js's <([^<>\s]+)>: the
// index of its closing ">" and whether it is an email (an "@" follows a
// byte other than ":"), else a URL (a letter is followed by ":"); ok is
// false for anything else. The scan stops at the first blank or "<", so a
// run of "<" without ">" is linear.
func autolinkAt(src string, pos int) (close int, email, ok bool) {
	end := pos + 1
	for ; end < len(src) && src[end] != '>'; end++ {
		if src[end] == '<' {
			return 0, false, false
		}
		if r, _ := utf8.DecodeRuneInString(src[end:]); isJSSpace(r) {
			return 0, false, false
		}
	}
	if end >= len(src) || end == pos+1 {
		return 0, false, false
	}
	content := src[pos+1 : end]
	for i := 1; i < len(content); i++ {
		if content[i] == '@' && content[i-1] != ':' {
			return end, true, true
		}
	}
	for i := 1; i < len(content); i++ {
		if content[i] == ':' && isLetter(content[i-1]) {
			return end, false, true
		}
	}
	return 0, false, false
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

func (p *inlineParser) handleSymbol() bool {
	src, pos := p.src, p.pos
	start := pos + 1
	if start >= len(src) {
		return false
	}
	end := strings.IndexByte(src[start:], ':')
	if end <= 0 {
		return false
	}
	end += start
	name := src[start:end]
	for i := 0; i < len(name); i++ {
		if c := name[i]; !isAlphanumeric(c) && c != '-' && c != '_' && c != '+' {
			return false
		}
	}
	p.pushNode(at(&Node{Tag: Symb, Text: name}, pos, end+1))
	p.pos = end + 1
	return true
}

// emitDashes turns a run of count hyphens starting at start into em dashes
// (3) and en dashes (2): all em if divisible by 3, else all en if divisible
// by 2, else em first.
func (p *inlineParser) emitDashes(start, count int) {
	emit := func(n *Node, width int) {
		p.pushNode(at(n, start, start+width))
		start += width
	}
	if count <= 1 {
		if count == 1 {
			p.addText(p.src[start:start+1], start)
		}
		return
	}
	allEm := count%3 == 0
	allEn := count%2 == 0
	for remaining := count; remaining > 0; {
		switch {
		case allEm:
			emit(&Node{Tag: EmDash}, 3)
			remaining -= 3
		case allEn:
			emit(&Node{Tag: EnDash}, 2)
			remaining -= 2
		case remaining >= 3 && (remaining%2 != 0 || remaining > 4):
			emit(&Node{Tag: EmDash}, 3)
			remaining -= 3
		case remaining >= 2:
			emit(&Node{Tag: EnDash}, 2)
			remaining -= 2
		default:
			p.addText(p.src[start:start+1], start)
			start++
			remaining--
		}
	}
}

func canOpenSingleQuote(src string, pos int) bool {
	if pos == 0 {
		return true
	}
	switch src[pos-1] {
	case ' ', '\t', '\n', '\r', '"', '\'', '-', '(', '[':
		return true
	}
	return false
}

func quoteTag(char byte) Tag {
	if char == '"' {
		return DoubleQuoted
	}
	return SingleQuoted
}

func (p *inlineParser) handleSmartQuote(char byte, marked bool) {
	src, pos := p.src, p.pos

	if !marked && pos+1 < len(src) && src[pos+1] == '}' {
		if p.closeMarked(char, quoteTag(char)) {
			p.pos = pos + 2
			return
		}
		if char == '\'' {
			p.pushNode(at(quoteLit("\u2019", "'"), pos, pos+2))
		} else {
			p.pushNode(at(quoteLit("\u201d", "\""), pos, pos+2))
		}
		p.pos = pos + 2
		return
	}

	canClose := !marked && pos > 0 && !isInlineSpace(src[pos-1])
	canOpen := marked || (pos+1 < len(src) && !isInlineSpace(src[pos+1]) &&
		(char != '\'' || canOpenSingleQuote(src, pos)))

	if canClose {
		if i := p.topOpener(char, false); i >= 0 && !p.openers[i].touches(pos) {
			if !p.outsideLink(i) {
				p.closeAt(i, quoteTag(char), pos+1)
			} else if char == '\'' {
				p.pushNode(at(quoteLit("\u2019", "'"), pos, pos+1))
			} else {
				p.pushNode(at(quoteLit("\u201c", "\""), pos, pos+1))
			}
			p.pos = pos + 1
			return
		}
	}

	if canOpen && !p.mayOpen() {
		// Refused at MaxNesting: the quote is a literal, and a marked
		// one's brace text before it.
		if marked {
			p.addText(src[pos-1:pos], pos-1)
		}
		canOpen = false
	}
	if canOpen {
		p.pushOpener(openerInfo{char: char, itemIdx: len(p.items), srcPos: openerStart(pos, marked), marked: marked})
	} else if char == '\'' {
		// Neither opens nor closes: an apostrophe.
		p.pushNode(at(quoteLit("\u2019", "'"), pos, pos+1))
	} else {
		p.pushNode(at(quoteLit("\u201c", "\""), pos, pos+1))
	}
	p.pos = pos + 1
}

// collectChildren resolves the items after openerIdx into nodes and drops
// them, along with the opener, from the item list.
func (p *inlineParser) collectChildren(openerIdx int) []*Node {
	children := p.resolve(p.items[openerIdx+1:])
	p.items = p.items[:openerIdx]
	return children
}

// resolve turns items into nodes. Text joins the text before it; an opener
// that never closed is text too, and a marked one starts a text node of its
// own, as djot.js's open marker breaks the run of text before it (the text
// after it joins it).
func (p *inlineParser) resolve(items []inlineItem) []*Node {
	var nodes []*Node
	for _, it := range items {
		switch it.kind {
		case itemNode:
			nodes = p.appendText(nodes, it.node)
		case itemOpener:
			if it.opener.marked {
				nodes = append(nodes, p.openerText(it.opener))
			} else {
				nodes = p.appendText(nodes, p.openerText(it.opener))
			}
		case itemAttrs:
			if it.attrsDeep && spanWord(nodes) >= 0 {
				// The span would nest past MaxNesting: the attribute is text.
				p.tooDeep = true
				nodes = p.appendText(nodes, strNode(p.src[it.attrsStart:it.attrsEnd], it.attrsStart))
				continue
			}
			nodes = p.attach(nodes, it.attrs)
		}
	}
	return nodes
}

// openerStart is where an opener whose delimiter is at pos starts: a braced
// opener starts at its "{".
func openerStart(pos int, marked bool) int {
	if marked {
		return pos - 1
	}
	return pos
}

// openerLen is the number of source bytes an opener item spans: a marked
// opener its brace and delimiter, else the delimiter. (A bracket's opener
// is not an item; its text is the node it points at.)
func openerLen(op openerInfo) int {
	if op.marked {
		return 2
	}
	return 1
}

// openerText is the text node an unmatched opener leaves behind: its source
// bytes, or for a smart quote the glyph it renders as, with the quote it was
// written as.
func (p *inlineParser) openerText(op openerInfo) *Node {
	switch op.char {
	case '\'':
		glyph := "\u2019"
		if op.marked {
			glyph = "\u2018"
		}
		return at(quoteLit(glyph, "'"), op.srcPos, op.srcPos+openerLen(op))
	case '"':
		return at(quoteLit("\u201c", "\""), op.srcPos, op.srcPos+openerLen(op))
	}
	return strNode(p.src[op.srcPos:op.srcPos+openerLen(op)], op.srcPos)
}

func (p *inlineParser) resolveItems() []*Node {
	return p.resolve(p.items)
}

// isSmartQuote reports whether n is the glyph a smart quote that paired
// with nothing left: text whose source is the quote it was written as.
func isSmartQuote(n *Node) bool { return n.Tag == Str && n.Source != "" }

// spanWord is where the word an inline attribute after nodes wraps in a span
// starts in the last node's text, or -1 when it makes no span: the last
// node is no text, a smart quote, or text ending in a blank.
func spanWord(nodes []*Node) int {
	if len(nodes) == 0 {
		return -1
	}
	last := nodes[len(nodes)-1]
	if last.Tag != Str || isSmartQuote(last) {
		return -1
	}
	wordStart := 0
	if i := strings.LastIndexFunc(last.Text, isJSSpace); i >= 0 {
		_, n := utf8.DecodeRuneInString(last.Text[i:])
		wordStart = i + n
	}
	if wordStart == len(last.Text) {
		return -1
	}
	return wordStart
}

// attachAttrs attaches ba to the node before it: a text node's last word
// becomes a span (the word after the last blank of any kind JavaScript's \s
// counts, djot.js's `[^\s]+$`; no text across another node is gathered); a
// container, verbatim, math, link, symbol, footnote reference or smart
// quote takes the attributes itself; a break, dash, ellipsis or
// non-breaking space takes none. It also reports the id the attributes
// registered and left on no node: theirs when they attached to nothing, or
// the id they replaced on the node they attached to.
func attachAttrs(nodes []*Node, ba blockAttrs) ([]*Node, string) {
	if len(nodes) == 0 {
		return nodes, ba.id
	}
	last := nodes[len(nodes)-1]
	if last.Tag == Str && !isSmartQuote(last) {
		text := last.Text
		wordStart := spanWord(nodes)
		if wordStart < 0 {
			return nodes, ba.id
		}
		word := tail(last, wordStart)
		ws := textSpan(word)
		span := at(&Node{Tag: Span, Children: []*Node{word}}, ws.start, ws.end)
		applyBlockAttrs(span, ba)
		if wordStart == 0 {
			nodes[len(nodes)-1] = span
		} else {
			last.Text = text[:wordStart]
			nodes = append(nodes, span)
		}
		return nodes, ""
	}
	switch last.Tag {
	case Str, Span, Link, Image, Emph, Strong, Verbatim, Mark,
		Superscript, Subscript, Insert, Delete, DoubleQuoted, SingleQuoted,
		URL, Email, InlineMath, DisplayMath, FootnoteReference, Symb:
		replaced := ""
		if ba.writesID() {
			replaced = last.ID
		}
		applyLaterAttrs(last, ba)
		return nodes, replaced
	}
	return nodes, ba.id
}

// applyBlockAttrs applies attributes to a node made for them: a block
// attribute to the block after it, an inline attribute to the span it makes.
// A node with attributes of its own (a div's class) keeps them after the
// block attribute's, classes joined, as djot.js: `{.x}` before `::: y` gives
// `class="x y"`.
func applyBlockAttrs(n *Node, ba blockAttrs) {
	if len(n.Attrs) > 0 && len(ba.attrs) > 0 {
		n.Attrs = ba.merge(attrsOf(n.Attrs)).attrs
	} else if len(ba.attrs) > 0 {
		n.Attrs = ba.attrs
	}
	if ba.id != "" {
		n.ID = ba.id
	}
}

// applyLaterAttrs applies a later inline attribute to a node that has one:
// the first keeps its places, the later's values win and its classes join
// after, as djot.js's attribute object (`[x]{#i .c}{.d}` gives `id="i"
// class="c d"`; an `id=""` after `#y` leaves no id).
func applyLaterAttrs(n *Node, ba blockAttrs) {
	merged := blockAttrs{id: n.ID, attrs: n.Attrs}.merge(ba)
	n.ID, n.Attrs = merged.id, merged.attrs
}

func isInlineSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func canOpenDelim(src string, pos int) bool {
	return pos+1 < len(src) && !isInlineSpace(src[pos+1])
}

func canCloseDelim(src string, pos int) bool {
	return pos > 0 && !isInlineSpace(src[pos-1])
}

// isEscapable reports whether a backslash before c escapes it: any ASCII
// punctuation (djot.js's pattPunctuation).
func isEscapable(c byte) bool {
	return strings.IndexByte("\\`*_{}[]()#+-.!|\"'~^:<>$%=&,/;?@", c) >= 0
}

// escapedLineEnd is where an escaped line end starting at i (the byte after
// the backslash) ends: the newline after any spaces and tabs, as djot.js's
// `[ \t]*\n`; -1 when the backslash escapes no line end.
func escapedLineEnd(raw string, i int) int {
	j := i
	for j < len(raw) && (raw[j] == ' ' || raw[j] == '\t') {
		j++
	}
	if j < len(raw) && raw[j] == '\n' {
		return j
	}
	return -1
}

// backslashGoes reports whether the backslash at i goes in a label or a
// destination: before punctuation, a space (an escaped space is a space
// there, djot.js) or a line end.
func backslashGoes(raw string, i int) bool {
	if i+1 >= len(raw) {
		return false
	}
	return isEscapable(raw[i+1]) || raw[i+1] == ' ' || escapedLineEnd(raw, i+1) >= 0
}

// unescapeLabel is a reference label as djot.js keys it, before
// normalizeLabel: a backslash goes before punctuation, a space or a line end
// (backslashGoes).
func unescapeLabel(raw string) string {
	if !strings.Contains(raw, "\\") {
		return raw
	}
	buf := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\\' && backslashGoes(raw, i) {
			i++
		}
		buf = append(buf, raw[i])
	}
	return string(buf)
}

func countRunAt(src string, pos int, char byte) int {
	i := pos
	for i < len(src) && src[i] == char {
		i++
	}
	return i - pos
}

func findClosingTicks(src string, start, count int) (int, bool) {
	for i := start; i < len(src); {
		if src[i] != '`' {
			i++
			continue
		}
		run := countRunAt(src, i, '`')
		if run == count {
			return i, true
		}
		i += run
	}
	return 0, false
}

func trimVerbatimContent(raw string) string {
	start, end := 0, len(raw)
	if len(raw) >= 2 && raw[0] == ' ' && raw[1] == '`' {
		start = 1
	}
	if end > start+1 && raw[end-1] == ' ' && raw[end-2] == '`' {
		end--
	}
	return raw[start:end]
}

// processUrl is a destination as djot.js builds it: a line end joins the
// lines without the next line's indent, an escaped line end drops the blanks
// before it too, and a backslash before punctuation or a space goes.
func processUrl(raw string) string {
	if !strings.ContainsAny(raw, "\n\\") {
		return raw
	}
	buf := make([]byte, 0, len(raw))
	// kept is the end of the last escaped byte: the trim before an escaped
	// line end stops there, so that an escaped space stays.
	kept := 0
	for i := 0; i < len(raw); {
		switch {
		case raw[i] == '\n':
			i++
			for i < len(raw) && (raw[i] == ' ' || raw[i] == '\t') {
				i++
			}
		case raw[i] == '\\' && escapedLineEnd(raw, i+1) >= 0:
			// An escaped line end takes the blanks before it, as djot.js
			// trims the text before a hard break; the blanks after the
			// backslash stay, and the newline takes the line-end branch.
			for len(buf) > kept && (buf[len(buf)-1] == ' ' || buf[len(buf)-1] == '\t') {
				buf = buf[:len(buf)-1]
			}
			i++
		case raw[i] == '\\' && backslashGoes(raw, i):
			buf = append(buf, raw[i+1])
			i += 2
			kept = len(buf)
		default:
			buf = append(buf, raw[i])
			i++
		}
	}
	return string(buf)
}

// isJSSpace is JavaScript's \s: the ASCII blanks, U+00A0, U+1680,
// U+2000-U+200A, U+2028, U+2029, U+202F, U+205F, U+3000 and U+FEFF. Not
// unicode.IsSpace, which adds U+0085 and lacks U+FEFF.
func isJSSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0x00a0, 0x1680,
		0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}

// trimJS is JavaScript's String.prototype.trim.
func trimJS(s string) string {
	return strings.TrimFunc(s, isJSSpace)
}

// normalizeLabel is djot.js's normalizeLabel: the label trimmed as
// JavaScript trims, then every run of ' ', '\t', '\r' and '\n' one space.
func normalizeLabel(raw string) string {
	raw = trimJS(raw)
	buf := make([]byte, 0, len(raw))
	inWS := false
	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case ' ', '\t', '\r', '\n':
			if !inWS {
				buf = append(buf, ' ')
				inWS = true
			}
			continue
		}
		buf = append(buf, raw[i])
		inWS = false
	}
	return string(buf)
}

// stringContent is djot.js's getStringContent over nodes: the text a heading
// id, a reference label and image alt text read. A smart quote, a dash and
// an ellipsis read as their source; breaks as a newline; a non-breaking
// space and a symbol as nothing. A footnote reference's label is read only
// when footnoteLabels is set: zjot leaves it out of a heading's text
// (zjot.test difference 2).
func stringContent(nodes []*Node, footnoteLabels bool) string {
	var b strings.Builder
	var walk func([]*Node)
	walk = func(nodes []*Node) {
		for _, n := range nodes {
			switch n.Tag {
			case Str:
				if n.Source != "" {
					b.WriteString(n.Source)
				} else {
					b.WriteString(n.Text)
				}
			case Verbatim, InlineMath, DisplayMath, RawInline, URL, Email:
				b.WriteString(n.Text)
			case SoftBreak, HardBreak:
				b.WriteByte('\n')
			case Ellipsis:
				b.WriteString("...")
			case EmDash:
				b.WriteString("---")
			case EnDash:
				b.WriteString("--")
			case FootnoteReference:
				if footnoteLabels {
					b.WriteString(n.Text)
				}
			case NonBreakingSpace, Symb:
			default:
				walk(n.Children)
			}
		}
	}
	walk(nodes)
	return b.String()
}

// mergeRefAttrs layers a reference definition's attributes under the link's
// own, as djot.js: the definition's that the link does not set come first,
// then the link's own in their order; nothing joins.
func mergeRefAttrs(n *Node, ref blockAttrs) {
	if n.ID == "" {
		n.ID = ref.id
	}
	if len(ref.attrs) == 0 {
		return
	}
	if len(n.Attrs) == 0 {
		n.Attrs = ref.attrs
		return
	}
	var merged []Attr
	for _, ra := range ref.attrs {
		if !hasKey(n.Attrs, ra.Key) {
			merged = append(merged, ra)
		}
	}
	n.Attrs = append(merged, n.Attrs...)
}

// textSpan is the extent of a text node in the inline source: of the slice
// it is, or of the literal it stands for.
func textSpan(n *Node) srcSpan {
	if n.off > 0 {
		return srcSpan{n.off - 1, n.off - 1 + len(n.Text)}
	}
	return n.src
}

// inlineAttrs parses an inline attribute at pos, unless pos is before
// attrsFrom.
func (p *inlineParser) inlineAttrs(pos int) (blockAttrs, int, bool) {
	if pos < p.attrsFrom {
		return blockAttrs{}, 0, false
	}
	return parseInlineAttrs(p.src, pos)
}

// attach is attachAttrs, keeping the id it leaves on no node.
func (p *inlineParser) attach(nodes []*Node, ba blockAttrs) []*Node {
	nodes, dropped := attachAttrs(nodes, ba)
	if dropped != "" {
		p.dropped = append(p.dropped, dropped)
	}
	return nodes
}
