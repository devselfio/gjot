package gjot

import (
	"sort"
	"strconv"
	"strings"
)

// sharedState is shared by a parser and every nested parser it spawns for
// block quotes, list items, footnotes and definitions, so reference
// definitions and ids seen anywhere are visible everywhere.
type sharedState struct {
	refDefs       map[string]refDef
	autoRefs      map[string]string
	idsUsed       map[string]bool
	footnoteDefs  map[string][]*Node
	footnoteOrder []string
}

type refDef struct {
	url   string
	attrs *blockAttrs
}

func newSharedState() *sharedState {
	return &sharedState{
		refDefs:      map[string]refDef{},
		autoRefs:     map[string]string{},
		idsUsed:      map[string]bool{},
		footnoteDefs: map[string][]*Node{},
	}
}

// lineOrigin maps one of a parser's lines back to the original input. A
// nested parser works on stripped copies of its parent's lines (block quote
// prefixes and list indentation removed), so every line carries where its
// content came from.
type lineOrigin struct {
	line   int // 1-based line in the original input
	col    int // columns stripped before this line's content
	offset int // byte offset of this line's content in the original input
}

type parser struct {
	lines     []string
	origins   []lineOrigin // one per line
	pos       int
	shared    *sharedState
	endMarker string // the fence that closes the enclosing div, if any
	trackPos  bool
}

func newParser(input string, shared *sharedState) *parser {
	p := &parser{shared: shared}
	offset := 0
	for i, raw := range strings.Split(input, "\n") {
		p.lines = append(p.lines, strings.TrimSuffix(raw, "\r"))
		p.origins = append(p.origins, lineOrigin{line: i + 1, offset: offset})
		offset += len(raw) + 1
	}
	// A trailing newline produces an empty final element; discard it so
	// line counts match the visible content.
	if n := len(p.lines); n > 0 && p.lines[n-1] == "" {
		p.lines = p.lines[:n-1]
		p.origins = p.origins[:n-1]
	}
	return p
}

// sub is a nested parser over stripped lines that shares state and position
// tracking with p.
func (p *parser) sub(lines []string, origins []lineOrigin) *parser {
	return &parser{lines: lines, origins: origins, shared: p.shared, trackPos: p.trackPos}
}

// childOrigin is the origin of line lineIdx with its first stripped bytes
// removed, for handing to a nested parser.
func (p *parser) childOrigin(lineIdx, stripped int) lineOrigin {
	if lineIdx >= len(p.origins) {
		return lineOrigin{}
	}
	o := p.origins[lineIdx]
	return lineOrigin{line: o.line, col: o.col + displayWidth(p.lines[lineIdx][:stripped]), offset: o.offset + stripped}
}

// displayWidth is the column width of s, with a tab counting as 4.
func displayWidth(s string) int {
	return len(s) + 3*strings.Count(s, "\t")
}

// makePos converts a line index and 1-based column to a source position.
// col 0 means "end of the previous line", as djot.js reports a block's end;
// a line index past the last line means the end of the input.
func (p *parser) makePos(lineIdx, col int) SourcePos {
	n := len(p.origins)
	if n == 0 {
		return SourcePos{Line: 1 + lineIdx, Col: col}
	}
	if lineIdx >= n {
		last := p.origins[n-1]
		return SourcePos{Line: last.line + lineIdx - (n - 1), Col: col, Offset: last.offset + len(p.lines[n-1])}
	}
	o := p.origins[lineIdx]
	pos := SourcePos{Line: o.line, Col: col + o.col}
	switch {
	case col > 0:
		pos.Offset = o.offset + col - 1
	case lineIdx == 0:
		pos.Offset = o.offset
	default:
		prev := p.origins[lineIdx-1]
		pos.Offset = prev.offset + len(p.lines[lineIdx-1])
	}
	return pos
}

func (p *parser) parseDoc() *Node {
	blocks := p.parseBlocks()
	blocks = p.wrapSections(blocks)
	blocks = p.resolveReferences(blocks)
	if len(p.shared.footnoteOrder) > 0 {
		blocks = append(blocks, p.buildFootnoteSection())
	}
	return &Node{Tag: Section, Children: blocks}
}

func (p *parser) parseBlocks() []*Node {
	return p.parseBlocksUntil("")
}

func firstBlock(tries ...func() *Node) *Node {
	for _, try := range tries {
		if n := try(); n != nil {
			return n
		}
	}
	return nil
}

