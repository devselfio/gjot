package gjot

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Djot writes doc back as Djot source. The result re-parses to the same
// HTML and AST (positions aside); it is not the original text. Reference
// definitions are written as inline destinations and footnotes are renumbered
// in order of first reference.
func Djot(doc *Node) []byte {
	d := &djotWriter{}
	lines := d.blocks(doc.Children)
	for _, def := range d.refDefs {
		lines = append(lines, "", "["+def.label+"]: "+def.url)
	}
	if len(lines) == 0 {
		return nil
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

// djotWriter renders blocks and collects the reference definitions that
// reference links need, written at the end of the document.
type djotWriter struct {
	refDefs []refDefOut
	defined map[string]bool
}

type refDefOut struct {
	label, url string
}

// defineRef records a definition for label unless one exists. It reports
// whether the link can be written by reference.
func (d *djotWriter) defineRef(label, url string) bool {
	if label == "" || strings.HasPrefix(label, "^") || strings.ContainsAny(label, "[]\n") {
		return false
	}
	if d.defined == nil {
		d.defined = map[string]bool{}
	}
	if !d.defined[label] {
		d.defined[label] = true
		d.refDefs = append(d.refDefs, refDefOut{label, url})
	}
	return true
}

// blocks renders sibling blocks as lines with one blank line between.
func (d *djotWriter) blocks(blocks []*Node) []string {
	var lines []string
	for i, b := range blocks {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, d.block(b)...)
	}
	return lines
}

// indented prefixes lines: the first with first, blank lines with blank,
// the rest with rest.
func indented(lines []string, first, rest, blank string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		switch {
		case i == 0:
			out[i] = first + l
		case l == "":
			out[i] = blank
		default:
			out[i] = rest + l
		}
	}
	return out
}

func (d *djotWriter) block(n *Node) []string {
	var lines []string
	switch n.Tag {
	case Section:
		if a := attrSet(n); a != "" {
			lines = append(lines, a)
		}
		return append(lines, d.blocks(n.Children)...)
	case Footnote:
		return d.footnotes(n)
	}
	if a := attrSet(n); a != "" {
		lines = append(lines, a)
	}
	switch n.Tag {
	case OrderedList:
		return append(lines, d.orderedList(n)...)
	case Para:
		text := d.inlines(n.Children)
		if strings.HasSuffix(text, "\n") {
			text += "{}" // keeps the trailing soft break from reading as a blank line
		}
		return append(lines, strings.Split(text, "\n")...)
	case Heading:
		hashes := strings.Repeat("#", n.Level)
		text := d.inlines(n.Children)
		if text == "" {
			return append(lines, hashes)
		}
		return append(lines, indented(strings.Split(text, "\n"), hashes+" ", hashes+" ", "")...)
	case ThematicBreak:
		return append(lines, "* * *")
	case CodeBlock, RawBlock:
		content := strings.Split(strings.TrimSuffix(n.Text, "\n"), "\n")
		if n.Text == "" {
			content = nil
		}
		fence := strings.Repeat("`", fenceLength(content, '`'))
		open := fence
		if n.Tag == RawBlock {
			open += "=" + n.Lang
		} else if n.Lang != "" {
			open += " " + n.Lang
		}
		lines = append(lines, open)
		lines = append(lines, content...)
		return append(lines, fence)
	case BlockQuote:
		inner := d.blocks(n.Children)
		if len(inner) == 0 {
			return append(lines, ">")
		}
		return append(lines, indented(inner, "> ", "> ", ">")...)
	case Div:
		inner := d.blocks(n.Children)
		fence := strings.Repeat(":", fenceLength(inner, ':'))
		open := fence
		if n.Classes != "" {
			open += " " + n.Classes
		}
		lines = append(lines, open)
		lines = append(lines, inner...)
		return append(lines, fence)
	case BulletList, TaskList:
		marker := n.Style
		if marker == "" {
			marker = "-"
		}
		for i, item := range n.Children {
			if i > 0 && !n.Tight {
				lines = append(lines, "")
			}
			first := marker + " "
			if item.Tag == TaskListItem {
				if item.Checked != nil && *item.Checked {
					first += "[x] "
				} else {
					first += "[ ] "
				}
			}
			lines = append(lines, d.item(item.Children, first, "  ")...)
		}
		return lines
	case DefinitionList:
		for i, item := range n.Children {
			if i > 0 {
				lines = append(lines, "")
			}
			var term, def *Node
			for _, c := range item.Children {
				switch c.Tag {
				case Term:
					term = c
				case Definition:
					def = c
				}
			}
			termLines := []string{""}
			if term != nil && len(term.Children) > 0 {
				termLines = strings.Split(d.inlines(term.Children), "\n")
			}
			lines = append(lines, indented(termLines, ": ", "  ", "")...)
			if def != nil && len(def.Children) > 0 {
				lines = append(lines, "")
				lines = append(lines, indented(d.blocks(def.Children), "  ", "  ", "")...)
			}
		}
		return lines
	case Table:
		return append(lines, d.table(n)...)
	}
	return lines
}

