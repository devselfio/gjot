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
}

func parseInlines(lines []string) []*Node {
	if len(lines) == 0 {
		return nil
	}
	return parseInlineContent(strings.Join(lines, "\n"))
}

func parseInlineContent(src string) []*Node {
	nodes, _ := parseLeafInlines(src, 0, true)
	return nodes
}

// parseLeafInlines is parseInlineContent for the container stack: no inline
// attribute is read at an offset before attrsFrom, endsLine says whether a
// line end follows src, and the ids of inline attributes that attached to
// no node (or that a later id replaced) are returned rather than lost.
func parseLeafInlines(src string, attrsFrom int, endsLine bool) (nodes []*Node, droppedIDs []string) {
	p := &inlineParser{src: src, attrsFrom: attrsFrom, endsLine: endsLine}
	nodes = p.parse()
	return nodes, p.dropped
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
	p.openers = p.openers[:i]
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
					p.pushOpener(openerInfo{char: next, itemIdx: len(p.items), srcPos: p.pos, marked: true})
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
					p.items = append(p.items, inlineItem{kind: itemAttrs, attrs: ba})
				}
				p.pos = end
			} else {
				p.addText(src[p.pos:p.pos+1], p.pos)
				p.pos++
			}
			p.textStart = p.pos
		case '[':
			p.flushText()
			if p.pos+1 < len(src) && src[p.pos+1] == '^' {
				if close, ok := findMatchingBracket(src, p.pos); ok {
					p.pushNode(at(&Node{Tag: FootnoteReference, Text: src[p.pos+2 : close]}, p.pos, close+1))
					p.pos = close + 1
					p.textStart = p.pos
					continue
				}
			}
			// The bracket is an item of its own: the opener's index points at it.
			p.openers = append(p.openers, openerInfo{char: '[', itemIdx: len(p.items), srcPos: p.pos})
			p.pushNode(strNode(src[p.pos:p.pos+1], p.pos))
			p.pos++
			p.textStart = p.pos
		case '!':
			if p.pos+1 < len(src) && src[p.pos+1] == '[' {
				p.flushText()
				p.openers = append(p.openers, openerInfo{char: '!', itemIdx: len(p.items), srcPos: p.pos})
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
	if afterClose+1 < len(src) && src[afterClose] == '{' && src[afterClose+1] == '=' {
		if fe := strings.IndexByte(src[afterClose:], '}'); fe >= 0 {
			fe += afterClose
			if format := src[afterClose+2 : fe]; format != "" && !strings.Contains(format, " ") {
				p.pushNode(at(&Node{Tag: RawInline, Text: content, Lang: format}, pos, fe+1))
				p.pos = fe + 1
				return
			}
		}
	}
	p.pushNode(at(&Node{Tag: Verbatim, Text: content}, pos, afterClose))
}

func (p *inlineParser) handleMath() {
	src, pos := p.src, p.pos
	dollars := 1
	if pos+1 < len(src) && src[pos+1] == '$' {
		dollars = 2
	}
	tickStart := pos + dollars
	if tickStart < len(src) && src[tickStart] == '`' {
		tickLen := countRunAt(src, tickStart, '`')
		contentStart := tickStart + tickLen
		if closePos, ok := findClosingTicks(src, contentStart, tickLen); ok {
			tag := InlineMath
			if dollars == 2 {
				tag = DisplayMath
			}
			p.pushNode(at(&Node{Tag: tag, Text: src[contentStart:closePos]}, pos, closePos+tickLen))
			p.pos = closePos + tickLen
			return
		}
	}
	p.addText(src[pos:pos+dollars], pos)
	p.pos += dollars
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
// verbatim takes: at a leaf's end its trailing spaces go, as text's would
// (djot.js trims the last match before the closing newline; a cell has
// none).
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
// tag spanning both braces; false when no opener is there to close or its
// pair would be empty.
func (p *inlineParser) closeMarked(char byte, tag Tag) bool {
	i := p.topOpener(char, true)
	if i < 0 || p.openers[i].touches(p.pos) {
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
			p.closeAt(i, matchedTag(char), pos+1)
			p.pos = pos + 1
			return
		}
	}

	if marked || canOpenDelim(src, pos) {
		p.pushOpener(openerInfo{char: char, itemIdx: len(p.items), srcPos: openerStart(pos, marked), marked: marked})
		p.pos = pos + 1
		return
	}

	p.addText(src[pos:pos+1], pos)
	p.pos = pos + 1
}

