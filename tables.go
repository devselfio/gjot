package gjot

import "strings"

func (p *parser) tryTable() *Node {
	if !winIsTableRow(p.lines[p.pos]) {
		return nil
	}
	var rawRows []string
	for p.pos < len(p.lines) && winIsTableRow(p.lines[p.pos]) {
		rawRows = append(rawRows, p.lines[p.pos])
		p.pos++
	}

	// A separator row sets the alignment of the rows below it and makes the
	// row above it a header row.
	isSep := make([]bool, len(rawRows))
	aligns := make([][]CellAlign, len(rawRows))
	for i, r := range rawRows {
		if winIsTableSep(r) {
			isSep[i] = true
			aligns[i] = winParseSepAligns(r)
		}
	}

	var rows []*Node
	var current []CellAlign
	for i, r := range rawRows {
		if isSep[i] {
			current = aligns[i]
			continue
		}
		head := i+1 < len(rawRows) && isSep[i+1]
		rowAligns := current
		if head {
			rowAligns = aligns[i+1]
		}
		rows = append(rows, &Node{Tag: Row, Head: head, Children: parseTableRow(r, rowAligns, head)})
	}

	// A caption may follow after blank lines; the last one wins.
	var capLines []string
	for {
		look := p.pos
		for look < len(p.lines) && winIsBlank(p.lines[look]) {
			look++
		}
		if look >= len(p.lines) {
			break
		}
		t := strings.TrimLeft(p.lines[look], " \t")
		if !strings.HasPrefix(t, "^ ") {
			break
		}
		p.pos = look + 1
		capLines = []string{strings.Trim(t[2:], " \t")}
		for p.pos < len(p.lines) && !winIsBlank(p.lines[p.pos]) && !winIsTableRow(p.lines[p.pos]) {
			capLines = append(capLines, strings.Trim(p.lines[p.pos], " \t"))
			p.pos++
		}
	}

	var children []*Node
	if len(capLines) > 0 {
		children = append(children, &Node{Tag: Caption, Children: parseInlines(capLines)})
	}
	return &Node{Tag: Table, Children: append(children, rows...)}
}

func parseTableRow(line string, aligns []CellAlign, head bool) []*Node {
	content := strings.Trim(line, " \t")
	content = strings.TrimPrefix(content, "|")
	content = strings.TrimSuffix(content, "|")

	var cells []*Node
	for col, cell := range splitTableCells(content) {
		n := &Node{Tag: Cell, Children: parseInlines([]string{strings.Trim(cell, " \t")}), Head: head, Align: AlignDefault}
		if col < len(aligns) {
			n.Align = aligns[col]
		}
		cells = append(cells, n)
	}
	return cells
}

// skipBacktickSpan returns the index past the backtick span opening at
// start, or len(text) when it never closes.
func skipBacktickSpan(text string, start int) int {
	run := countRunAt(text, start, '`')
	for k := start + run; k < len(text); {
		if text[k] != '`' {
			k++
			continue
		}
		r2 := countRunAt(text, k, '`')
		if r2 == run {
			return k + r2
		}
		k += r2
	}
	return len(text)
}

// winIsTableRow: starts with a pipe and has at least two pipes outside escapes
// and backtick spans.
func winIsTableRow(line string) bool {
	trimmed := strings.Trim(line, " \t")
	if trimmed == "" || trimmed[0] != '|' {
		return false
	}
	pipes := 0
	for i := 0; i < len(trimmed); {
		switch {
		case trimmed[i] == '\\' && i+1 < len(trimmed):
			i += 2
		case trimmed[i] == '`':
			i = skipBacktickSpan(trimmed, i)
		default:
			if trimmed[i] == '|' {
				pipes++
			}
			i++
		}
	}
	return pipes >= 2
}

func winIsTableSep(line string) bool {
	trimmed := strings.Trim(line, " \t|")
	return trimmed != "" && strings.Trim(trimmed, "-:| ") == ""
}

func winParseSepAligns(line string) []CellAlign {
	content := strings.Trim(line, " \t")
	content = strings.TrimPrefix(content, "|")
	content = strings.TrimSuffix(content, "|")
	var result []CellAlign
	for _, cell := range strings.Split(content, "|") {
		c := strings.Trim(cell, " \t")
		left := strings.HasPrefix(c, ":")
		right := strings.HasSuffix(c, ":")
		switch {
		case c == "":
			result = append(result, AlignDefault)
		case left && right:
			result = append(result, AlignCenter)
		case right:
			result = append(result, AlignRight)
		case left:
			result = append(result, AlignLeft)
		default:
			result = append(result, AlignDefault)
		}
	}
	return result
}

func splitTableCells(content string) []string {
	var cells []string
	start := 0
	for i := 0; i < len(content); {
		switch {
		case content[i] == '\\' && i+1 < len(content):
			i += 2
		case content[i] == '`':
			i = skipBacktickSpan(content, i)
		case content[i] == '|':
			cells = append(cells, content[start:i])
			start = i + 1
			i++
		default:
			i++
		}
	}
	return append(cells, content[start:])
}