// writeItem renders a list item's blocks under its marker.
func (d *djotWriter) item(blocks []*Node, first, rest string) []string {
	inner := d.blocks(blocks)
	if len(inner) == 0 {
		return []string{first}
	}
	return indented(inner, first, rest, "")
}

// fenceLength is one more than the longest run of char opening a line, and
// at least three.
func fenceLength(lines []string, char byte) int {
	n := 3
	for _, l := range lines {
		if run := countLeadingChar(strings.TrimLeft(l, " "), char); run >= n {
			n = run + 1
		}
	}
	return n
}

// orderedList writes the items with markers counted from the list's start
// in its style and delimiter, so the list re-parses to the same style and
// start and stays apart from a neighbour of another delimiter.
func (d *djotWriter) orderedList(n *Node) []string {
	style, delim, ok := parseStyleString(n.Style)
	if !ok {
		style, delim = styleDecimal, delimPeriod
	}
	start := orderedStart(n)
	var lines []string
	for i, item := range n.Children {
		if i > 0 && !n.Tight {
			lines = append(lines, "")
		}
		marker := orderedMarker(start+i, style, delim)
		if marker == "" {
			marker = orderedMarker(start, style, delim)
		}
		lines = append(lines, d.item(item.Children, marker+" ", strings.Repeat(" ", len(marker)+1))...)
	}
	return lines
}

// orderedMarker is the marker for item number num in a list of the given
// style and delimiter, or "" when the style cannot count that far
// (alphabetic past z).
func orderedMarker(num int, style listStyle, d delim) string {
	var number string
	switch style {
	case styleLowerAlpha, styleUpperAlpha:
		if num < 1 || num > 26 {
			return ""
		}
		c := byte('a' + num - 1)
		if style == styleUpperAlpha {
			c = byte('A' + num - 1)
		}
		number = string(rune(c))
	case styleLowerRoman:
		number = strings.ToLower(roman(num))
	case styleUpperRoman:
		number = roman(num)
	default:
		number = strconv.Itoa(num)
	}
	switch d {
	case delimParen:
		return number + ")"
	case delimParens:
		return "(" + number + ")"
	}
	return number + "."
}

func roman(num int) string {
	if num < 1 {
		return "I"
	}
	var b strings.Builder
	for _, step := range []struct {
		value  int
		symbol string
	}{{1000, "M"}, {900, "CM"}, {500, "D"}, {400, "CD"}, {100, "C"}, {90, "XC"}, {50, "L"}, {40, "XL"}, {10, "X"}, {9, "IX"}, {5, "V"}, {4, "IV"}, {1, "I"}} {
		for num >= step.value {
			b.WriteString(step.symbol)
			num -= step.value
		}
	}
	return b.String()
}

