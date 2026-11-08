package gjot

import (
	"strconv"
	"strings"
)

// leaf.go is what one line says, in the shape of zjot v2.0.0's leaf.zig:
// pure functions over a single line, used by the container stack. Unlike
// the window parser's helpers (the win* functions), a tab is one column
// here, as in djot.js.

// isBlank reports whether a line holds only spaces, tabs and a carriage
// return.
func isBlank(line string) bool {
	for i := 0; i < len(line); i++ {
		if c := line[i]; c != ' ' && c != '\t' && c != '\r' {
			return false
		}
	}
	return true
}

// countIndent is the number of leading spaces and tabs, each one column.
func countIndent(line string) int {
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	return i
}

// countLeadingChar is the length of the run of c that starts the line.
func countLeadingChar(line string, c byte) int {
	i := 0
	for i < len(line) && line[i] == c {
		i++
	}
	return i
}

// isThematicBreak reports whether a line is three or more of '-' and '*',
// in any mix, with spaces or tabs between and around them.
func isThematicBreak(line string) bool {
	marks := 0
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '-', '*':
			marks++
		case ' ', '\t', '\r':
		default:
			return false
		}
	}
	return marks >= 3
}

// fenceInfo describes a code fence: its length and character, the one-word
// info string, and the fence's indent in columns.
type fenceInfo struct {
	length int
	char   byte
	lang   string
	indent int
}

// isCodeFence reports whether a line opens a code block: three or more
// backticks or tildes, then at most one word. The word may not hold a
// backtick, whichever character the fence is made of.
func isCodeFence(line string) (fenceInfo, bool) {
	indent := countIndent(line)
	rest := line[indent:]
	if rest == "" || (rest[0] != '`' && rest[0] != '~') {
		return fenceInfo{}, false
	}
	char := rest[0]
	length := countLeadingChar(rest, char)
	if length < 3 {
		return fenceInfo{}, false
	}
	lang := strings.Trim(rest[length:], " \t\r")
	if strings.ContainsAny(lang, " \t") || strings.IndexByte(lang, '`') >= 0 {
		return fenceInfo{}, false
	}
	return fenceInfo{length: length, char: char, lang: lang, indent: indent}, true
}

// isClosingCodeFence reports whether a line closes the code block f opened:
// a run of f's character at least as long, and nothing else.
func isClosingCodeFence(line string, f fenceInfo) bool {
	rest := line[countIndent(line):]
	run := countLeadingChar(rest, f.char)
	return run >= f.length && isBlank(rest[run:])
}

// headingLevel is the number of '#' that open a heading in a line whose
// indent is already skipped, or 0 when the line opens no heading. The run
// must end the line or be followed by a space or tab, and be at most six
// long: a line of seven or more '#' is a paragraph (zjot.test L118).
func headingLevel(trimmed string) int {
	n := countLeadingChar(trimmed, '#')
	if n == 0 || n > 6 {
		return 0
	}
	if n < len(trimmed) && !isSpaceOrTab(trimmed[n]) && trimmed[n] != '\r' {
		return 0
	}
	return n
}

// startsBlockQuote reports whether a line opens a block quote: '>' after
// the indent, then a space, a tab or the line's end.
func startsBlockQuote(line string) bool {
	rest := line[countIndent(line):]
	return rest != "" && rest[0] == '>' && (len(rest) == 1 || isSpaceOrTab(rest[1]) || rest[1] == '\r')
}

// divInfo describes a fenced div's opening line: the colon fence and what
// follows it, trimmed. Whether that is a valid class is divClass's question.
type divInfo struct {
	fence string
	class string
}

// isFencedDivStart reports whether a line starts with three or more colons
// after its indent.
func isFencedDivStart(line string) (divInfo, bool) {
	rest := line[countIndent(line):]
	n := countLeadingChar(rest, ':')
	if n < 3 {
		return divInfo{}, false
	}
	return divInfo{fence: rest[:n], class: strings.Trim(rest[n:], " \t\r")}, true
}

// closesDiv reports whether a line closes a div opened with fenceLen
// colons: at least as many colons, and nothing else.
func closesDiv(line string, fenceLen int) bool {
	rest := line[countIndent(line):]
	n := countLeadingChar(rest, ':')
	return n >= 3 && n >= fenceLen && isBlank(rest[n:])
}