func (p *inlineParser) handleCloseBracket() {
	src, pos := p.src, p.pos
	for i := len(p.openers) - 1; i >= 0; i-- {
		op := p.openers[i]
		if op.char != '[' && op.char != '!' {
			continue
		}
		children := p.collectChildren(op.itemIdx)
		tag, bracket, start := Link, "[", op.srcPos
		if op.char == '!' {
			// zjot starts an image at its "[", not the "!".
			tag, bracket, start = Image, "![", op.srcPos+1
		}
		p.pos = pos + 1

		if p.pos < len(src) {
			switch src[p.pos] {
			case '(':
				if cp, ok := findMatchingParen(src, p.pos); ok {
					dest := processUrl(src[p.pos+1 : cp])
					p.pos = cp + 1
					p.pushNode(at(&Node{Tag: tag, Children: children, Destination: &dest}, start, p.pos))
					p.openers = p.openers[:i]
					return
				}
			case '[':
				if cr, ok := findMatchingBracket(src, p.pos); ok {
					// The label is stored normalized, as djot.js's AST
					// prints it; a label of blanks only keeps its blanks,
					// since "" would read as the text-as-label form [a][]
					// (zjot.test L9266: [a][ ] resolves nothing).
					ref := src[p.pos+1 : cr]
					if norm := normalizeLabel(ref); norm != "" {
						ref = norm
					}
					p.pushNode(at(&Node{Tag: tag, Children: children, Reference: ref}, start, cr+1))
					p.pos = cr + 1
					p.openers = p.openers[:i]
					return
				}
			case '{':
				if ba, end, ok := p.inlineAttrs(p.pos); ok {
					// The span ends at its "]"; the attributes are not part of it.
					span := at(&Node{Tag: Span, Children: children}, op.srcPos, pos+1)
					applyBlockAttrs(span, ba)
					p.pushNode(span)
					p.pos = end
					p.openers = p.openers[:i]
					return
				}
			}
		}

		// Not a link after all: put the bracketed text back as text.
		p.pushNode(strNode(bracket, op.srcPos))
		for _, c := range children {
			p.pushNode(c)
		}
		p.pushNode(strNode(src[pos:pos+1], pos))
		p.openers = p.openers[:i]
		return
	}
	p.addText(src[pos:pos+1], pos)
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
			p.closeAt(i, quoteTag(char), pos+1)
			p.pos = pos + 1
			return
		}
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

func isWordSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' }