func (d *djotWriter) table(n *Node) []string {
	var lines []string
	var caption *Node
	first := true
	for _, row := range n.Children {
		if row.Tag == Caption {
			caption = row
			continue
		}
		if row.Tag != Row {
			continue
		}
		if first && !row.Head && hasAlignment(row) {
			lines = append(lines, separatorLine(row))
		}
		first = false
		cells := make([]string, len(row.Children))
		for i, c := range row.Children {
			cells[i] = d.inlines(c.Children)
		}
		lines = append(lines, "| "+strings.Join(cells, " | ")+" |")
		if row.Head {
			lines = append(lines, separatorLine(row))
		}
	}
	if len(lines) == 0 {
		lines = append(lines, "|---|")
	}
	if caption != nil {
		lines = append(lines, "")
		lines = append(lines, indented(strings.Split(d.inlines(caption.Children), "\n"), "^ ", "", "")...)
	}
	return lines
}

func hasAlignment(row *Node) bool {
	for _, c := range row.Children {
		if c.Align != "" && c.Align != AlignDefault {
			return true
		}
	}
	return false
}

func separatorLine(row *Node) string {
	cells := make([]string, len(row.Children))
	for i, c := range row.Children {
		switch c.Align {
		case AlignLeft:
			cells[i] = ":--"
		case AlignRight:
			cells[i] = "--:"
		case AlignCenter:
			cells[i] = ":-:"
		default:
			cells[i] = "---"
		}
	}
	return "|" + strings.Join(cells, "|") + "|"
}

// writeFootnotes writes the endnotes section back as definitions, numbered
// as the references are, without the generated backlinks.
func (d *djotWriter) footnotes(section *Node) []string {
	var lines []string
	for i, item := range section.Children {
		if i > 0 {
			lines = append(lines, "")
		}
		blocks := withoutBacklink(item.Children)
		marker := "[^" + strconv.Itoa(i+1) + "]: "
		if len(blocks) == 0 {
			lines = append(lines, marker)
			continue
		}
		lines = append(lines, indented(d.blocks(blocks), marker, "    ", "")...)
	}
	return lines
}

func withoutBacklink(blocks []*Node) []*Node {
	n := len(blocks)
	if n == 0 || blocks[n-1].Tag != Para {
		return blocks
	}
	last := blocks[n-1]
	kids := last.Children
	if k := len(kids); k > 0 && kids[k-1].Tag == Link && kids[k-1].Attr("role") == "doc-backlink" {
		kids = kids[:k-1]
	}
	if len(kids) == 0 {
		return blocks[:n-1]
	}
	out := append([]*Node{}, blocks[:n-1]...)
	return append(out, &Node{Tag: Para, Children: kids})
}

// attrSet writes a node's attributes as `{#id .c k="v"}`, or "" when it has
// none. Generated ids are left out.
func attrSet(n *Node) string {
	var parts []string
	if n.ID != "" && !n.AutoID {
		parts = append(parts, "#"+n.ID)
	}
	if n.Classes != "" && n.Tag != Div {
		for _, c := range strings.Fields(n.Classes) {
			parts = append(parts, "."+c)
		}
	}
	for _, a := range n.Attrs {
		if a.Key == "class" {
			for _, c := range strings.Fields(a.Value) {
				parts = append(parts, "."+c)
			}
			continue
		}
		parts = append(parts, a.Key+"=\""+attrValueEscaper.Replace(a.Value)+"\"")
	}
	if len(parts) == 0 {
		return ""
	}
	return "{" + strings.Join(parts, " ") + "}"
}

var attrValueEscaper = strings.NewReplacer("\\", "\\\\", "\"", "\\\"")

// --- Inlines ---

