package gjot

import (
	"strconv"
	"strings"
)

// itemContent is a list item's lines split at its first blank line: the
// leading paragraph lines (with their origin, for source positions) and
// the block lines that follow.
type itemContent struct {
	paraLines    []string
	paraOrigins  []lineOrigin
	blockLines   []string
	blockOrigins []lineOrigin
	sawBlank     bool
}

func (p *parser) collectItemContent(contentCol, listIndent int, firstRest string) itemContent {
	var c itemContent
	addPara := func(text string, stripped int) {
		c.paraLines = append(c.paraLines, text)
		c.paraOrigins = append(c.paraOrigins, p.childOrigin(p.pos, stripped))
	}
	addBlock := func(text string, stripped int) {
		c.blockLines = append(c.blockLines, text)
		c.blockOrigins = append(c.blockOrigins, p.childOrigin(p.pos, stripped))
	}
	addPara(firstRest, len(p.lines[p.pos])-len(firstRest))
	p.pos++

	for p.pos < len(p.lines) && !c.sawBlank {
		next := p.lines[p.pos]
		if isBlank(next) {
			c.sawBlank = true
			p.pos++
			continue
		}
		if m := parseBulletMarker(next); m != nil && m.indent <= listIndent {
			break
		}
		if m := parseOrderedMarker(next); m != nil && m.indent <= listIndent {
			break
		}
		addPara(stripIndent(next, contentCol))
		p.pos++
	}

	lastWasBlank := true
	for p.pos < len(p.lines) && c.sawBlank {
		next := p.lines[p.pos]
		if isBlank(next) {
			addBlock("", 0)
			p.pos++
			lastWasBlank = true
			continue
		}
		nextIndent := indentCols(next)
		if nextIndent > listIndent {
			strip := contentCol
			if parseBulletMarker(next) != nil || parseOrderedMarker(next) != nil {
				strip = listIndent + 1
			}
			addBlock(stripIndent(next, strip))
			p.pos++
			lastWasBlank = false
			continue
		}
		if !lastWasBlank && !isNewBlockStart(next) {
			stripped := strings.TrimLeft(next, " \t")
			addBlock(stripped, len(next)-len(stripped))
			p.pos++
			lastWasBlank = false
			continue
		}
		break
	}

	for len(c.blockLines) > 0 && isBlank(c.blockLines[len(c.blockLines)-1]) {
		c.blockLines = c.blockLines[:len(c.blockLines)-1]
		c.blockOrigins = c.blockOrigins[:len(c.blockOrigins)-1]
	}
	return c
}

func (p *parser) parseItemContent(c *itemContent) []*Node {
	var blocks []*Node
	if strings.Join(c.paraLines, "") != "" {
		blocks = append(blocks, p.sub(c.paraLines, c.paraOrigins).parseBlocks()...)
	}
	if len(c.blockLines) > 0 {
		blocks = append(blocks, p.sub(c.blockLines, c.blockOrigins).parseBlocks()...)
	}
	return blocks
}

// updateListTightness: an item with blocks after a blank line is loose only
// if it holds more than one paragraph; a trailing blank with no blocks
// loosens the list if another item follows.
func updateListTightness(tight, sawBlankBetween *bool, c *itemContent, blocks []*Node) {
	if c.sawBlank && len(c.blockLines) > 0 {
		paras := 0
		for _, b := range blocks {
			if b.Tag == Para {
				paras++
			}
		}
		if paras > 1 {
			*tight = false
		}
	}
	if c.sawBlank && len(c.blockLines) == 0 {
		*sawBlankBetween = true
	}
}

func (p *parser) itemRange(startLine, listIndent int) *Range {
	if !p.trackPos {
		return nil
	}
	endCol := 1
	if p.pos >= len(p.lines) {
		endCol = 0
	}
	return &Range{Start: p.makePos(startLine, listIndent+1), End: p.makePos(p.pos, endCol)}
}

func (p *parser) listRange(startLine, listIndent int) *Range {
	if !p.trackPos {
		return nil
	}
	return &Range{Start: p.makePos(startLine, listIndent+1), End: p.makePos(p.pos, 0)}
}

func (p *parser) tryBulletList() *Node {
	first := parseBulletMarker(p.lines[p.pos])
	if first == nil {
		return nil
	}
	listIndent, marker, listStart := first.indent, first.marker, p.pos

	var items []*Node
	tight, sawBlankBetween, hasTask := true, false, false
	for p.pos < len(p.lines) {
		cur := parseBulletMarker(p.lines[p.pos])
		if cur == nil || cur.indent != listIndent || cur.marker != marker {
			break
		}
		if sawBlankBetween {
			tight = false
		}
		sawBlankBetween = false

		itemStart := p.pos
		content := p.collectItemContent(cur.contentCol, listIndent, cur.rest)

		item := &Node{Tag: ListItem}
		if first := content.paraLines[0]; strings.HasPrefix(first, "[ ] ") || strings.HasPrefix(first, "[x] ") || strings.HasPrefix(first, "[X] ") {
			checked := first[1] != ' '
			item = &Node{Tag: TaskListItem, Checked: &checked}
			content.paraLines[0] = first[4:]
			content.paraOrigins[0].col += 4
			content.paraOrigins[0].offset += 4
			hasTask = true
		}
		item.Children = p.parseItemContent(&content)
		item.Pos = p.itemRange(itemStart, listIndent)
		items = append(items, item)

		updateListTightness(&tight, &sawBlankBetween, &content, item.Children)
	}

	tag := BulletList
	if hasTask {
		tag = TaskList
	}
	return &Node{Tag: tag, Children: items, Tight: tight, Style: string(rune(marker)), Pos: p.listRange(listStart, listIndent)}
}