// divClass reports whether the text after a div's fence is a class it may
// carry: empty, or one word of ASCII letters, digits, '_' and '-'.
func divClass(class string) bool {
	for i := 0; i < len(class); i++ {
		c := class[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// definitionLabel is the opening of a reference or footnote definition:
// "[label]:" or "[^label]:". rest is what follows the colon and the spaces
// and tabs after it; restCol is rest's byte offset in the line. isFootnote
// says the label was written "^label" with a non-empty label (label then
// holds it without the '^'); spaced says a space, a tab or the line's end
// follows the colon. Which kind of definition the line opens is footnote's
// and refLabel's answer, not isFootnote's.
type definitionLabel struct {
	label      string
	rest       string
	restCol    int
	isFootnote bool
	spaced     bool
}

// footnote reports whether the line opens a footnote definition: a
// "[^label]:" followed by a space, a tab or the line's end. "[^n]:x" opens a
// reference definition labelled "^n" (zjot.test L7462).
func (d definitionLabel) footnote() bool { return d.isFootnote && d.spaced }

// refLabel is the label of the reference definition the line opens when it
// opens no footnote: the label as written, with its '^' if it had one.
func (d definitionLabel) refLabel() string {
	if d.isFootnote {
		return "^" + d.label
	}
	return d.label
}

// parseDefinitionLabel reports whether a line, after its indent, starts
// with "[label]:" or "[^label]:", whatever follows the colon. "[^]:" is a
// reference labelled "^". Whether the line opens a footnote or a reference
// is left to the caller, through footnote and refLabel, as zjot's
// Parser.zig decides it rather than leaf.zig.
func parseDefinitionLabel(line string) (definitionLabel, bool) {
	start := countIndent(line)
	rest := line[start:]
	if rest == "" || rest[0] != '[' {
		return definitionLabel{}, false
	}
	close := strings.IndexByte(rest, ']')
	if close < 0 || close+1 >= len(rest) || rest[close+1] != ':' {
		return definitionLabel{}, false
	}
	after := close + 2
	d := definitionLabel{label: rest[1:close]}
	d.spaced = after == len(rest) || isSpaceOrTab(rest[after]) || rest[after] == '\r'
	if len(d.label) > 1 && d.label[0] == '^' {
		d.label, d.isFootnote = d.label[1:], true
	}
	for after < len(rest) && isSpaceOrTab(rest[after]) {
		after++
	}
	d.rest, d.restCol = rest[after:], start+after
	return d, true
}

// isTableRow reports whether a line is a table row: after the indent it
// starts with '|' and, past trailing spaces and tabs, ends with a '|' that
// is neither escaped nor inside a backtick span.
func isTableRow(line string) bool {
	_, ok := tableRowCells(line)
	return ok
}

// isTableSep reports whether a table row is a separator: every cell is a
// run of '-' with an optional ':' at either end, spaces and tabs around it.
func isTableSep(line string) bool {
	cells, ok := tableRowCells(line)
	if !ok {
		return false
	}
	for _, c := range cells {
		c = strings.Trim(c, " \t")
		c = strings.TrimPrefix(c, ":")
		c = strings.TrimSuffix(c, ":")
		if c == "" || strings.Trim(c, "-") != "" {
			return false
		}
	}
	return true
}

// parseSepAligns is the alignment each cell of a separator row gives its
// column.
func parseSepAligns(line string) []CellAlign {
	cells := rowCells(line)
	aligns := make([]CellAlign, 0, len(cells))
	for _, c := range cells {
		c = strings.Trim(c, " \t")
		left, right := strings.HasPrefix(c, ":"), strings.HasSuffix(c, ":")
		switch {
		case left && right && len(c) > 1:
			aligns = append(aligns, AlignCenter)
		case right && !left:
			aligns = append(aligns, AlignRight)
		case left:
			aligns = append(aligns, AlignLeft)
		default:
			aligns = append(aligns, AlignDefault)
		}
	}
	return aligns
}

// rowCells is the text of each cell between a table row's outer pipes,
// untrimmed; a pipe that is escaped or inside a backtick span separates no
// cells. It is nil when the line is not a table row.
func rowCells(line string) []string {
	cells, _ := tableRowCells(line)
	return cells
}

func tableRowCells(line string) ([]string, bool) {
	rest := strings.TrimRight(line[countIndent(line):], " \t\r")
	if rest == "" || rest[0] != '|' {
		return nil, false
	}
	var cells []string
	start := 1
	for i := 1; i < len(rest); {
		switch rest[i] {
		case '\\':
			// An escaped backslash or backtick is skipped whole, but a
			// pipe is left for the case below to see.
			if i+1 < len(rest) && rest[i+1] == '|' {
				i++
			} else {
				i += 2
			}
		case '`':
			i = skipBacktickSpan(rest, i)
		case '|':
			// A pipe right after a backslash is escaped, whatever stands
			// before the backslash (zjot.test L9459).
			if rest[i-1] == '\\' {
				i++
				continue
			}
			cells = append(cells, rest[start:i])
			i++
			start = i
		default:
			i++
		}
	}
	if start != len(rest) || len(cells) == 0 {
		return nil, false
	}
	return cells, true
}

// listStyle is an ordered list's numbering.
type listStyle int

const (
	styleDecimal listStyle = iota
	styleLowerAlpha
	styleUpperAlpha
	styleLowerRoman
	styleUpperRoman
)

// htmlType is the style's value for an HTML <ol type>, empty for decimal.
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

// delim is how an ordered marker is punctuated: "1.", "1)" or "(1)".
type delim int

const (
	delimPeriod delim = iota
	delimParen
	delimParens
)

// orderedStyles is the styles an ordered list may still be: a marker such
// as "i." is roman or alphabetic until a later item decides.
type orderedStyles struct {
	possible []listStyle
}

// matches reports whether marker m can continue a list of these styles.
func (s orderedStyles) matches(m marker) bool {
	for _, a := range s.possible {
		for _, b := range m.styles {
			if a == b {
				return true
			}
		}
	}
	return false
}

// accept narrows the styles to those m shares with them.
func (s *orderedStyles) accept(m marker) {
	var kept []listStyle
	for _, a := range s.possible {
		for _, b := range m.styles {
			if a == b {
				kept = append(kept, a)
				break
			}
		}
	}
	s.possible = kept
}

// marker is a list item's marker. rest is the line after the marker and the
// one space or tab that follows it. A bullet marker sets bullet; an ordered
// marker sets styles (most likely first) and delim. markerText is the bullet
// itself, or the ordered marker's number or letters without punctuation.
type marker struct {
	rest       string
	bullet     byte
	markerText string
	styles     []listStyle
	delim      delim
}

// isTaskItem reports whether a bullet item's text opens with a task box:
// "[ ]", "[x]" or "[X]", then a space, a tab or the line's end.
func isTaskItem(rest string) bool {
	if len(rest) < 3 || rest[0] != '[' || rest[2] != ']' || (rest[1] != ' ' && rest[1] != 'x' && rest[1] != 'X') {
		return false
	}
	return len(rest) == 3 || isSpaceOrTab(rest[3]) || rest[3] == '\r'
}

// parseBulletMarker reports whether a line opens a bullet item: '-', '+' or
// '*' after the indent, then a space, a tab or the line's end (an empty
// item). A thematic break is not an item.
func parseBulletMarker(line string) (marker, bool) {
	rest := line[countIndent(line):]
	if rest == "" || (rest[0] != '-' && rest[0] != '+' && rest[0] != '*') {
		return marker{}, false
	}
	after, ok := afterMarker(rest, 1)
	if !ok || (rest[0] != '+' && isThematicBreak(rest)) {
		return marker{}, false
	}
	return marker{rest: after, bullet: rest[0], markerText: rest[:1]}, true
}

// parseOrderedMarker reports whether a line opens an ordered item: a
// number, one letter or a roman numeral, followed by '.' or ')' or wrapped
// in parentheses, then a space, a tab or the line's end.
func parseOrderedMarker(line string) (marker, bool) {
	rest := line[countIndent(line):]
	parens := rest != "" && rest[0] == '('
	body := rest
	if parens {
		body = rest[1:]
	}
	n := 0
	var styles []listStyle
	switch {
	case body != "" && isDigit(body[0]):
		for n < len(body) && isDigit(body[n]) {
			n++
		}
		styles = []listStyle{styleDecimal}
	case body != "" && (isRomanLower(body[0]) || isRomanUpper(body[0])):
		roman := isRomanLower
		if isRomanUpper(body[0]) {
			roman = isRomanUpper
		}
		for n < len(body) && roman(body[n]) {
			n++
		}
		switch {
		case n > 1:
			styles = []listStyle{romanStyle(body[0])}
		default:
			styles = []listStyle{romanStyle(body[0]), alphaStyle(body[0])}
		}
	case body != "" && isASCIILetter(body[0]):
		n = 1
		styles = []listStyle{alphaStyle(body[0])}
	default:
		return marker{}, false
	}
	if n >= len(body) {
		return marker{}, false
	}
	var d delim
	switch {
	case parens && body[n] == ')':
		d = delimParens
	case !parens && body[n] == '.':
		d = delimPeriod
	case !parens && body[n] == ')':
		d = delimParen
	default:
		return marker{}, false
	}
	width := n + 1
	if parens {
		width++
	}
	after, ok := afterMarker(rest, width)
	if !ok {
		return marker{}, false
	}
	return marker{rest: after, markerText: body[:n], styles: styles, delim: d}, true
}

// afterMarker is the text after a marker of the given width and the one
// space or tab that must follow it, unless the marker ends the line.
func afterMarker(rest string, width int) (string, bool) {
	switch {
	case width == len(rest) || (width+1 == len(rest) && rest[width] == '\r'):
		return "", true
	case isSpaceOrTab(rest[width]):
		return rest[width+1:], true
	}
	return "", false
}

func romanStyle(c byte) listStyle {
	if isRomanUpper(c) {
		return styleUpperRoman
	}
	return styleLowerRoman
}

func alphaStyle(c byte) listStyle {
	if c >= 'A' && c <= 'Z' {
		return styleUpperAlpha
	}
	return styleLowerAlpha
}

func isDigit(c byte) bool       { return c >= '0' && c <= '9' }
func isASCIILetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isSpaceOrTab(c byte) bool  { return c == ' ' || c == '\t' }

// getListStart is the number an ordered list starts at, given its first
// marker's text and the style it settled on; 1 when a decimal number does
// not fit in an int.
func getListStart(markerText string, s listStyle) int {
	switch s {
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

// romanToNumber is the value of a roman numeral of either case, 1 when it
// is empty or worth nothing.
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
