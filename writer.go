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
		return append(lines, d.leafLines(n.Children)...)
	case Heading:
		hashes := strings.Repeat("#", n.Level)
		if len(n.Children) == 0 {
			return append(lines, hashes)
		}
		return append(lines, indented(d.leafLines(n.Children), hashes+" ", hashes+" ", "")...)
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
	// A caption right under the rows re-parses as the table's, and keeps a
	// list the table ends tight; its block attributes stand before it, as
	// any block's do. One with neither content nor attributes is not written.
	if caption != nil {
		a := attrSet(caption)
		if a != "" {
			lines = append(lines, a)
		}
		if a != "" || len(caption.Children) > 0 {
			lines = append(lines, indented(d.leafLines(caption.Children), "^ ", "", "")...)
		}
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
	// open counts, per delimiter, the openers a plain closer could close:
	// the plain forms being written and the delimiters written raw in text
	// that the parser reads as openers. openMarked counts the same for
	// marked openers ({_ and its kind), which only X} closes.
	open       map[byte]int
	openMarked map[byte]int
	prevDash   bool // the previous node was a dash
	// apostrophe is set once a ’ has been written as ' in this writer's
	// output, its inner writers' included; an enclosing SingleQuoted is then
	// braced, so that ' cannot close it.
	apostrophe bool
}

// leafLines is the lines of a paragraph, heading or caption, whose end the
// parser trims: a trailing soft break or space gets an empty attribute
// after it, so the break does not read as a blank line and the space is
// not trimmed, and a trailing hard break is written as its backslash alone,
// which ends the leaf.
func (d *djotWriter) leafLines(nodes []*Node) []string {
	text := d.inlines(nodes)
	switch {
	case len(nodes) > 0 && nodes[len(nodes)-1].Tag == HardBreak:
		text = strings.TrimSuffix(text, "\n")
	case strings.HasSuffix(text, "\n"), strings.HasSuffix(text, " "):
		text += "{}"
	}
	return strings.Split(text, "\n")
}

func (d *djotWriter) inlines(nodes []*Node) string {
	w := newInlineWriter(d, true)
	w.nodes(nodes)
	return w.b.String()
}