func (p *parser) tryOrderedList() *Node {
	first := parseOrderedMarker(p.lines[p.pos])
	if first == nil {
		return nil
	}
	listIndent, listStart := first.indent, p.pos
	possible := first.styles

	var items []*Node
	tight, sawBlankBetween := true, false
	for p.pos < len(p.lines) {
		cur := parseOrderedMarker(p.lines[p.pos])
		if cur == nil || cur.indent != listIndent {
			break
		}
		// A marker continues the list only if it can share a style with it;
		// the shared styles narrow as the list goes on.
		if len(items) > 0 {
			common := intersectStyles(possible, cur.styles)
			if len(common) == 0 {
				break
			}
			possible = common
		}

		if sawBlankBetween && len(items) > 0 {
			tight = false
		}
		sawBlankBetween = false

		itemStart := p.pos
		content := p.collectItemContent(cur.contentCol, listIndent, cur.rest)
		item := &Node{Tag: ListItem, Children: p.parseItemContent(&content)}
		item.Pos = p.itemRange(itemStart, listIndent)
		items = append(items, item)

		updateListTightness(&tight, &sawBlankBetween, &content, item.Children)
	}

	style := possible[0]
	var attrs []Attr
	if start := getListStart(first.markerText, style); start != 1 {
		attrs = append(attrs, Attr{"start", strconv.Itoa(start)})
	}
	if t := style.htmlType(); t != "" {
		attrs = append(attrs, Attr{"type", t})
	}
	return &Node{Tag: OrderedList, Children: items, Tight: tight, Attrs: attrs, Pos: p.listRange(listStart, listIndent)}
}

func intersectStyles(possible, cur []listStyle) []listStyle {
	var common []listStyle
	for _, ps := range possible {
		for _, cs := range cur {
			if cs == ps {
				common = append(common, ps)
				break
			}
		}
	}
	return common
}

func isDefinitionMarker(trimmed string) bool {
	return len(trimmed) >= 2 && trimmed[0] == ':' && (trimmed[1] == ' ' || trimmed[1] == '\t')
}

func (p *parser) tryDefinitionList() *Node {
	if !isDefinitionMarker(strings.TrimLeft(p.lines[p.pos], " \t")) {
		return nil
	}

	var items []*Node
	for p.pos < len(p.lines) {
		cur := p.lines[p.pos]
		curTrimmed := strings.TrimLeft(cur, " \t")
		if !isDefinitionMarker(curTrimmed) {
			break
		}
		markerIndent := indentCols(cur)
		contentIndent := markerIndent + 2
		var content []string
		var origins []lineOrigin
		add := func(text string, stripped int) {
			content = append(content, text)
			origins = append(origins, p.childOrigin(p.pos, stripped))
		}
		add(curTrimmed[2:], len(cur)-len(curTrimmed)+2)
		p.pos++

		for p.pos < len(p.lines) {
			next := p.lines[p.pos]
			if isBlank(next) {
				first := p.pos
				for p.pos < len(p.lines) && isBlank(p.lines[p.pos]) {
					p.pos++
				}
				if p.pos >= len(p.lines) || indentCols(p.lines[p.pos]) < contentIndent {
					break
				}
				for blank := first; blank < p.pos; blank++ {
					content = append(content, "")
					origins = append(origins, p.childOrigin(blank, 0))
				}
				continue
			}
			ni := indentCols(next)
			switch {
			case ni >= contentIndent:
				add(stripIndent(next, contentIndent))
			case ni > markerIndent:
				stripped := strings.TrimLeft(next, " \t")
				add(stripped, len(next)-len(stripped))
			default:
				goto itemDone
			}
			p.pos++
		}
	itemDone:
		blocks := p.sub(content, origins).parseBlocks()
		term := &Node{Tag: Term}
		if len(blocks) > 0 && blocks[0].Tag == Para {
			term.Children = blocks[0].Children
			blocks = blocks[1:]
		}
		items = append(items, &Node{Tag: DefinitionListItem, Children: []*Node{term, {Tag: Definition, Children: blocks}}})
	}

	if len(items) == 0 {
		return nil
	}
	return &Node{Tag: DefinitionList, Children: items}
}

type bulletInfo struct {
	rest       string
	indent     int
	contentCol int
	marker     byte
}