func (p *parser) parseBlocksUntil(endMarker string) []*Node {
	prevMarker := p.endMarker
	p.endMarker = endMarker
	defer func() { p.endMarker = prevMarker }()

	var children []*Node
	var pending *blockAttrs
	for p.pos < len(p.lines) {
		line := p.lines[p.pos]
		if isBlank(line) {
			p.pos++
			pending = nil
			continue
		}
		if endMarker != "" && p.isClosingFence(line) {
			p.pos++
			break
		}
		if ba, ok := p.tryBlockAttr(); ok {
			if pending != nil {
				ba = pending.merge(ba)
			}
			pending = &ba
			continue
		}

		node := firstBlock(p.tryHeading, p.tryThematicBreak, p.tryCodeBlock, p.tryBlockQuote, p.tryFencedDiv)
		if node == nil {
			if p.tryRefDef(pending) {
				pending = nil
				continue
			}
			if p.tryFootnoteDef() {
				continue
			}
			node = firstBlock(p.tryDefinitionList, p.tryBulletList, p.tryOrderedList, p.tryTable)
		}
		if node == nil {
			node = p.parseParagraph()
		}
		if pending != nil {
			applyBlockAttrs(node, *pending)
			pending = nil
		}
		children = append(children, node)
	}
	return children
}

// tryBlockAttr consumes a standalone attribute block, which may continue
// over indented lines until it parses.
func (p *parser) tryBlockAttr() (blockAttrs, bool) {
	line := p.lines[p.pos]
	trimmed := strings.Trim(line, " \t")
	if len(trimmed) < 1 || trimmed[0] != '{' {
		return blockAttrs{}, false
	}
	if trimmed[len(trimmed)-1] == '}' {
		if ba, ok := parseAttrsFromStr(trimmed); ok {
			p.pos++
			return ba, true
		}
	}

	indent := indentCols(line)
	full := trimmed
	consumed := 1
	for p.pos+consumed < len(p.lines) {
		next := p.lines[p.pos+consumed]
		if isBlank(next) || indentCols(next) <= indent {
			break
		}
		full += "\n" + strings.Trim(next, " \t")
		consumed++
		if ba, ok := parseAttrsFromStr(full); ok {
			p.pos += consumed
			return ba, true
		}
	}
	return blockAttrs{}, false
}

func (p *parser) isClosingFence(line string) bool {
	if p.endMarker == "" {
		return false
	}
	trimmed := strings.TrimLeft(line, " \t")
	return countLeadingChar(trimmed, ':') >= len(p.endMarker) && isBlank(strings.TrimLeft(trimmed, ":"))
}

func (p *parser) parseParagraph() *Node {
	start := p.pos
	var textLines []string
	for p.pos < len(p.lines) {
		line := p.lines[p.pos]
		if isBlank(line) || p.isClosingFence(line) {
			break
		}
		textLines = append(textLines, strings.TrimLeft(line, " \t"))
		p.pos++
	}

	src := strings.Join(textLines, "\n")
	var inlines []*Node
	if src != "" {
		inlines = parseInlineContent(src)
	}
	para := &Node{Tag: Para, Children: inlines}
	if p.trackPos {
		para.Pos = &Range{Start: p.makePos(start, p.contentCol(start)), End: p.makePos(p.pos, 0)}
		applyInlinePositions(inlines, p.buildInlineLineMap(textLines, start))
	}
	return para
}

// contentCol is the 1-based column where a line's content starts.
func (p *parser) contentCol(lineIdx int) int {
	if lineIdx >= len(p.lines) {
		return 1
	}
	line := p.lines[lineIdx]
	return len(line) - len(strings.TrimLeft(line, " \t")) + 1
}

func (p *parser) buildInlineLineMap(textLines []string, startLine int) lineMap {
	var m lineMap
	joined := 0
	for i, line := range textLines {
		orig := startLine + i
		m.segments = append(m.segments, lineSegment{joinedStart: joined, orig: p.makePos(orig, p.contentCol(orig))})
		joined += len(line) + 1
	}
	return m
}