type inlineWriter struct {
	d         *djotWriter
	b         strings.Builder
	lineStart bool
	open      map[byte]int // plain-form delimiters currently open
	prevDash  bool         // the previous node was a dash
	// apostrophe is set once a ’ has been written as ' in this writer's
	// output, its inner writers' included; an enclosing SingleQuoted is then
	// braced, so that ' cannot close it.
	apostrophe bool
}

func (d *djotWriter) inlines(nodes []*Node) string {
	w := &inlineWriter{d: d, lineStart: true, open: map[byte]int{}}
	w.nodes(nodes)
	return w.b.String()
}

func (w *inlineWriter) write(s string) {
	if s == "" {
		return
	}
	w.b.WriteString(s)
	w.lineStart = s[len(s)-1] == '\n'
	w.prevDash = false
}

// prev is the last byte written, or 0 at the start.
func (w *inlineWriter) prev() byte {
	s := w.b.String()
	if s == "" {
		return 0
	}
	return s[len(s)-1]
}

func isText(n *Node) bool { return n.Tag == Str || n.Tag == Escape }

// nodes writes a sequence, coalescing adjacent text nodes so escaping sees
// the whole run (an escaped "." may have split "1." into two nodes). The run
// keeps where each ’ parsed from a ' stands, since the merged text cannot.
func (w *inlineWriter) nodes(nodes []*Node) {
	for i := 0; i < len(nodes); {
		n := nodes[i]
		j := i + 1
		var next *Node
		if isText(n) {
			var text strings.Builder
			var apos []int
			for j = i; j < len(nodes) && isText(nodes[j]); j++ {
				if t := nodes[j]; t.Text == apostrophe && t.Source == "'" {
					apos = append(apos, text.Len())
				}
				text.WriteString(nodes[j].Text)
			}
			if j < len(nodes) {
				next = nodes[j]
			}
			w.text(text.String(), apos, next)
			i = j
			continue
		}
		if j < len(nodes) {
			next = nodes[j]
		}
		w.node(n, next)
		i = j
	}
}

// delimited writes children between open and close delimiters. The plain
// form is used when the content does not start or end with whitespace and
// the opener cannot be read as a closer of an enclosing plain form; the
// braced form is used otherwise, or always when nested is set.
func (w *inlineWriter) delimited(n *Node, char byte, braceOnly, nestedBraces bool) {
	inner := &inlineWriter{d: w.d, lineStart: w.lineStart, open: map[byte]int{}}
	for k, v := range w.open {
		inner.open[k] = v
	}
	inner.open[char]++
	inner.nodes(n.Children)
	content := inner.b.String()
	w.apostrophe = w.apostrophe || inner.apostrophe
	// A ' written for an apostrophe would close a plain ' opener; the parser
	// matches a plain closer only against plain openers, so a braced quote
	// keeps it an apostrophe.
	braced := braceOnly || content == "" || char == '\'' && inner.apostrophe ||
		isInlineSpace(content[0]) || isInlineSpace(content[len(content)-1]) ||
		(w.open[char] > 0 && (nestedBraces || !isInlineSpace(w.prev()) && w.prev() != 0))
	d := string(rune(char))
	if braced {
		w.write("{" + d + content + d + "}")
	} else {
		w.write(d + content + d)
	}
	w.attrs(n)
}

// text writes a run of text; apos is as for escapeText.
func (w *inlineWriter) text(text string, apos []int, next *Node) {
	if w.lineStart && text != "" && isInlineSpace(text[0]) {
		w.write("{}") // the parser trims a line's leading whitespace
	}
	if w.prevDash && strings.HasPrefix(text, "-") {
		w.write("{}")
	}
	out, wrote := escapeText(text, w.lineStart, next, apos, w.b.String())
	w.apostrophe = w.apostrophe || wrote
	w.write(out)
}

func (w *inlineWriter) attrs(n *Node) {
	w.write(attrSet(n))
}