func newInlineWriter(d *djotWriter, lineStart bool) *inlineWriter {
	return &inlineWriter{d: d, lineStart: lineStart, open: map[byte]int{}, openMarked: map[byte]int{}}
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
// keeps where each ’ parsed from a ' stands, since the merged text cannot,
// and where each node after the first starts: the parser joins all the text
// it can, so adjacent text nodes are a boundary it made (an escape, an
// unclosed marked opener, an empty attribute, a smart quote's glyph) and
// the written text keeps it.
func (w *inlineWriter) nodes(nodes []*Node) {
	for i := 0; i < len(nodes); {
		n := nodes[i]
		j := i + 1
		var next *Node
		if isText(n) {
			var text strings.Builder
			var apos, bounds []int
			for j = i; j < len(nodes) && isText(nodes[j]); j++ {
				t := nodes[j]
				if t.Text == apostrophe && t.Source == "'" {
					apos = append(apos, text.Len())
				}
				if j > i && text.Len() > 0 {
					bounds = append(bounds, text.Len())
				}
				text.WriteString(t.Text)
			}
			if j < len(nodes) {
				next = nodes[j]
			}
			w.text(text.String(), apos, bounds, next)
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
// braced form is used otherwise, or always when nested is set. An empty
// pair closes nothing, so empty content is written as an empty attribute,
// which makes no node and keeps the delimiters apart.
func (w *inlineWriter) delimited(n *Node, char byte, braceOnly, nestedBraces bool) {
	inner := w.content(n, char, false)
	content := inner.b.String()
	if content == "" {
		content = "{}"
	}
	// A ' written for an apostrophe would close a plain ' opener; the parser
	// matches a plain closer only against plain openers, so a braced quote
	// keeps it an apostrophe.
	braced := braceOnly || char == '\'' && inner.apostrophe ||
		isInlineSpace(content[0]) || isInlineSpace(content[len(content)-1]) ||
		(w.open[char] > 0 && (nestedBraces || !isInlineSpace(w.prev()) && w.prev() != 0))
	if braced && strings.Contains(content, string(rune(char))+"}") {
		// Written for the plain form, where X} is text; in the braced form it
		// would close the pair, so the content is written again for that form.
		inner = w.content(n, char, true)
		content = inner.b.String()
	}
	w.apostrophe = w.apostrophe || inner.apostrophe
	d := string(rune(char))
	if braced {
		w.write("{" + d + content + d + "}")
	} else {
		w.write(d + content + d)
	}
	w.attrs(n)
}

// text writes a run of text; apos and bounds are as for escapeText.
func (w *inlineWriter) text(text string, apos, bounds []int, next *Node) {
	if w.lineStart && text != "" && isInlineSpace(text[0]) {
		w.write("{}") // the parser trims a line's leading whitespace
	}
	if w.prevDash && strings.HasPrefix(text, "-") {
		w.write("{}")
	}
	out, wrote := w.escapeText(text, next, apos, bounds)
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
		w.text(n.Text, nil, nil, next)
	case SoftBreak:
		w.write("\n")
	case HardBreak:
		if isInlineSpace(w.prev()) {
			w.write("{}") // the parser trims the blanks before a hard break
		}
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

// content writes n's children for a pair of char around them, in the plain
// form or the braced one, with the enclosing delimiters remembered.
func (w *inlineWriter) content(n *Node, char byte, braced bool) *inlineWriter {
	inner := newInlineWriter(w.d, w.lineStart)
	for k, v := range w.open {
		inner.open[k] = v
	}
	for k, v := range w.openMarked {
		inner.openMarked[k] = v
	}
	inner.open[char]++
	if braced {
		inner.openMarked[char]++
	}
	inner.nodes(n.Children)
	return inner
}

// bracketed writes link, image and span content with the enclosing plain
// delimiters forgotten, since brackets reset the opener stack's reach. A
// smart-quote closer inside still reaches past them, so an apostrophe
// written inside is reported to the enclosing writer (see delimited).
func (w *inlineWriter) bracketed(children []*Node) {
	inner := newInlineWriter(w.d, w.lineStart)
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

// alwaysEscaped is the bytes that start markup wherever they stand at a
// text run's start; inside a run each is escaped only where its raw form
// would read as markup (escapeText).
const alwaysEscaped = "\\`*_{}[]<~^$\"'|"

// markedDelims is the bytes a brace makes a marked opener of ({_) and a
// closing brace a marked closer of (_}).
const markedDelims = "_*~^+-='\""

func isSymbolChar(c byte) bool {
	return isAlphanumeric(c) || c == '-' || c == '_' || c == '+'
}

// escapeText backslash-escapes the bytes of a text run that would otherwise
// read back as markup. An escaped byte starts a new text node when read
// back, so a byte is escaped only where it must be: at the run's start
// (where a node starts anyway) and where its raw form would read as
// markup, which for a delimiter means closing an opener that is open
// (open, openMarked) or pairing within the run, for a brace opening an
// attribute or a smart quote, for a bracket closing into a link within the
// run, and for a backslash escaping or breaking what follows it. A byte
// that reads as text where it stands is written raw, so a text node the
// parser joined reads back as one node. A delimiter written raw that reads
// as an opener is counted in open, so that the delimiters after it are
// escaped where they would close it. Other markup is contextual: a dash is
// escaped where it would join another into an en or em dash (an adjacent
// dash in the text, or a dash node following), a dot where it would join
// into an ellipsis, a colon where it would start a symbol, and at the
// start of a line the first "-", "#", ">", "+" and the delimiter of what
// would read as an ordered list marker. apos lists the offsets of ’ that
// parsed from a ' and are written as ' again where that reads back as an
// apostrophe (apostropheAfter); wrote reports whether one was. bounds lists
// the offsets where a text node after the first starts: the byte there is
// escaped, which starts a node when read back, or where it cannot be an
// empty attribute goes before it, which makes no node and ends the one
// before; a ’ written as ' reads back as a node of its own, so the boundary
// after it needs neither.
func (w *inlineWriter) escapeText(text string, next *Node, apos, bounds []int) (string, bool) {
	var b strings.Builder
	wrote := false
	markerDelim := -1
	if w.lineStart {
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
	// openerAt is one past the run offset of the last raw delimiter of each
	// kind that reads as an opener: a closer right after it would make an
	// empty pair, which closes nothing.
	openerAt := map[byte]int{}
	quote := false // the byte before was a ’ written as '
	for i := 0; i < len(text); i++ {
		bound := len(bounds) > 0 && bounds[0] == i
		if bound {
			bounds = bounds[1:]
		}
		if len(apos) > 0 && apos[0] == i {
			apos = apos[1:]
			prev, _ := utf8.DecodeLastRuneInString(w.b.String())
			if i > 0 {
				prev, _ = utf8.DecodeLastRuneInString(text[:i])
			}
			if apostropheAfter(prev) {
				b.WriteByte('\'')
				wrote = true
				quote = true
				i += len(apostrophe) - 1
				continue
			}
		}
		c := text[i]
		escape := i == markerDelim || (i == 0 || bound) && strings.IndexByte(alwaysEscaped, c) >= 0
		if bound && !escape && !quote {
			b.WriteString("{}")
		}
		quote = false
		switch c {
		case '`', '"', '\'', '|':
			// No reading of the raw byte is text.
			escape = true
		case '\\':
			escape = escape || !backslashIsText(text, i)
		case '*', '_', '~', '^':
			if i < last && text[i+1] == '}' && w.openMarked[c] == 0 {
				break // X} with no marked opener to close: text
			}
			closes := i > 0 && !isInlineSpace(text[i-1]) && w.open[c] > 0 && openerAt[c] != i
			escape = escape || closes
			if !escape && delimiterOpens(text, i, next) {
				w.open[c]++
				openerAt[c] = i + 1
			}
		case '{':
			escape = escape || braceOpensMarkup(text, i, next)
			if !escape && i < last && strings.IndexByte(markedDelims, text[i+1]) >= 0 {
				w.openMarked[text[i+1]]++
			}
		case '}':
			escape = escape || i > 0 && strings.IndexByte(markedDelims, text[i-1]) >= 0 && w.openMarked[text[i-1]] > 0
		case '[':
			escape = escape || bracketCloses(text, i)
		case '<':
			_, _, ok := autolinkAt(text, i)
			escape = escape || ok
		case '$':
			escape = escape || i < last && text[i+1] == '`'
		case '-':
			escape = escape || (w.lineStart && i == 0) || (i < last && text[i+1] == '-') ||
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
			escape = escape || (w.lineStart && i == 0)
		}
		if escape {
			b.WriteByte('\\')
		}
		b.WriteByte(c)
	}
	return b.String(), wrote
}

// backslashIsText reports whether the backslash at i reads as text: the
// byte after it is not escapable and not a space (a non-breaking space),
// and a tab after it is followed by more text in the run (blanks to the
// run's end could run on to a line end, a hard break).
func backslashIsText(text string, i int) bool {
	if i+1 >= len(text) {
		return false
	}
	switch next := text[i+1]; {
	case isEscapable(next), next == ' ':
		return false
	case next == '\t':
		j := i + 1
		for j < len(text) && (text[j] == ' ' || text[j] == '\t') {
			j++
		}
		return j < len(text)
	}
	return true
}

// delimiterOpens reports whether the raw delimiter at i reads as an opener:
// a non-blank follows it, in the run or as the next node written (a soft
// break is a newline).
func delimiterOpens(text string, i int, next *Node) bool {
	if i+1 < len(text) {
		return !isInlineSpace(text[i+1])
	}
	return next != nil && next.Tag != SoftBreak
}

// braceOpensMarkup reports whether the raw brace at i reads as markup: an
// inline attribute (an empty one too), a marked smart quote's opener, or a
// marked opener whose closer follows in the run. A marked opener with no
// closer is text in a node of its own, as the parser gives it. A brace
// ending the run before another node could pair with that node's first
// byte, so it is escaped.
func braceOpensMarkup(text string, i int, next *Node) bool {
	if i+1 >= len(text) {
		return next != nil
	}
	n := text[i+1]
	switch {
	case n == '\'' || n == '"':
		return true
	case strings.IndexByte(markedDelims, n) >= 0:
		return strings.Contains(text[i+2:], string(n)+"}")
	}
	_, _, ok := parseInlineAttrs(text, i)
	return ok
}

// bracketCloses reports whether the raw bracket at i would close into a
// link, image or span within the run: a "]" followed by "(", "[" or "{"
// comes later in it.
func bracketCloses(text string, i int) bool {
	for j := i + 1; j+1 < len(text); j++ {
		if text[j] == ']' && strings.IndexByte("([{", text[j+1]) >= 0 {
			return true
		}
	}
	return false
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