func (p *parser) tryHeading() *Node {
	line := p.lines[p.pos]
	trimmed := strings.TrimLeft(line, " \t")
	hashes := countLeadingChar(trimmed, '#')
	if hashes == 0 || hashes > 6 {
		return nil
	}
	if hashes < len(trimmed) && trimmed[hashes] != ' ' && trimmed[hashes] != '\t' {
		return nil
	}

	p.pos++
	var contentLines []string
	if after := strings.Trim(trimmed[hashes:], " \t"); after != "" {
		contentLines = append(contentLines, after)
	}
	for p.pos < len(p.lines) {
		next := p.lines[p.pos]
		if isBlank(next) || isThematicBreak(next) || isCodeFence(next) != nil ||
			startsBlockQuote(next) || isFencedDivStart(next) != nil ||
			parseBulletMarker(next) != nil || parseOrderedMarker(next) != nil {
			break
		}
		if _, ok := tryParseBlockAttr(next); ok {
			break
		}
		nextTrimmed := strings.TrimLeft(next, " \t")
		nextHashes := countLeadingChar(nextTrimmed, '#')
		hashFollowedBySpace := nextHashes < len(nextTrimmed) && (nextTrimmed[nextHashes] == ' ' || nextTrimmed[nextHashes] == '\t')
		switch {
		case nextHashes == hashes && hashFollowedBySpace:
			contentLines = append(contentLines, strings.Trim(nextTrimmed[nextHashes:], " \t"))
		case nextHashes > 0 && nextHashes <= 6 && nextHashes != hashes && (nextHashes >= len(nextTrimmed) || hashFollowedBySpace):
			return p.heading(hashes, contentLines)
		default:
			contentLines = append(contentLines, next)
		}
		p.pos++
	}
	return p.heading(hashes, contentLines)
}

func (p *parser) heading(level int, contentLines []string) *Node {
	src := strings.Join(contentLines, "\n")
	var inlines []*Node
	if src != "" {
		inlines = parseInlineContent(src)
	}
	return &Node{Tag: Heading, Level: level, Children: inlines, idText: headingIDText(src, inlines)}
}

// headingIDText is the raw source from the first to the last top-level text
// node of a heading: what its auto id and implicit reference derive from.
func headingIDText(src string, inlines []*Node) string {
	first, end := -1, 0
	for _, c := range inlines {
		if c.Tag == Str && c.Text != "" && c.off > 0 {
			if first < 0 {
				first = c.off - 1
			}
			end = c.off - 1 + len(c.Text)
		}
	}
	if first < 0 {
		return ""
	}
	return src[first:end]
}

func (p *parser) tryThematicBreak() *Node {
	if !isThematicBreak(p.lines[p.pos]) {
		return nil
	}
	p.pos++
	return &Node{Tag: ThematicBreak}
}

func (p *parser) tryCodeBlock() *Node {
	fence := isCodeFence(p.lines[p.pos])
	if fence == nil {
		return nil
	}
	p.pos++

	var content strings.Builder
	for p.pos < len(p.lines) {
		l := p.lines[p.pos]
		p.pos++
		if closesCodeFence(l, fence) {
			break
		}
		stripped, _ := stripIndent(l, fence.indent)
		content.WriteString(stripped)
		content.WriteByte('\n')
	}

	if strings.HasPrefix(fence.lang, "=") {
		return &Node{Tag: RawBlock, Text: content.String(), Lang: fence.lang[1:]}
	}
	return &Node{Tag: CodeBlock, Text: content.String(), Lang: fence.lang}
}

func (p *parser) tryBlockQuote() *Node {
	if !startsBlockQuote(p.lines[p.pos]) {
		return nil
	}
	start := p.pos
	var inner []string
	var origins []lineOrigin
	var openFence *fenceInfo
	lazyOK := false // the last inner line is text a lazy line may continue
	for p.pos < len(p.lines) {
		l := p.lines[p.pos]
		content, stripped := l, 0
		switch {
		case startsBlockQuote(l):
			content = stripBlockQuotePrefix(l)
			stripped = len(l) - len(content)
		case lazyOK && !isBlank(l) && !p.isClosingFence(l):
		default:
			return p.blockQuote(inner, origins)
		}
		inner = append(inner, content)
		origins = append(origins, p.childOrigin(start+len(inner)-1, stripped))
		p.pos++

		// Verbatim blocks, rules and fences never grow by laziness.
		switch {
		case openFence != nil:
			if closesCodeFence(content, openFence) {
				openFence = nil
			}
			lazyOK = false
		case isCodeFence(content) != nil:
			openFence = isCodeFence(content)
			lazyOK = false
		default:
			lazyOK = !isBlank(content) && !isThematicBreak(content) && isFencedDivStart(content) == nil
		}
	}
	return p.blockQuote(inner, origins)
}

func (p *parser) blockQuote(inner []string, origins []lineOrigin) *Node {
	return &Node{Tag: BlockQuote, Children: p.sub(inner, origins).parseBlocks()}
}