func (w *inlineWriter) node(n *Node, next *Node) {
	switch n.Tag {
	case Str, Escape:
		// nodes, node's only caller, writes text runs itself; this case is
		// kept so that a text node is never silently dropped.
		w.text(n.Text, nil, next)
	case SoftBreak:
		w.write("\n")
	case HardBreak:
		w.write("\\\n")
	case NonBreakingSpace:
		w.write("\\ ")
		if next == nil || next.Tag == SoftBreak || next.Tag == HardBreak {
			w.write("{}")
		}
	case Emph:
		w.delimited(n, '_', false, false)
	case Strong:
		w.delimited(n, '*', false, false)
	case Superscript:
		w.delimited(n, '^', false, true)
	case Subscript:
		w.delimited(n, '~', false, true)
	case Insert:
		w.delimited(n, '+', true, false)
	case Delete:
		w.delimited(n, '-', true, false)
	case Mark:
		w.delimited(n, '=', true, false)
	case DoubleQuoted:
		w.delimited(n, '"', false, false)
	case SingleQuoted:
		p := w.prev()
		plainOK := p == 0 || strings.IndexByte(" \t\n\r\"'-([", p) >= 0
		w.delimited(n, '\'', !plainOK, false)
	case Verbatim:
		w.write(verbatim(n.Text))
		w.attrs(n)
	case RawInline:
		w.write(verbatim(n.Text) + "{=" + n.Lang + "}")
	case InlineMath:
		w.write("$" + ticks(n.Text))
	case DisplayMath:
		w.write("$$" + ticks(n.Text))
	case Link, Image:
		if n.Tag == Image {
			w.write("![")
		} else {
			w.write("[")
		}
		w.bracketed(n.Children)
		w.write("]")
		switch {
		case n.Destination == nil:
			w.write("[" + n.Reference + "]")
		case n.Reference != "" && w.d.defineRef(normalizeLabel(n.Reference), *n.Destination):
			w.write("[" + n.Reference + "]")
		default:
			w.write("(" + strings.ReplaceAll(*n.Destination, "\\", "\\\\") + ")")
		}
		w.attrs(n)
	case Span:
		w.write("[")
		w.bracketed(n.Children)
		w.write("]")
		if a := attrSet(n); a != "" {
			w.write(a)
		} else {
			w.write("{}")
		}
	case URL, Email:
		w.write("<" + n.Text + ">")
	case FootnoteReference:
		w.write("[^" + n.Text + "]")
	case Symb:
		w.write(":" + n.Text + ":")
	case Ellipsis:
		w.write("...")
	case EmDash, EnDash:
		if w.prevDash {
			w.write("{}")
		}
		if n.Tag == EmDash {
			w.write("---")
		} else {
			w.write("--")
		}
		w.prevDash = true
	case LeftSingleQuote:
		w.write("\u2018")
	case RightSingleQuote:
		w.write("\u2019")
	case LeftDoubleQuote:
		w.write("\u201c")
	case RightDoubleQuote:
		w.write("\u201d")
	default:
		w.nodes(n.Children)
	}
}

// bracketed writes link, image and span content with the enclosing plain
// delimiters forgotten, since brackets reset the opener stack's reach. A
// smart-quote closer inside still reaches past them, so an apostrophe
// written inside is reported to the enclosing writer (see delimited).
func (w *inlineWriter) bracketed(children []*Node) {
	inner := &inlineWriter{d: w.d, lineStart: w.lineStart, open: map[byte]int{}}
	inner.nodes(children)
	w.apostrophe = w.apostrophe || inner.apostrophe
	w.write(inner.b.String())
}

// ticks wraps text in a backtick run longer than any run inside it.
func ticks(text string) string {
	longest := 0
	for i := 0; i < len(text); {
		if text[i] != '`' {
			i++
			continue
		}
		run := countRunAt(text, i, '`')
		longest = max(longest, run)
		i += run
	}
	t := strings.Repeat("`", longest+1)
	return t + text + t
}