// attachAttrs attaches ba to the node before it: a container node takes the
// attributes directly; trailing text has its last word wrapped in a span,
// gathering back across adjacent text nodes to the word's start. It also
// reports the id the attributes registered and left on no node: theirs when
// they attached to nothing, or the id they replaced on the node they
// attached to.
func attachAttrs(nodes []*Node, ba blockAttrs) ([]*Node, string) {
	if len(nodes) == 0 {
		return nodes, ba.id
	}
	last := nodes[len(nodes)-1]
	switch last.Tag {
	case Str:
		text := last.Text
		if text == "" {
			return nodes, ba.id
		}
		wordStart := len(text)
		for wordStart > 0 && !isWordSpace(text[wordStart-1]) {
			wordStart--
		}
		if wordStart == len(text) {
			return nodes, ba.id
		}

		gatherStart := len(nodes) - 1
		if wordStart == 0 {
			for gatherStart > 0 {
				prev := nodes[gatherStart-1]
				if prev.Tag != Str || prev.Text == "" || isWordSpace(prev.Text[len(prev.Text)-1]) {
					break
				}
				gatherStart--
				ws := len(prev.Text)
				for ws > 0 && !isWordSpace(prev.Text[ws-1]) {
					ws--
				}
				if ws > 0 {
					wordStart = ws
					break
				}
			}
		}

		var spanChildren []*Node
		if gatherStart < len(nodes)-1 || wordStart > 0 {
			if wordStart > 0 {
				spanChildren = append(spanChildren, tail(nodes[gatherStart], wordStart))
				spanChildren = append(spanChildren, nodes[gatherStart+1:]...)
			} else {
				spanChildren = append(spanChildren, nodes[gatherStart:]...)
			}
		} else {
			spanChildren = append(spanChildren, tail(last, wordStart))
		}

		s := childrenSpan(spanChildren)
		span := at(&Node{Tag: Span, Children: spanChildren}, s.start, s.end)
		applyBlockAttrs(span, ba)

		switch {
		case wordStart > 0 && gatherStart < len(nodes)-1:
			first := nodes[gatherStart]
			first.Text = first.Text[:wordStart]
			return append(nodes[:gatherStart+1], span), ""
		case wordStart > 0:
			last.Text = text[:wordStart]
			return append(nodes, span), ""
		default:
			return append(nodes[:gatherStart], span), ""
		}
	case Span, Link, Image, Emph, Strong, Verbatim, Mark,
		Superscript, Subscript, Insert, Delete, DoubleQuoted, SingleQuoted:
		replaced := ""
		if ba.id != "" {
			replaced = last.ID
		}
		applyBlockAttrs(last, ba)
		return nodes, replaced
	}
	return nodes, ba.id
}

// applyBlockAttrs layers ba over the node's own attributes, so an attribute
// block before an ordered list keeps the list's start and type.
func applyBlockAttrs(n *Node, ba blockAttrs) {
	if ba.id != "" {
		n.ID = ba.id
	}
	if ba.classes != "" {
		n.Classes = ba.classes
	}
	if len(ba.attrs) > 0 {
		if len(n.Attrs) == 0 {
			n.Attrs = ba.attrs
		} else {
			n.Attrs = blockAttrs{attrs: n.Attrs}.merge(blockAttrs{attrs: ba.attrs}).attrs
		}
	}
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

func isEscapable(c byte) bool {
	return strings.IndexByte("\\`*_{}[]()#+-.!|\"'~^:<>$%=", c) >= 0
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

func findMatching(src string, pos int, open, close byte) (int, bool) {
	if pos >= len(src) || src[pos] != open {
		return 0, false
	}
	depth := 1
	for i := pos + 1; i < len(src); i++ {
		switch src[i] {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

func findMatchingParen(src string, pos int) (int, bool)   { return findMatching(src, pos, '(', ')') }
func findMatchingBracket(src string, pos int) (int, bool) { return findMatching(src, pos, '[', ']') }

// processUrl joins a destination split over lines and resolves escapes.
func processUrl(raw string) string {
	if !strings.ContainsAny(raw, "\n\\") {
		return raw
	}
	buf := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); {
		switch {
		case raw[i] == '\n':
			i++
			for i < len(raw) && (raw[i] == ' ' || raw[i] == '\t') {
				i++
			}
		case raw[i] == '\\' && i+1 < len(raw) && isEscapable(raw[i+1]):
			buf = append(buf, raw[i+1])
			i += 2
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
// own: the link's id and keys win, classes concatenate with the definition's first.
func mergeRefAttrs(n *Node, ref blockAttrs) {
	if n.ID == "" {
		n.ID = ref.id
	}
	if ref.classes != "" {
		if n.Classes != "" {
			n.Classes = ref.classes + " " + n.Classes
		} else {
			n.Classes = ref.classes
		}
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

// childrenSpan is the extent from the first child's start to the last
// child's end in the inline source, skipping children without one.
func childrenSpan(children []*Node) srcSpan {
	var span srcSpan
	for _, c := range children {
		s := c.src
		if c.Tag == Str && c.off > 0 {
			s = srcSpan{c.off - 1, c.off - 1 + len(c.Text)}
		}
		if s.end == 0 {
			continue
		}
		if span.end == 0 {
			span.start = s.start
		}
		span.end = s.end
	}
	return span
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