func (p *parser) tryFencedDiv() *Node {
	div := isFencedDivStart(p.lines[p.pos])
	if div == nil {
		return nil
	}
	p.pos++
	return &Node{Tag: Div, Children: p.parseBlocksUntil(div.fence), Classes: div.class}
}

// tryRefDef consumes `[label]: url`, with the url optionally continued on
// indented lines, into the shared reference table.
func (p *parser) tryRefDef(pending *blockAttrs) bool {
	trimmed := strings.TrimLeft(p.lines[p.pos], " \t")
	if len(trimmed) < 4 || trimmed[0] != '[' {
		return false
	}
	closeBracket := strings.IndexByte(trimmed[1:], ']')
	if closeBracket < 0 || closeBracket+2 >= len(trimmed) || trimmed[closeBracket+2] != ':' {
		return false
	}
	label := trimmed[1 : closeBracket+1]
	if strings.HasPrefix(label, "^") {
		return false
	}

	p.pos++
	var parts []string
	if dest := strings.Trim(trimmed[closeBracket+3:], " \t"); dest != "" {
		parts = append(parts, dest)
	}
	for p.pos < len(p.lines) {
		next := p.lines[p.pos]
		if isBlank(next) || (next[0] != ' ' && next[0] != '\t') {
			break
		}
		if strings.HasPrefix(strings.TrimLeft(next, " \t"), "[") {
			break
		}
		parts = append(parts, strings.Trim(next, " \t"))
		p.pos++
	}
	p.shared.refDefs[label] = refDef{url: strings.Join(parts, ""), attrs: pending}
	return true
}

// tryFootnoteDef consumes `[^label]: content` with its indented
// continuation into the shared footnote table.
func (p *parser) tryFootnoteDef() bool {
	line := p.lines[p.pos]
	trimmed := strings.TrimLeft(line, " \t")
	if len(trimmed) < 5 || !strings.HasPrefix(trimmed, "[^") {
		return false
	}
	close := strings.IndexByte(trimmed[2:], ']')
	if close < 0 || close+3 >= len(trimmed) || trimmed[close+3] != ':' {
		return false
	}
	label := trimmed[2 : close+2]

	var content []string
	var origins []lineOrigin
	add := func(text string, stripped int) {
		content = append(content, text)
		origins = append(origins, p.childOrigin(p.pos, stripped))
	}
	if rest := strings.TrimLeft(trimmed[close+4:], " \t"); rest != "" {
		add(strings.TrimRight(rest, " \t"), len(line)-len(rest))
	}
	p.pos++
	for p.pos < len(p.lines) {
		l := p.lines[p.pos]
		switch {
		case isBlank(l):
			look := p.pos + 1
			for look < len(p.lines) && isBlank(p.lines[look]) {
				look++
			}
			if look < len(p.lines) && (p.lines[look][0] == ' ' || p.lines[look][0] == '\t') {
				add("", 0)
				p.pos++
				continue
			}
		case l[0] == ' ' || l[0] == '\t':
			stripped := strings.TrimLeft(l, " \t")
			add(stripped, len(l)-len(stripped))
			p.pos++
			continue
		}
		break
	}

	p.shared.footnoteDefs[label] = p.sub(content, origins).parseBlocks()
	return true
}

// --- Section wrapping, id generation, reference resolution ---

func (p *parser) registerExplicitIDs(blocks []*Node) {
	for _, b := range blocks {
		if b.ID != "" {
			p.shared.idsUsed[b.ID] = true
		}
		p.registerExplicitIDs(b.Children)
	}
}

type sectionInfo struct {
	heading  *Node // the original heading, whose id and attrs move to the section
	id       string
	autoID   bool
	children []*Node
}