// verbatim is ticks with a space of padding where the content touches the
// fence with a backtick, which the parser strips again. Empty content can
// only be written as an unclosed trailing backtick.
func verbatim(text string) string {
	if text == "" {
		return "`"
	}
	if text[0] == '`' {
		text = " " + text
	}
	if text[len(text)-1] == '`' {
		text += " "
	}
	return ticks(text)
}

const alwaysEscaped = "\\`*_{}[]<~^$\"'|"

func isSymbolChar(c byte) bool {
	return isAlphanumeric(c) || c == '-' || c == '_' || c == '+'
}

// escapeText backslash-escapes the bytes of a text node that could otherwise
// read as markup in context. An escape splits the text node on re-parse, so
// characters are escaped only where they could combine into markup.
//
// apos holds the offsets, in ascending order, of each ’ in text that was
// parsed from a ' (Node.Source); before is what was written ahead of text.
// Such a ’ is written back as ' when the character before it allows (see
// apostropheAfter), so it reads back with that source: a heading id reads
// "Don't", not "Don’t". A ’ typed as the glyph is always written as the
// glyph. The result reports whether a ' was written so.
func escapeText(text string, lineStart bool, next *Node, apos []int, before string) (string, bool) {
	var b strings.Builder
	wrote := false
	markerDelim := -1
	if lineStart {
		if m, ok := parseOrderedMarker(text); ok {
			// the "." or ")" that ends the marker
			markerDelim = countIndent(text) + len(m.markerText)
			if m.delim == delimParens {
				markerDelim++
			}
		}
	}
	nextDash := next != nil && (next.Tag == EmDash || next.Tag == EnDash)
	nextDots := next != nil && next.Tag == Ellipsis
	last := len(text) - 1
	for i := 0; i < len(text); i++ {
		if len(apos) > 0 && apos[0] == i {
			apos = apos[1:]
			prev, _ := utf8.DecodeLastRuneInString(before)
			if i > 0 {
				prev, _ = utf8.DecodeLastRuneInString(text[:i])
			}
			if apostropheAfter(prev) {
				b.WriteByte('\'')
				wrote = true
				i += len(apostrophe) - 1
				continue
			}
		}
		c := text[i]
		escape := strings.IndexByte(alwaysEscaped, c) >= 0 || i == markerDelim
		switch c {
		case '-':
			escape = escape || (lineStart && i == 0) || (i < last && text[i+1] == '-') ||
				(i > 0 && text[i-1] == '-') || (i == last && nextDash)
		case '.':
			escape = escape || (i < last && text[i+1] == '.') || (i > 0 && text[i-1] == '.') ||
				(i == last && nextDots)
		case ':':
			// A symbol needs a name and a closing colon.
			j := i + 1
			for j < len(text) && isSymbolChar(text[j]) {
				j++
			}
			escape = j > i+1 && j < len(text) && text[j] == ':'
		case '#', '>', '+':
			escape = escape || (lineStart && i == 0)
		}
		if escape {
			b.WriteByte('\\')
		}
		b.WriteByte(c)
	}
	return b.String(), wrote
}

// apostrophe is the glyph an apostrophe renders as.
const apostrophe = "’"

// apostropheAfter reports whether a ' written after prev, a letter, a digit
// or _, reads back as an apostrophe. It cannot open: canOpenSingleQuote
// allows none of them before it. As a plain closer it matches only a plain '
// opener: text ' is always escaped, so the only plain ' openers are
// SingleQuoted delimiters, and delimited braces every SingleQuoted that
// encloses such a ', through brackets too (inlineWriter.apostrophe). In
// escapeText's output the character before the ' is prev itself (a letter
// or digit is never escaped; _ is written \_), or the ' or ’ just written,
// which does not qualify.
func apostropheAfter(prev rune) bool {
	return unicode.IsLetter(prev) || unicode.IsDigit(prev) || prev == '_'
}