func parseBulletMarker(line string) *bulletInfo {
	indent := indentCols(line)
	trimmed := strings.TrimLeft(line, " \t")
	if len(trimmed) < 2 || (trimmed[0] != '-' && trimmed[0] != '+' && trimmed[0] != '*') {
		return nil
	}
	if trimmed[1] != ' ' && trimmed[1] != '\t' {
		return nil
	}
	if trimmed[0] != '+' && isThematicBreak(line) {
		return nil
	}
	return &bulletInfo{rest: trimmed[2:], indent: indent, contentCol: indent + 2, marker: trimmed[0]}
}

type listStyle int

const (
	styleDecimal listStyle = iota
	styleLowerAlpha
	styleUpperAlpha
	styleLowerRoman
	styleUpperRoman
)

func (s listStyle) htmlType() string {
	switch s {
	case styleLowerAlpha:
		return "a"
	case styleUpperAlpha:
		return "A"
	case styleLowerRoman:
		return "i"
	case styleUpperRoman:
		return "I"
	}
	return ""
}

type orderedInfo struct {
	rest       string
	indent     int
	contentCol int
	styles     []listStyle // candidate styles; a single letter may be alpha or roman
	markerText string
}

func parseOrderedMarker(line string) *orderedInfo {
	indent := indentCols(line)
	trimmed := strings.TrimLeft(line, " \t")
	if len(trimmed) < 2 {
		return nil
	}
	parenOpen := trimmed[0] == '('
	start := 0
	if parenOpen {
		start = 1
	}
	after := trimmed[start:]
	if after == "" {
		return nil
	}
	delimOK := func(d byte) bool {
		if parenOpen {
			return d == ')'
		}
		return d == '.' || d == ')'
	}

	digits := 0
	for digits < len(after) && after[digits] >= '0' && after[digits] <= '9' {
		digits++
	}
	if digits > 0 && digits < len(after) && delimOK(after[digits]) {
		return finishMarker(trimmed, indent, start+digits+1, []listStyle{styleDecimal}, after[:digits])
	}
	if len(after) < 2 {
		return nil
	}

	c := after[0]
	if c >= 'a' && c <= 'z' && delimOK(after[1]) {
		styles := []listStyle{styleLowerAlpha}
		if isRomanLower(c) {
			styles = []listStyle{styleLowerRoman, styleLowerAlpha}
		}
		return finishMarker(trimmed, indent, start+2, styles, after[:1])
	}
	if c >= 'A' && c <= 'Z' && delimOK(after[1]) {
		styles := []listStyle{styleUpperAlpha}
		if isRomanUpper(c) {
			styles = []listStyle{styleUpperRoman, styleUpperAlpha}
		}
		return finishMarker(trimmed, indent, start+2, styles, after[:1])
	}
	for _, roman := range []struct {
		is    func(byte) bool
		style listStyle
	}{{isRomanLower, styleLowerRoman}, {isRomanUpper, styleUpperRoman}} {
		if !roman.is(c) {
			continue
		}
		j := 0
		for j < len(after) && roman.is(after[j]) {
			j++
		}
		if j > 1 && j < len(after) && delimOK(after[j]) {
			return finishMarker(trimmed, indent, start+j+1, []listStyle{roman.style}, after[:j])
		}
	}
	return nil
}

func finishMarker(trimmed string, indent, markerLen int, styles []listStyle, markerText string) *orderedInfo {
	info := &orderedInfo{indent: indent, contentCol: indent + markerLen + 1, styles: styles, markerText: markerText}
	if markerLen >= len(trimmed) {
		return info
	}
	if trimmed[markerLen] != ' ' && trimmed[markerLen] != '\t' {
		return nil
	}
	info.rest = trimmed[markerLen+1:]
	return info
}

func getListStart(markerText string, style listStyle) int {
	switch style {
	case styleDecimal:
		n, err := strconv.Atoi(markerText)
		if err != nil {
			return 1
		}
		return n
	case styleLowerAlpha:
		if len(markerText) == 1 {
			return int(markerText[0]-'a') + 1
		}
	case styleUpperAlpha:
		if len(markerText) == 1 {
			return int(markerText[0]-'A') + 1
		}
	case styleLowerRoman, styleUpperRoman:
		return romanToNumber(markerText)
	}
	return 1
}

func isRomanLower(c byte) bool { return strings.IndexByte("ivxlcdm", c) >= 0 }
func isRomanUpper(c byte) bool { return strings.IndexByte("IVXLCDM", c) >= 0 }

func romanToNumber(s string) int {
	total, prev := 0, 0
	for i := len(s) - 1; i >= 0; i-- {
		var n int
		switch s[i] {
		case 'i', 'I':
			n = 1
		case 'v', 'V':
			n = 5
		case 'x', 'X':
			n = 10
		case 'l', 'L':
			n = 50
		case 'c', 'C':
			n = 100
		case 'd', 'D':
			n = 500
		case 'm', 'M':
			n = 1000
		}
		if n < prev {
			total = max(total-n, 0)
		} else {
			total += n
		}
		prev = n
	}
	if total == 0 {
		return 1
	}
	return total
}