// wrapSections nests blocks into sections by heading level. A heading's id
// and attributes move to its section so the section carries the anchor.
func (p *parser) wrapSections(blocks []*Node) []*Node {
	p.registerExplicitIDs(blocks)

	var result []*Node
	var stack []sectionInfo
	closeTop := func() {
		top := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		sec := &Node{Tag: Section, Level: top.heading.Level, ID: top.id, AutoID: top.autoID,
			Children: top.children, Attrs: top.heading.Attrs, Classes: top.heading.Classes}
		if len(stack) > 0 {
			stack[len(stack)-1].children = append(stack[len(stack)-1].children, sec)
		} else {
			result = append(result, sec)
		}
	}

	for _, block := range blocks {
		if block.Tag != Heading {
			p.assignNestedHeadingIDs(block.Children)
			if len(stack) > 0 {
				stack[len(stack)-1].children = append(stack[len(stack)-1].children, block)
			} else {
				result = append(result, block)
			}
			continue
		}
		for len(stack) > 0 && stack[len(stack)-1].heading.Level >= block.Level {
			closeTop()
		}

		id, auto := block.ID, false
		if id == "" {
			id, auto = p.generateID(block.idText), true
		}
		p.shared.autoRefs[normalizeLabel(block.idText)] = "#" + id

		heading := *block
		heading.ID, heading.Attrs, heading.Classes = "", nil, ""
		stack = append(stack, sectionInfo{heading: block, id: id, autoID: auto, children: []*Node{&heading}})
	}
	for len(stack) > 0 {
		closeTop()
	}
	for _, label := range sortedKeys(p.shared.footnoteDefs) {
		p.assignNestedHeadingIDs(p.shared.footnoteDefs[label])
	}
	return result
}

// assignNestedHeadingIDs gives headings outside the section structure
// (inside block quotes, divs, list items, definitions, footnotes) their own
// id, generated when not written, and an implicit reference.
func (p *parser) assignNestedHeadingIDs(blocks []*Node) {
	for _, b := range blocks {
		if b.Tag == Heading {
			if b.ID == "" {
				b.ID = p.generateID(b.idText)
				b.AutoID = true
			}
			p.shared.autoRefs[normalizeLabel(b.idText)] = "#" + b.ID
		}
		p.assignNestedHeadingIDs(b.Children)
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// generateID derives a unique id from heading text: alphanumerics kept,
// whitespace and -_ runs become one hyphen, collisions get -1, -2, ...
func (p *parser) generateID(text string) string {
	var buf []byte
	prevHyphen := true
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case isAlphanumeric(c):
			buf = append(buf, c)
			prevHyphen = false
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '-' || c == '_':
			if !prevHyphen && len(buf) > 0 {
				buf = append(buf, '-')
				prevHyphen = true
			}
		}
	}
	base := strings.TrimRight(string(buf), "-")
	if base == "" {
		base = "s-" + strconv.Itoa(len(p.shared.idsUsed)+1)
	}
	if !p.shared.idsUsed[base] {
		p.shared.idsUsed[base] = true
		return base
	}
	for counter := 1; ; counter++ {
		candidate := base + "-" + strconv.Itoa(counter)
		if !p.shared.idsUsed[candidate] {
			p.shared.idsUsed[candidate] = true
			return candidate
		}
	}
}

// resolveReferences fills in reference links from the definitions and
// implicit heading references, and numbers footnote references in order of
// first use. Nodes are updated in place.
func (p *parser) resolveReferences(nodes []*Node) []*Node {
	for _, n := range nodes {
		if (n.Tag == Link || n.Tag == Image) && n.Destination == nil {
			raw := n.Reference
			if raw == "" {
				raw = getPlainText(n)
			}
			label := normalizeLabel(raw)
			if def, ok := p.shared.refDefs[label]; ok {
				url := def.url
				n.Destination = &url
				if def.attrs != nil {
					mergeRefAttrs(n, *def.attrs)
				}
			} else if dest, ok := p.shared.autoRefs[label]; ok {
				n.Destination = &dest
			}
		}
		if n.Tag == FootnoteReference {
			num := p.footnoteNumber(n.Text)
			n.Text = strconv.Itoa(num)
			n.ID = "fnref" + strconv.Itoa(num)
			dest := "#fn" + strconv.Itoa(num)
			n.Destination = &dest
		}
		p.resolveReferences(n.Children)
	}
	return nodes
}

func (p *parser) footnoteNumber(label string) int {
	for i, existing := range p.shared.footnoteOrder {
		if existing == label {
			return i + 1
		}
	}
	p.shared.footnoteOrder = append(p.shared.footnoteOrder, label)
	return len(p.shared.footnoteOrder)
}

// buildFootnoteSection renders the referenced footnotes, in reference
// order, as an endnotes list with a backlink after each note's last paragraph.
func (p *parser) buildFootnoteSection() *Node {
	var items []*Node
	// footnoteOrder can grow while resolving a note that references another.
	for i := 0; i < len(p.shared.footnoteOrder); i++ {
		label := p.shared.footnoteOrder[i]
		content := p.resolveReferences(p.shared.footnoteDefs[label])
		num := strconv.Itoa(len(items) + 1)
		backref := "#fnref" + num
		backlink := &Node{Tag: Link, Destination: &backref, Text: "↩︎", Attrs: []Attr{{"role", "doc-backlink"}}}
		if n := len(content); n > 0 && content[n-1].Tag == Para {
			last := content[n-1]
			last.Children = append(last.Children, backlink)
		} else {
			content = append(content, &Node{Tag: Para, Children: []*Node{backlink}})
		}
		items = append(items, &Node{Tag: ListItem, ID: "fn" + num, Children: content})
	}
	return &Node{Tag: Footnote, Children: items, Attrs: []Attr{{"role", "doc-endnotes"}}}
}

