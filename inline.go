package gjot

import "strings"

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

type inlineItem struct {
	kind   itemKind
	node   *Node
	opener openerInfo
	attrs  blockAttrs
}

type inlineParser struct {
	src       string
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
	p := &inlineParser{src: src}
	return p.parse()
}

// strNode is a text node sliced from the inline source at off.
func strNode(text string, off int) *Node {
	return &Node{Tag: Str, Text: text, off: off + 1}
}

// lit is a text node that is not a slice of the inline source.
func lit(text string) *Node {
	return &Node{Tag: Str, Text: text}
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
// when both are adjacent slices of the source.
func (p *inlineParser) addText(text string, off int) {
	if n := len(p.items); n > 0 && p.items[n-1].kind == itemNode {
		last := p.items[n-1].node
		if last.Tag == Str && last.off > 0 && last.off-1+len(last.Text) == off {
			last.Text = p.src[last.off-1 : off+len(text)]
			return
		}
	}
	p.pushNode(strNode(text, off))
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
		case '*', '_':
			p.flushText()
			p.handleEmphDelimiter(c, false)
			p.textStart = p.pos
		case '{':
			if p.pos+1 < len(src) {
				switch next := src[p.pos+1]; next {
				case '_', '*':
					p.flushText()
					p.pos++
					p.handleEmphDelimiter(next, true)
					p.textStart = p.pos
					continue
				case '+', '-', '=', '~', '^':
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
			if ba, end, ok := parseInlineAttrs(src, p.pos); ok {
				if !ba.empty() {
					p.items = append(p.items, inlineItem{kind: itemAttrs, attrs: ba})
				}
				p.pos = end
			} else {
				p.pushNode(at(lit("{"), p.pos, p.pos+1))
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
			p.openers = append(p.openers, openerInfo{char: '[', itemIdx: len(p.items), srcPos: p.pos})
			p.pushNode(at(lit("["), p.pos, p.pos+1))
			p.pos++
			p.textStart = p.pos
		case '!':
			if p.pos+1 < len(src) && src[p.pos+1] == '[' {
				p.flushText()
				p.openers = append(p.openers, openerInfo{char: '!', itemIdx: len(p.items), srcPos: p.pos})
				p.pushNode(at(lit("!["), p.pos, p.pos+2))
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
				p.pushNode(at(lit("<"), p.pos, p.pos+1))
				p.pos++
			}
			p.textStart = p.pos
		case '^', '~':
			p.flushText()
			if p.pos+1 < len(src) && src[p.pos+1] == '}' && p.handleBracedClose(c) {
				p.pos += 2
				p.textStart = p.pos
				continue
			}
			tag := Superscript
			if c == '~' {
				tag = Subscript
			}
			p.handleSuperSubscript(c, tag)
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
	p.trimTrailingWhitespace()
	return p.resolveItems()
}

func (p *inlineParser) handleBackslash() {
	src, pos := p.src, p.pos
	if pos+1 >= len(src) {
		p.pushNode(at(lit("\\"), pos, pos+1))
		p.pos++
		return
	}
	next := src[pos+1]
	switch {
	case next == '\n':
		p.stripTrailingSpacesFromLastStr()
		p.pushNode(at(&Node{Tag: HardBreak}, pos+1, pos+2))
		p.pos += 2
	case next == ' ' || next == '\t':
		skip := pos + 1
		for skip < len(src) && (src[skip] == ' ' || src[skip] == '\t') {
			skip++
		}
		if skip < len(src) && src[skip] == '\n' {
			p.stripTrailingSpacesFromLastStr()
			p.pushNode(at(&Node{Tag: HardBreak}, pos+1, skip+1))
			p.pos = skip + 1
		} else if next == ' ' {
			p.pushNode(at(&Node{Tag: NonBreakingSpace}, pos+1, pos+2))
			p.pos += 2
		} else {
			p.pushNode(at(lit("\\"), pos, pos+1))
			p.pos++
		}
	case isEscapable(next):
		p.pushNode(strNode(src[pos+1:pos+2], pos+1))
		p.pos += 2
	default:
		p.pushNode(at(lit("\\"), pos, pos+1))
		p.pos++
	}
}

func (p *inlineParser) handleBackticks() {
	src, pos := p.src, p.pos
	tickLen := countRunAt(src, pos, '`')
	after := pos + tickLen
	closePos, ok := findClosingTicks(src, after, tickLen)
	if !ok {
		p.pushNode(at(&Node{Tag: Verbatim, Text: trimVerbatimContent(src[after:])}, pos, len(src)))
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

func (p *inlineParser) stripTrailingSpacesFromLastStr() {
	n := len(p.items)
	if n == 0 || p.items[n-1].kind != itemNode {
		return
	}
	last := p.items[n-1].node
	if last.Tag != Str {
		return
	}
	last.Text = strings.TrimRight(last.Text, " \t")
	if last.Text == "" {
		p.items = p.items[:n-1]
	}
}

func (p *inlineParser) trimTrailingWhitespace() {
	for n := len(p.items); n > 0; n = len(p.items) {
		it := p.items[n-1]
		if it.kind != itemNode || it.node.Tag != Str {
			return
		}
		it.node.Text = strings.TrimRight(it.node.Text, " \t")
		if it.node.Text != "" {
			return
		}
		p.items = p.items[:n-1]
	}
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

func (p *inlineParser) handleBracedClose(char byte) bool {
	for i := len(p.openers) - 1; i >= 0; i-- {
		op := p.openers[i]
		if op.char == char && op.marked && op.itemIdx+1 < len(p.items) {
			p.closeAt(i, bracedTag(char), p.pos+2)
			return true
		}
	}
	return false
}

func emphTag(char byte) Tag {
	if char == '*' {
		return Strong
	}
	return Emph
}

func (p *inlineParser) handleEmphDelimiter(char byte, marked bool) {
	src, pos := p.src, p.pos

	if !marked && pos+1 < len(src) && src[pos+1] == '}' {
		for i := len(p.openers) - 1; i >= 0; i-- {
			op := p.openers[i]
			if op.char == char && op.marked && op.itemIdx+1 < len(p.items) {
				p.closeAt(i, emphTag(char), pos+2)
				p.pos = pos + 2
				return
			}
		}
		p.addText(src[pos:pos+2], pos)
		p.pos = pos + 2
		return
	}

	if canCloseDelim(src, pos) && !marked {
		for i := len(p.openers) - 1; i >= 0; i-- {
			op := p.openers[i]
			if op.char == char && op.marked == marked {
				if op.itemIdx+1 < len(p.items) {
					p.closeAt(i, emphTag(char), pos+1)
					p.pos = pos + 1
					return
				}
				break
			}
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
					p.pushNode(at(&Node{Tag: tag, Children: children, Reference: src[p.pos+1 : cr]}, start, cr+1))
					p.pos = cr + 1
					p.openers = p.openers[:i]
					return
				}
			case '{':
				if ba, end, ok := parseInlineAttrs(src, p.pos); ok {
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
		p.pushNode(at(lit(bracket), op.srcPos, op.srcPos+len(bracket)))
		for _, c := range children {
			p.pushNode(c)
		}
		p.pushNode(at(lit("]"), pos, pos+1))
		p.openers = p.openers[:i]
		return
	}
	p.pushNode(at(lit("]"), pos, pos+1))
	p.pos = pos + 1
}

func (p *inlineParser) handleAutolink() bool {
	src, pos := p.src, p.pos
	rest := src[pos+1:]
	close := strings.IndexByte(rest, '>')
	if close < 0 {
		return false
	}
	content := rest[:close]
	switch {
	case hasScheme(content):
		p.pushNode(at(&Node{Tag: URL, Text: content}, pos, pos+close+2))
	case strings.Contains(content, "@") && !strings.Contains(content, " "):
		p.pushNode(at(&Node{Tag: Email, Text: content}, pos, pos+close+2))
	default:
		return false
	}
	p.pos = pos + 1 + close + 1
	return true
}

// hasScheme reports whether s starts with a URL scheme ("scheme:").
func hasScheme(s string) bool {
	if s == "" || !isLetter(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if c == ':' {
			return true
		}
		if !isAlphanumeric(c) && c != '+' && c != '.' && c != '-' {
			return false
		}
	}
	return false
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

func (p *inlineParser) handleSuperSubscript(char byte, tag Tag) {
	src, pos := p.src, p.pos
	for i := len(p.openers) - 1; i >= 0; i-- {
		op := p.openers[i]
		if op.char == char && op.itemIdx+1 < len(p.items) {
			p.closeAt(i, tag, pos+1)
			p.pos = pos + 1
			return
		}
	}
	if canOpenDelim(src, pos) {
		p.pushOpener(openerInfo{char: char, itemIdx: len(p.items), srcPos: pos})
		p.pos = pos + 1
		return
	}
	p.addText(src[pos:pos+1], pos)
	p.pos = pos + 1
}

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
			emit(lit("-"), 1)
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
			emit(lit("-"), 1)
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
		for i := len(p.openers) - 1; i >= 0; i-- {
			op := p.openers[i]
			if op.char == char && op.marked {
				if op.itemIdx+1 < len(p.items) {
					p.closeAt(i, quoteTag(char), pos+2)
					p.pos = pos + 2
					return
				}
				break
			}
		}
		if char == '\'' {
			p.pushNode(at(lit("\u2019"), pos, pos+2))
		} else {
			p.pushNode(at(lit("\u201d"), pos, pos+2))
		}
		p.pos = pos + 2
		return
	}

	canClose := !marked && pos > 0 && !isInlineSpace(src[pos-1])
	canOpen := marked || (pos+1 < len(src) && !isInlineSpace(src[pos+1]) &&
		(char != '\'' || canOpenSingleQuote(src, pos)))

	if canClose {
		for i := len(p.openers) - 1; i >= 0; i-- {
			op := p.openers[i]
			if op.char == char && !op.marked {
				if op.itemIdx+1 < len(p.items) {
					p.closeAt(i, quoteTag(char), pos+1)
					p.pos = pos + 1
					return
				}
				break
			}
		}
	}

	if canOpen {
		p.pushOpener(openerInfo{char: char, itemIdx: len(p.items), srcPos: openerStart(pos, marked), marked: marked})
	} else if char == '\'' {
		// Neither opens nor closes: an apostrophe.
		p.pushNode(at(lit("\u2019"), pos, pos+1))
	} else {
		p.pushNode(at(lit("\u201c"), pos, pos+1))
	}
	p.pos = pos + 1
}

// collectChildren resolves the items after openerIdx into nodes and drops
// them, along with the opener, from the item list.
func (p *inlineParser) collectChildren(openerIdx int) []*Node {
	var children []*Node
	for _, it := range p.items[openerIdx+1:] {
		switch it.kind {
		case itemNode:
			children = append(children, it.node)
		case itemOpener:
			children = append(children, openerLit(it.opener))
		case itemAttrs:
			children = applyAttrsToResolved(children, it.attrs)
		}
	}
	p.items = p.items[:openerIdx]
	return children
}

// openerStart is where an opener whose delimiter is at pos starts: a braced
// opener starts at its "{".
func openerStart(pos int, marked bool) int {
	if marked {
		return pos - 1
	}
	return pos
}

// openerLen is the number of source bytes an opener spans.
func openerLen(op openerInfo) int {
	if op.marked || op.char == '!' {
		return 2
	}
	return 1
}

// openerLit is the text node an unmatched opener leaves behind.
func openerLit(op openerInfo) *Node {
	return at(lit(openerText(op)), op.srcPos, op.srcPos+openerLen(op))
}

// openerText is the source text an unmatched opener stands for.
func openerText(op openerInfo) string {
	if op.marked {
		switch op.char {
		case '-', '+', '=', '*', '_', '~', '^':
			return "{" + string(rune(op.char))
		case '\'':
			return "\u2018"
		case '"':
			return "\u201c"
		}
		return string(rune(op.char))
	}
	switch op.char {
	case '!':
		return "!["
	case '"':
		return "\u201c"
	case '\'':
		return "\u2019"
	}
	return string(rune(op.char))
}

func (p *inlineParser) resolveItems() []*Node {
	var nodes []*Node
	for _, it := range p.items {
		switch it.kind {
		case itemNode:
			nodes = append(nodes, it.node)
		case itemOpener:
			nodes = append(nodes, openerLit(it.opener))
		case itemAttrs:
			nodes = applyAttrsToResolved(nodes, it.attrs)
		}
	}
	return nodes
}

func isWordSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' }

// applyAttrsToResolved attaches ba to the node before it: a container node
// takes the attributes directly; trailing text has its last word wrapped in
// a span, gathering back across adjacent text nodes to the word's start.
func applyAttrsToResolved(nodes []*Node, ba blockAttrs) []*Node {
	if len(nodes) == 0 {
		return nodes
	}
	last := nodes[len(nodes)-1]
	switch last.Tag {
	case Str:
		text := last.Text
		if text == "" {
			return nodes
		}
		wordStart := len(text)
		for wordStart > 0 && !isWordSpace(text[wordStart-1]) {
			wordStart--
		}
		if wordStart == len(text) {
			return nodes
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
			return append(nodes[:gatherStart+1], span)
		case wordStart > 0:
			last.Text = text[:wordStart]
			return append(nodes, span)
		default:
			return append(nodes[:gatherStart], span)
		}
	case Span, Link, Image, Emph, Strong, Verbatim, Mark,
		Superscript, Subscript, Insert, Delete, DoubleQuoted, SingleQuoted:
		applyBlockAttrs(last, ba)
	}
	return nodes
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

// normalizeLabel collapses whitespace runs in a reference label to one space.
func normalizeLabel(raw string) string {
	if !strings.ContainsAny(raw, "\n\r\t") {
		return raw
	}
	buf := make([]byte, 0, len(raw))
	inWS := false
	for i := 0; i < len(raw); i++ {
		if isInlineSpace(raw[i]) {
			if !inWS && len(buf) > 0 {
				buf = append(buf, ' ')
				inWS = true
			}
			continue
		}
		buf = append(buf, raw[i])
		inWS = false
	}
	return strings.TrimSuffix(string(buf), " ")
}

func getPlainText(n *Node) string {
	if n.Text != "" {
		return n.Text
	}
	var b strings.Builder
	for _, c := range n.Children {
		switch c.Tag {
		case Str:
			b.WriteString(c.Text)
		case SoftBreak:
			b.WriteByte(' ')
		default:
			if len(c.Children) > 0 {
				b.WriteString(getPlainText(c))
			} else {
				b.WriteString(c.Text)
			}
		}
	}
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