// --- Line classification ---

func isBlank(line string) bool {
	return strings.Trim(line, " \t\r") == ""
}

func isNewBlockStart(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	if trimmed == "" {
		return false
	}
	if trimmed[0] == '#' || trimmed[0] == '>' {
		return true
	}
	if isCodeFence(line) != nil || parseBulletMarker(line) != nil || parseOrderedMarker(line) != nil || isThematicBreak(trimmed) {
		return true
	}
	return trimmed[0] == ':' && len(trimmed) > 1 && (trimmed[1] == ' ' || trimmed[1] == '\t')
}

// indentCols is the width of a line's leading whitespace; a tab counts as 4.
func indentCols(line string) int {
	n := 0
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case ' ':
			n++
		case '\t':
			n += 4
		default:
			return n
		}
	}
	return n
}

// stripIndent removes up to cols columns of leading whitespace and returns
// the rest with the number of bytes removed. A tab that crosses the limit
// is removed whole.
func stripIndent(line string, cols int) (string, int) {
	i, w := 0, 0
	for i < len(line) && w < cols {
		switch line[i] {
		case ' ':
			w++
		case '\t':
			w += 4
		default:
			return line[i:], i
		}
		i++
	}
	return line[i:], i
}

func countLeadingChar(line string, char byte) int {
	for i := 0; i < len(line); i++ {
		if line[i] != char {
			return i
		}
	}
	return len(line)
}

func isThematicBreak(line string) bool {
	trimmed := strings.Trim(line, " \t")
	if len(trimmed) < 3 {
		return false
	}
	count := 0
	for i := 0; i < len(trimmed); i++ {
		switch trimmed[i] {
		case ' ':
		case '*', '-':
			count++
		default:
			return false
		}
	}
	return count >= 3
}

type fenceInfo struct {
	length int
	char   byte
	lang   string
	indent int
}

// closesCodeFence reports whether line closes the block opened by fence.
func closesCodeFence(line string, fence *fenceInfo) bool {
	unindented := strings.TrimLeft(line, " \t")
	run := countLeadingChar(unindented, fence.char)
	return run >= fence.length && isBlank(strings.TrimLeft(unindented[run:], string(rune(fence.char))))
}

func isCodeFence(line string) *fenceInfo {
	indent := indentCols(line)
	rest := strings.TrimLeft(line, " \t")
	if len(rest) < 3 || (rest[0] != '`' && rest[0] != '~') {
		return nil
	}
	char := rest[0]
	length := countLeadingChar(rest, char)
	if length < 3 {
		return nil
	}
	after := strings.Trim(rest[length:], " \t")
	if char == '`' && after != "" {
		// An info string with spaces or a backtick run as long as the fence
		// makes this inline code, not a fence.
		if strings.Contains(after, " ") {
			return nil
		}
		for i := 0; i < len(after); i++ {
			if after[i] == '`' {
				run := countRunAt(after, i, '`')
				if run >= length {
					return nil
				}
				i += run
			}
		}
	}
	return &fenceInfo{length: length, char: char, lang: after, indent: indent}
}

func startsBlockQuote(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	if trimmed == "" || trimmed[0] != '>' {
		return false
	}
	return len(trimmed) == 1 || trimmed[1] == ' ' || trimmed[1] == '\t'
}

func stripBlockQuotePrefix(line string) string {
	trimmed := strings.TrimLeft(line, " \t")
	switch {
	case trimmed == "":
		return ""
	case trimmed[0] != '>':
		return line
	case len(trimmed) == 1:
		return ""
	case trimmed[1] == ' ' || trimmed[1] == '\t':
		return trimmed[2:]
	}
	return trimmed[1:]
}

type divInfo struct {
	fence string
	class string
}

func isFencedDivStart(line string) *divInfo {
	trimmed := strings.TrimLeft(line, " \t")
	colons := countLeadingChar(trimmed, ':')
	if colons < 3 {
		return nil
	}
	return &divInfo{fence: trimmed[:colons], class: strings.Trim(trimmed[colons:], " \t")}
}
