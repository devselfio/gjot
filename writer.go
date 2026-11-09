package gjot

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Djot writes doc back as Djot source. The result re-parses to the same
// HTML and AST (positions aside); it is not the original text. Reference
// definitions are written as inline destinations and footnote definitions
// with their labels, where the ids their headings generated demand it
// (due), else after the document.
func Djot(doc *Node) []byte {
	d := &djotWriter{ids: newIDs(), claims: map[*Node]*idClaims{}}
	var body []*Node
	for _, b := range doc.Children {
		if b.Tag == Footnote {
			d.defs = append(d.defs, b)
		} else {
			body = append(body, b)
		}
	}
	d.placed = make([]bool, len(d.defs))
	d.bound = len(d.defs)
	lines := joinBlocks(d.blocks(body), d.place(len(d.defs)-1, nil))
	for _, def := range d.refDefs {
		lines = append(lines, "", "["+def.label+"]: "+def.url)
	}
	if len(lines) == 0 {
		return nil
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

// djotWriter renders blocks and collects the reference definitions that
// reference links need, written at the end of the document. It rebuilds
// the document's id registry as the re-parse will read the written text
// (ids, see headingID), and places the footnote definitions where the ids
// their headings generated demand (due).
type djotWriter struct {
	refDefs []refDefOut
	defined map[string]bool
	// ids is the registry the written document builds as it is re-parsed:
	// every id is registered as it is written, a heading's generated again.
	ids ids
	// defs is the footnote definitions in the order they close, placed as
	// they are written (placed); bound is the index of the definition being
	// written, or len(defs) outside one, since a definition nests only in
	// one that closes after it. claims memoizes idClaims by node.
	defs   []*Node
	placed []bool
	bound  int
	claims map[*Node]*idClaims
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

// blocks renders sibling blocks as lines with one blank line between, the
// footnote definitions due before a block written before it.
func (d *djotWriter) blocks(blocks []*Node) []string {
	var lines []string
	for i, b := range blocks {
		if i > 0 {
			lines = append(lines, "")
		}
		if defs := d.due(b); len(defs) > 0 {
			lines = append(append(lines, defs...), "")
		}
		lines = append(lines, d.block(b)...)
	}
	return lines
}

// joinBlocks is the lines of two runs of blocks, a blank line between them
// when both have lines.
func joinBlocks(a, b []string) []string {
	if len(a) > 0 && len(b) > 0 {
		a = append(a, "")
	}
	return append(a, b...)
}

// attrLines is a block's attribute line, none when it has no attributes,
// its id registered where the line stands.
func (d *djotWriter) attrLines(n *Node) []string {
	a := attrSet(n)
	if a == "" {
		return nil
	}
	d.ids.register(explicitID(n))
	return []string{a}
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
	if n.Tag == Section {
		if len(n.Children) > 0 && n.Children[0].Tag == Heading {
			return d.blocks(unsectioned(n))
		}
		return append(d.attrLines(n), d.blocks(n.Children)...)
	}
	lines := d.attrLines(n)
	switch n.Tag {
	case Footnote:
		return append(lines, d.footnote(n, d.blocks(n.Children))...)
	case OrderedList:
		return append(lines, d.orderedList(n)...)
	case Para:
		return append(lines, d.leafLines(n.Children, "")...)
	case Heading:
		hashes := strings.Repeat("#", n.Level)
		carriers, explicit := d.headingID(n)
		if explicit {
			e := *n
			e.AutoID = false
			lines = d.attrLines(&e)
		}
		if len(n.Children) == 0 && carriers == "" {
			return append(lines, hashes)
		}
		return append(lines, indented(d.leafLines(n.Children, carriers), hashes+" ", hashes+" ", "")...)
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
		if class := divFenceClass(n); class != "" {
			open += " " + class
		}
		lines = append(lines, open)
		lines = append(lines, inner...)
		return append(lines, fence)
	case BulletList, TaskList:
		marker := n.Style
		if marker == "" {
			marker = "-"
		}
		return append(lines, d.listItems(n, func(_ int, item *Node) (string, string) {
			switch {
			case item.Tag != TaskListItem:
				return marker, "  "
			case item.Checked != nil && *item.Checked:
				return marker + " [x]", "  "
			}
			return marker + " [ ]", "  "
		})...)
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

// listItems renders a list's items, each under the marker and at the
// continuation indent markerOf gives it, with a blank line between items
// when the list is loose. A loose list of one item with at most one block
// has no gap to show it: the source had a block the AST does not hold (a
// reference or footnote definition, a dropped block attribute or caption)
// after a blank line in the item, so the item ends with `{}` after a blank
// line, which reads back as nothing and keeps the list loose.
func (d *djotWriter) listItems(n *Node, markerOf func(i int, item *Node) (marker, indent string)) []string {
	var lines []string
	for i, item := range n.Children {
		if i > 0 && !n.Tight {
			lines = append(lines, "")
		}
		marker, indent := markerOf(i, item)
		lines = append(lines, d.item(item.Children, marker, indent)...)
		if !n.Tight && len(n.Children) == 1 && len(item.Children) <= 1 {
			lines = append(lines, "", indent+"{}")
		}
	}
	return lines
}

// item renders a list item's blocks under its marker, the first block on
// the marker's line and the rest indented by rest. A thematic break goes
// under a bare marker instead: the marker and `* * *` on one line read as a
// thematic break.
func (d *djotWriter) item(blocks []*Node, marker, rest string) []string {
	inner := d.blocks(blocks)
	if len(inner) == 0 {
		return []string{marker}
	}
	if blocks[0].Tag == ThematicBreak {
		return append([]string{marker}, indented(inner, rest, rest, "")...)
	}
	return indented(inner, marker+" ", rest, "")
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
	return d.listItems(n, func(i int, _ *Node) (string, string) {
		marker := orderedMarker(start+i, style, delim)
		if marker == "" {
			marker = orderedMarker(start, style, delim)
		}
		return marker, strings.Repeat(" ", len(marker)+1)
	})
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
		a := d.attrLines(caption)
		lines = append(lines, a...)
		if len(a) > 0 || len(caption.Children) > 0 {
			lines = append(lines, indented(d.leafLines(caption.Children, ""), "^ ", "", "")...)
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

// footnote writes a footnote definition's lines under its label, the
// content indented by the marker's width so a code block keeps its
// indentation relative to its fence (the footnote's lines need only be
// indented past the label's).
func (d *djotWriter) footnote(n *Node, inner []string) []string {
	marker := "[^" + n.Text + "]:"
	if len(inner) == 0 {
		return []string{marker}
	}
	return indented(inner, marker+" ", strings.Repeat(" ", len(marker)+1), "")
}

// unsectioned is a section's blocks with its heading given back the id and
// attributes the document moved onto the section (document.sections), so
// the heading writes them and registers them where it stands.
func unsectioned(sec *Node) []*Node {
	h := *sec.Children[0]
	h.ID, h.AutoID = sec.ID, sec.AutoID
	h.Attrs = append(append([]Attr{}, sec.Attrs...), h.Attrs...)
	return append([]*Node{&h}, sec.Children[1:]...)
}

// explicitID is the id a node's attribute set writes, "" when none: the
// written one, else the id the document gave the node when not generated.
func explicitID(n *Node) string {
	if id, written := n.writtenID(); written {
		return id
	}
	if !n.AutoID {
		return n.ID
	}
	return ""
}

// --- Ids and footnote definitions ---
//
// djot.js registers every id as it reads it, and a heading's generated id
// avoids every id registered before it; so the written document must
// register, before each heading, what the source registered before it. The
// tree does not hold every such id (one a later attribute overrode, one on
// an attribute dropped with a blank line or a container's end, a term's, a
// replaced or dropped caption's, one attached to nothing), and holds the
// footnote definitions after the body, wherever they stood: headingID
// carries the missing ids on the heading, and due writes each definition
// where its headings' ids say it stood.

// headingID settles n's id in the registry as the re-parse will: a written
// id was registered with its attribute line; a generated one is generated
// again, after the carriers returned here register what the source
// registered before the heading and the tree does not hold: inline
// attributes attached to nothing, written at the heading's start, one per
// id between the registry's state and the heading's. The heading's own
// inlines register theirs after this, where the re-parse reads them before
// the heading's: none can be the heading's id, which was generated after
// them, so the carriers come out the same, at most one of them redundant.
// When the id cannot be generated any more (a tree built by hand that
// numbers headings out of order) explicit is set and the caller writes the
// id on the heading.
func (d *djotWriter) headingID(n *Node) (carriers string, explicit bool) {
	text := stringContent(n.Children, false)
	if n.ID == "" {
		d.ids.generate(text)
		return "", false
	}
	if !n.AutoID {
		return "", false
	}
	g, ok := idTarget(text, n.ID)
	if !ok || d.ids.used[n.ID] || g.k > 0 && d.ids.nextSuffix[g.base] > g.k {
		return "", true
	}
	for {
		c := d.ids.generate(text)
		if c == n.ID {
			return carriers, false
		}
		carriers += "{" + attrPart("id", c) + "}"
	}
}

// genID is a generated id as idTarget reads it: its base (idBase of the
// heading's text, "s" when that is empty) and its suffix, 0 for the base
// itself.
type genID struct {
	id, base string
	k        int
	empty    bool
}

// idTarget reads id as an id generate makes of text; ok is false when it
// is not one (a tree built by hand).
func idTarget(text, id string) (g genID, ok bool) {
	g = genID{id: id, base: idBase(text)}
	if g.empty = g.base == ""; g.empty {
		g.base = "s"
	}
	if id == g.base {
		return g, !g.empty
	}
	rest, found := strings.CutPrefix(id, g.base+"-")
	if !found {
		return g, false
	}
	g.k, _ = strconv.Atoi(rest)
	return g, g.k >= 1 && strconv.Itoa(g.k) == rest
}

// covers reports whether generating g needed id taken: its base (which an
// empty text never has), and every suffix below its own.
func (g genID) covers(id string) bool {
	if id == g.base {
		return g.k > 0 && !g.empty
	}
	rest, found := strings.CutPrefix(id, g.base+"-")
	if !found {
		return false
	}
	i, err := strconv.Atoi(rest)
	return err == nil && i >= 1 && i < g.k && strconv.Itoa(i) == rest
}

// idClaims is what a subtree registers as it is re-parsed: the ids its
// headings generate and the ids written on its nodes.
type idClaims struct {
	generated []genID
	explicit  map[string]bool
}

func (d *djotWriter) claimsOf(n *Node) *idClaims {
	if c, ok := d.claims[n]; ok {
		return c
	}
	c := &idClaims{explicit: map[string]bool{}}
	c.collect(n)
	d.claims[n] = c
	return c
}

func (c *idClaims) collect(n *Node) {
	if n.Tag == Section && len(n.Children) > 0 && n.Children[0].Tag == Heading {
		for _, b := range unsectioned(n) {
			c.collect(b)
		}
		return
	}
	if n.Tag == Heading && n.AutoID && n.ID != "" {
		if g, ok := idTarget(stringContent(n.Children, false), n.ID); ok {
			c.generated = append(c.generated, g)
		}
	}
	if id := explicitID(n); id != "" {
		c.explicit[id] = true
	}
	for _, ch := range n.Children {
		c.collect(ch)
	}
}

// before reports whether an id a's headings generate must be registered
// before b is re-parsed: b writes it, or a heading of b generated an id
// that needed it taken.
func (d *djotWriter) before(a, b *Node) bool {
	ca, cb := d.claimsOf(a), d.claimsOf(b)
	for _, g := range ca.generated {
		if cb.explicit[g.id] {
			return true
		}
		for _, h := range cb.generated {
			if h.covers(g.id) {
				return true
			}
		}
	}
	return false
}

// due is the footnote definitions written before next: the unplaced ones
// that must stand before it (before), every definition closing before one
// of those (the definitions are in closing order, which the re-parse must
// see again), and every later definition one of those must follow, which
// it then nests in.
func (d *djotWriter) due(next *Node) []string {
	k := -1
	for i := 0; i < d.bound; i++ {
		if !d.placed[i] && d.before(d.defs[i], next) {
			k = i
		}
	}
	for grown := true; grown; {
		grown = false
		for i := 0; i <= k; i++ {
			for j := k + 1; j < d.bound && !d.placed[i]; j++ {
				if !d.placed[j] && d.before(d.defs[j], d.defs[i]) {
					k, grown = j, true
				}
			}
		}
	}
	return d.place(k, next)
}

// place writes the unplaced definitions up to index k in their order, but
// for one that must follow next (it stands inside next, as do those after
// it) and for one that must follow a later definition up to k (it nests
// there, written at its end at the latest).
func (d *djotWriter) place(k int, next *Node) []string {
	var lines []string
	for i := 0; i <= k; i++ {
		if d.placed[i] {
			continue
		}
		if next != nil && d.before(next, d.defs[i]) {
			break
		}
		nested := false
		for j := i + 1; j <= k && !nested; j++ {
			nested = !d.placed[j] && d.before(d.defs[j], d.defs[i])
		}
		if !nested {
			lines = joinBlocks(lines, d.definition(i))
		}
	}
	return lines
}

// definition writes footnote definition i, with the unplaced definitions
// that close before it nested at its end.
func (d *djotWriter) definition(i int) []string {
	n := d.defs[i]
	d.placed[i] = true
	outer := d.bound
	d.bound = i
	inner := joinBlocks(d.blocks(n.Children), d.place(i-1, nil))
	d.bound = outer
	return append(d.attrLines(n), d.footnote(n, inner)...)
}

// attrSet writes a node's attributes as `{#id .c k="v"}`, or "" when it has
// none: the attributes in their order, an id the document gave the node and
// no attribute wrote first; a generated id is left out, as is the class a
// div's fence carries. An id or class the short form cannot spell, an empty
// id among them, is written as its key.
func attrSet(n *Node) string {
	attrs := n.Attrs
	if n.Tag == Div {
		attrs = withoutLastClass(attrs, divFenceClass(n))
	}
	var parts []string
	if n.ID != "" && !n.AutoID && !hasKey(attrs, "id") {
		parts = append(parts, attrPart("id", n.ID))
	}
	for _, a := range attrs {
		parts = append(parts, attrPart(a.Key, a.Value))
	}
	if len(parts) == 0 {
		return ""
	}
	return "{" + strings.Join(parts, " ") + "}"
}

// attrPart is one attribute in its short form (`#id`, `.a .b`) when the
// parser reads that form back to the same value, else `key="value"`.
func attrPart(key, value string) string {
	switch key {
	case "id":
		if value != "" && allBytes(value, isIDChar) {
			return "#" + value
		}
	case "class":
		words := strings.Fields(value)
		if len(words) > 0 && strings.Join(words, " ") == value && allWords(words, isAttrName) {
			return "." + strings.Join(words, " .")
		}
	}
	return key + "=\"" + attrValueEscaper.Replace(value) + "\""
}

func allBytes(s string, ok func(byte) bool) bool {
	for i := 0; i < len(s); i++ {
		if !ok(s[i]) {
			return false
		}
	}
	return true
}

func allWords(words []string, ok func(byte) bool) bool {
	for _, w := range words {
		if !allBytes(w, ok) {
			return false
		}
	}
	return true
}

// divFenceClass is the class a div's fence carries: the last of its
// classes, when it is one the fence may name and the class entry is the
// last attribute, since the fence class joins after the block attribute
// before it (`{.x}` then `::: y` reads back as `class="x y"`; `{.y k=v}`
// then `:::` keeps its order only with `.y` in the attribute line).
func divFenceClass(n *Node) string {
	attrs := n.Attrs
	if len(attrs) == 0 || attrs[len(attrs)-1].Key != "class" {
		return ""
	}
	words := strings.Fields(attrs[len(attrs)-1].Value)
	if len(words) == 0 || !divClass(words[len(words)-1]) {
		return ""
	}
	return words[len(words)-1]
}

// withoutLastClass is attrs with the class divFenceClass writes on the
// fence taken off; the class entry goes when that was its only class.
func withoutLastClass(attrs []Attr, fenceClass string) []Attr {
	if fenceClass == "" {
		return attrs
	}
	out := append([]Attr{}, attrs[:len(attrs)-1]...)
	last := attrs[len(attrs)-1]
	if words := strings.Fields(last.Value); len(words) > 1 {
		out = append(out, Attr{"class", strings.Join(words[:len(words)-1], " ")})
	}
	return out
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
	// closer is what follows this writer's output: the enclosing pair's
	// delimiter or the bracket's "]"; "" at a leaf's end. inBrackets is
	// set inside link, image and span text, where a "]" is escaped.
	closer     string
	inBrackets bool
	prevDash   bool // the previous node was a dash
	// rawQuote is set, per quote, once one has been written raw in this
	// writer's output or its inner writers' where a plain closer reaches
	// it: a ’ written as ' (an apostrophe) or a lone quote that opens
	// (loneQuote). An enclosing plain pair of that quote is then braced, so
	// that the apostrophe does not close it and its closer does not close
	// the lone quote instead.
	rawQuote map[byte]bool
}

// leafLines is the lines of a paragraph, heading or caption, whose end the
// parser trims: a trailing soft break or space gets an empty attribute
// after it, so the break does not read as a blank line and the space is
// not trimmed, and a trailing hard break is written as its backslash alone,
// which ends the leaf. prefix opens the leaf (a heading's id carriers),
// written where the inlines see it: a quote after it is not at a line's start.
func (d *djotWriter) leafLines(nodes []*Node, prefix string) []string {
	w := newInlineWriter(d, true)
	w.write(prefix)
	w.nodes(nodes)
	text := w.b.String()
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
	return &inlineWriter{d: d, lineStart: lineStart, open: map[byte]int{}, openMarked: map[byte]int{}, rawQuote: map[byte]bool{}}
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

func isText(n *Node) bool { return n.Tag == Str }

// nodes writes a sequence, coalescing adjacent text nodes so escaping sees
// the whole run (an escaped "." may have split "1." into two nodes). The run
// keeps where each ’ parsed from a ' stands, since the merged text cannot,
// and where each node after the first starts: the parser joins all the text
// it can, so adjacent text nodes are a boundary it made (an escape, an
// unclosed marked opener, an empty attribute, a smart quote's glyph) and
// the written text keeps it. A smart quote carrying attributes is not text
// but a quote (loneQuote).
func (w *inlineWriter) nodes(nodes []*Node) {
	for i := 0; i < len(nodes); {
		n := nodes[i]
		j := i + 1
		var next *Node
		if isText(n) && loneQuote(n) == "" {
			var text strings.Builder
			var apos, bounds []int
			for j = i; j < len(nodes) && isText(nodes[j]) && loneQuote(nodes[j]) == ""; j++ {
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
	// A ' written for an apostrophe would close a plain ' opener, and a
	// plain closer would close a lone quote of its kind written inside
	// (loneQuote) instead of the pair; the parser matches a plain closer
	// only against plain openers, so the braced form keeps the pair.
	braced := braceOnly || inner.rawQuote[char] ||
		isInlineSpace(content[0]) || isInlineSpace(content[len(content)-1]) ||
		(w.open[char] > 0 && (nestedBraces || !isInlineSpace(w.prev()) && w.prev() != 0))
	if braced && (strings.Contains(content, string(rune(char))+"}") || strings.Contains(content, "{"+string(rune(char)))) {
		// Written for the plain form, where X} is text and {X opens a pair of
		// its own; in the braced form they would close the pair or take its
		// closer, so the content is written again for that form.
		inner = w.content(n, char, true)
		content = inner.b.String()
	}
	w.adopt(inner)
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
	leadingBlanks := w.lineStart && text != "" && isInlineSpace(text[0])
	if leadingBlanks {
		w.write("{}") // the parser trims a line's leading whitespace
	}
	if w.prevDash && strings.HasPrefix(text, "-") {
		w.write("{}")
	}
	out, wrote := w.escapeText(text, next, apos, bounds)
	if wrote {
		w.rawQuote['\''] = true
	}
	w.write(out)
	// A line of blanks alone would read back as an empty attribute block.
	if leadingBlanks && strings.TrimLeft(text, " \t") == "" && (next == nil || next.Tag == SoftBreak) {
		w.write("{}")
	}
}

// loneQuote is how a smart quote carrying attributes is written: a spelling
// that reads back as that quote's glyph paired with nothing, which its
// attributes then attach to (as the glyph in text they would not: a text
// node's last word takes them). A right quote is a marked closer with no
// marked opener to close, a left one an opener that pairs with nothing
// (the plain " and the marked {'). "" for any other node.
func loneQuote(n *Node) string {
	if !isSmartQuote(n) || attrSet(n) == "" {
		return ""
	}
	switch n.Text {
	case "\u201c":
		return "\""
	case "\u201d":
		return "\"}"
	case "\u2018":
		return "{'"
	default:
		return "'}"
	}
}

// quote writes a smart quote carrying attributes (loneQuote) and its
// attributes. The plain opener is counted in open, so that a later plain
// pair of the quote is braced where its opener would read as this one's
// closer, and in rawQuote, so that an enclosing plain pair is braced too;
// the marked opener is counted in openMarked. A right quote has one
// spelling, which inside a braced pair of its quote closes that pair.
func (w *inlineWriter) quote(n *Node) {
	spelling := loneQuote(n)
	w.write(spelling)
	w.attrs(n)
	switch spelling {
	case "\"":
		w.open['"']++
		w.rawQuote['"'] = true
	case "{'":
		w.openMarked['\'']++
	}
}

// adopt takes over the lone quotes an inner writer wrote (see rawQuote).
func (w *inlineWriter) adopt(inner *inlineWriter) {
	for q, set := range inner.rawQuote {
		if set {
			w.rawQuote[q] = true
		}
	}
}

// attrs writes n's attribute set and registers its id where it stands.
func (w *inlineWriter) attrs(n *Node) {
	w.write(attrSet(n))
	w.d.ids.register(explicitID(n))
}

func (w *inlineWriter) node(n *Node, next *Node) {
	switch n.Tag {
	case Str:
		// nodes, node's only caller, writes text runs itself; this case is
		// kept so that a text node is never silently dropped.
		if loneQuote(n) != "" {
			w.quote(n)
		} else {
			w.text(n.Text, nil, nil, next)
		}
	case SoftBreak:
		w.write("\n")
		if next != nil && next.Tag == SoftBreak {
			w.write("{}") // a line of its own, else the two breaks read as a blank line
		}
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
		w.write("$" + verbatim(n.Text))
		w.attrs(n)
	case DisplayMath:
		w.write("$$" + verbatim(n.Text))
		w.attrs(n)
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
			w.write("[" + label(n.Reference) + "]")
		case n.Reference != "" && w.d.defineRef(normalizeLabel(n.Reference), *n.Destination):
			w.write("[" + label(n.Reference) + "]")
		default:
			w.write("(" + destination(*n.Destination) + ")")
		}
		w.attrs(n)
	case Span:
		w.write("[")
		w.bracketed(n.Children)
		w.write("]")
		if attrSet(n) != "" {
			w.attrs(n)
		} else {
			w.write("{}")
		}
	case URL, Email:
		w.write("<" + n.Text + ">")
		w.attrs(n)
	case FootnoteReference:
		// Its label; a written id is dropped, as the HTML drops it.
		w.write("[^" + n.Text + "]")
		w.attrs(&Node{Tag: FootnoteReference, Attrs: n.Attrs})
	case Symb:
		w.write(":" + n.Text + ":")
		w.attrs(n)
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
	default:
		w.nodes(n.Children)
	}
}

// content writes n's children for a pair of char around them, in the plain
// form or the braced one, with the enclosing delimiters remembered.
func (w *inlineWriter) content(n *Node, char byte, braced bool) *inlineWriter {
	inner := w.child()
	inner.open[char]++
	if braced {
		inner.openMarked[char]++
	}
	inner.closer = string(rune(char))
	inner.nodes(n.Children)
	return inner
}

// child is a writer for content nested in this one's output, which knows
// the openers that are open around it: a closer inside reaches them, as a
// plain closer inside a bracket reaches an opener before the bracket.
func (w *inlineWriter) child() *inlineWriter {
	inner := newInlineWriter(w.d, w.lineStart)
	for k, v := range w.open {
		inner.open[k] = v
	}
	for k, v := range w.openMarked {
		inner.openMarked[k] = v
	}
	inner.inBrackets = w.inBrackets
	return inner
}

// bracketed writes link, image and span content. A lone quote written
// inside is reported to the enclosing writer (see delimited).
func (w *inlineWriter) bracketed(children []*Node) {
	inner := w.child()
	inner.closer = "]"
	inner.inBrackets = true
	inner.nodes(children)
	w.adopt(inner)
	w.write(inner.b.String())
}

// destination escapes the bytes of a link's destination that the parser
// would read as markup before its ")": the escape itself, a paren (an
// unpaired ")" closes the link), a bracket (a nested link or span, or a
// moved destination), an autolink's "<", a verbatim's backtick and an
// attribute's brace, each of which could take the ")" with it. processUrl
// drops the escapes when the destination is read back.
func destination(url string) string {
	return escapeBytes(url, "\\()[]<`{")
}

// label escapes the bytes of a reference label's key that the parser would
// read as an escape or as a bracket (a "[" opens a bracket inside the label
// and a "]" closes the nearest one, so an escaped "]" alone would leave its
// "[" open); unescapeLabel drops the escapes when the label is read back. A
// key is normalized, so it has no blanks an escape would need to keep.
func label(key string) string {
	return escapeBytes(key, "\\[]")
}

// escapeBytes writes a backslash before each byte of s that is in set.
func escapeBytes(s, set string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(set, s[i]) >= 0 {
			b.WriteByte('\\')
		}
		b.WriteByte(s[i])
	}
	return b.String()
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
// fence with a backtick, which the parser strips again (from math too).
// Empty content can only be written as an unclosed trailing backtick.
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
// back, so a byte is escaped only where it must be: where a node starts
// anyway (the run's start, a boundary between the run's nodes) and where
// its raw form would read as markup. The judgement is made on the bytes
// that are written: the run with the boundaries kept (expand) and what
// follows it (after: the next node's first bytes or the enclosing closer).
// For a delimiter, markup means closing an opener that is open (open,
// openMarked), shadowing the enclosing pair's closer, or pairing within the
// run; for a brace, opening an attribute, a marked quote or a marked pair
// that closes; for a closing bracket, closing a raw bracket into a link, or
// standing in a link's text; for a backslash, escaping or breaking what
// follows it; for a dollar, starting math. A byte that reads as text where
// it stands is written raw, so a text node the parser joined reads back as
// one node. In a link's, image's or span's text both brackets are escaped:
// a raw one would pair with the bracket that closes the text. A raw
// delimiter that reads as an opener is counted in open, so
// that the delimiters after it are escaped where they would close it. A
// dash is escaped where it would join another into an en or em dash, a dot
// where it would join into an ellipsis, a colon where it would start a
// symbol, and at the start of a line the first "-", "#", ">", "+" and the
// delimiter of what would read as an ordered list marker. apos lists the
// offsets of ’ that parsed from a ' and are written as ' again where that
// reads back as an apostrophe (apostropheAfter); wrote reports whether one
// was. bounds lists the offsets where a text node after the first starts.
func (w *inlineWriter) escapeText(text string, next *Node, apos, bounds []int) (string, bool) {
	text, kinds, wrote := w.expand(text, apos, bounds)
	after := w.after(next)
	var b strings.Builder
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
	// openerAt is one past the offset of the last raw delimiter of each
	// kind that reads as an opener: a closer right after it would make an
	// empty pair, which closes nothing.
	openerAt := map[byte]int{}
	// markedEnd is one past the delimiter of the last raw marked opener: a
	// "}" right after one is text, not that delimiter's closer.
	markedEnd := -1
	for i := 0; i < len(text); i++ {
		c := text[i]
		if kinds[i] == kindFixed {
			b.WriteByte(c)
			continue
		}
		// tail is what is written after the byte: the rest of the run, then
		// what follows the run.
		tail := text[i+1:] + after
		prev := w.prev()
		if i > 0 {
			prev = text[i-1]
		}
		escape := i == markerDelim || (i == 0 || kinds[i] == kindStart) && strings.IndexByte(alwaysEscaped, c) >= 0
		switch c {
		case '`', '"', '\'', '|':
			// No reading of the raw byte is text.
			escape = true
		case '\\':
			escape = escape || !backslashIsText(tail)
		case '*', '_', '~', '^':
			if strings.HasPrefix(tail, "}") && w.openMarked[c] == 0 && !(i+1 < len(kinds) && kinds[i+1] == kindStart) {
				break // X} with no marked opener to close: text
			}
			closes := !isInlineSpace(prev) && prev != 0 && w.open[c] > 0 && openerAt[c] != i
			opens := tail != "" && !isInlineSpace(tail[0])
			// Inside a pair of the same delimiter, a raw opener would take
			// the pair's closer: the nearest opener is the one it closes.
			steals := opens && strings.HasPrefix(w.closer, string(rune(c)))
			escape = escape || closes || steals
			if !escape && opens {
				w.open[c]++
				openerAt[c] = i + 1
			}
		case '{':
			inRun := text[i+1:]
			// A marked opener {X starts a node of its own when read back, so
			// where a node starts it needs no escape; its delimiter is
			// written with it, not judged as a delimiter of its own. Not
			// when X starts a node of the run (the boundary is kept by
			// escaping X), nor right after a verbatim, which takes {= as
			// its raw format.
			marked := inRun != "" && strings.IndexByte(markedDelims, inRun[0]) >= 0 && kinds[i+1] != kindStart &&
				!(i == 0 && w.prev() == '`' && inRun[0] == '=')
			if marked {
				// The opener is escaped where the line could read as a block
				// attribute ("_" and "-" are key bytes), where its closer
				// follows in the run, and inside a marked pair of the same
				// delimiter, whose closer it would take. Both bytes are
				// escaped then, so that the freed delimiter does not pair
				// with a later node's.
				if i == 0 && w.lineStart && w.attrsCouldClose(tail, next) ||
					braceOpensMarkup(tail, len(inRun)) || w.openMarked[inRun[0]] > 0 {
					b.WriteByte('\\')
					b.WriteByte(c)
					b.WriteByte('\\')
					b.WriteByte(inRun[0])
				} else {
					w.openMarked[inRun[0]]++
					b.WriteByte(c)
					b.WriteByte(inRun[0])
					markedEnd = i + 2
				}
				i++
				continue
			}
			escape = escape || braceOpensMarkup(tail, len(inRun)) || w.attrsCouldClose(tail, next)
		case '}':
			escape = escape || i != markedEnd && strings.IndexByte(markedDelims, prev) >= 0 && w.openMarked[prev] > 0
		case '[':
			// In a link's text a raw bracket would pair with the link's "]";
			// before a "^" it would start a footnote reference when a "]"
			// follows (in the run, or in what the next node writes).
			escape = escape || w.inBrackets ||
				strings.HasPrefix(tail, "^") && (len(text)-i-1 == 0 || strings.Contains(tail, "]"))
			if !escape {
				w.open['[']++
			}
		case ']':
			escape = escape || w.inBrackets || w.open['['] > 0 && bracketCloses(tail, len(text)-i-1)
		case '<':
			_, _, ok := autolinkAt(text, i)
			escape = escape || ok
		case '$':
			escape = escape || strings.HasPrefix(tail, "`") || strings.HasPrefix(tail, "$`") || strings.HasPrefix(tail, "$$`")
		case '-':
			escape = escape || (w.lineStart && i == 0) || strings.HasPrefix(tail, "-") || prev == '-'
		case '.':
			escape = escape || strings.HasPrefix(tail, ".") || prev == '.'
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

// The kinds of byte in an expanded run.
const (
	kindText  byte = iota // a byte of the run, judged by escapeText
	kindStart             // a byte of the run that starts a node: escaped
	kindFixed             // a byte the writer added, written as it is
)

// expand is the run as it is written, before escaping: a boundary between
// two of the run's nodes is kept by escaping the byte there (kindStart)
// or, where it is not escapable, by an empty attribute before it, which
// makes no node and ends the one before; a ’ that reads back as an
// apostrophe is written as ' and needs neither. wrote reports whether one
// was.
func (w *inlineWriter) expand(text string, apos, bounds []int) (expanded string, kinds []byte, wrote bool) {
	var b strings.Builder
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
				kinds = append(kinds, kindFixed)
				wrote = true
				quote = true
				i += len(apostrophe) - 1
				continue
			}
		}
		kind := kindText
		switch {
		case bound && strings.IndexByte(alwaysEscaped, text[i]) >= 0:
			kind = kindStart
		case bound && !quote:
			b.WriteString("{}")
			kinds = append(kinds, kindFixed, kindFixed)
		}
		quote = false
		b.WriteByte(text[i])
		kinds = append(kinds, kind)
	}
	return b.String(), kinds, wrote
}

// after is what the writer writes right after a text run followed by next:
// the first bytes of next's written form (enough to judge the run's last
// byte by), or with no next the enclosing closer. A pair that may be
// written braced is given as its plain delimiter, which escapes at least
// as much.
func (w *inlineWriter) after(next *Node) string {
	if next == nil {
		return w.closer
	}
	switch next.Tag {
	case Emph:
		return "_"
	case Strong:
		return "*"
	case Superscript:
		return "^"
	case Subscript:
		return "~"
	case Insert, Delete, Mark:
		return "{"
	case DoubleQuoted:
		return "\""
	case SingleQuoted:
		return "'"
	case Verbatim, RawInline:
		return "`"
	case InlineMath:
		return "$`"
	case DisplayMath:
		return "$$`"
	case Link, Span, FootnoteReference:
		return "["
	case Image:
		return "!"
	case URL, Email:
		return "<"
	case Symb:
		return ":"
	case Ellipsis:
		return "."
	case EmDash, EnDash:
		return "-"
	case SoftBreak:
		return "\n"
	case HardBreak, NonBreakingSpace:
		return "\\"
	}
	return ""
}

// backslashIsText reports whether a backslash followed by tail reads as
// text: the byte after it is not escapable and not a space (a non-breaking
// space), and a tab after it is followed by something other than blanks (a
// leaf's end or a line end would make it a hard break).
func backslashIsText(tail string) bool {
	if tail == "" {
		return false
	}
	switch next := tail[0]; {
	case isEscapable(next), next == ' ', next == '\n':
		return false
	case next == '\t':
		j := strings.TrimLeft(tail, " \t")
		return j != "" && j[0] != '\n'
	}
	return true
}

// braceOpensMarkup reports whether a raw brace followed by tail reads as
// markup: an inline attribute (an empty one too), a marked smart quote's
// opener, or a marked opener; a marked opener is text in a node of its own
// (as the parser gives it) only when its closer is nowhere in the run,
// whose remaining length is run. The brace before another node's first
// byte is judged by that byte.
func braceOpensMarkup(tail string, run int) bool {
	if tail == "" {
		return false
	}
	n := tail[0]
	switch {
	case n == '\'' || n == '"':
		return true
	case strings.IndexByte(markedDelims, n) >= 0:
		return run == 0 || strings.Contains(tail[1:run], string(n)+"}")
	}
	_, _, ok := parseInlineAttrs("{"+tail, 0)
	return ok
}

// attrsCouldClose reports whether a raw brace followed by tail (the rest
// of its run and what follows the run) could read as an attribute closed
// by a "}" written later (an attribute set, a braced pair, a kept
// boundary): the attribute is complete within tail, or the attribute
// parser has not failed by the end of tail (a brace before a node whose
// first bytes are unknown is taken to). No "}" comes after a leaf's last
// run, unless it ends in a space, which leafLines keeps with an empty
// attribute.
func (w *inlineWriter) attrsCouldClose(tail string, next *Node) bool {
	switch feedAttr("{" + tail) {
	case attrDone:
		return true
	case attrFail:
		return false
	}
	return next != nil || w.closer != "" || strings.HasSuffix(tail, " ")
}

// bracketCloses reports whether a raw "]" followed by tail would close an
// open bracket into a link, image or span: a "{" follows (a span, whether
// or not the attributes parse), or a destination or label follows and
// closes within the run, whose remaining length is run; one that would run
// into the next node is taken to close.
func bracketCloses(tail string, run int) bool {
	if tail == "" || strings.IndexByte("([{", tail[0]) < 0 {
		return false
	}
	if run == 0 || tail[0] == '{' {
		return true
	}
	return closesWithin(tail, tail[0], map[byte]byte{'(': ')', '[': ']'}[tail[0]])
}

// closesWithin reports whether the open bracket at tail's start is paired
// by a close before tail ends, counting the pairs between.
func closesWithin(tail string, open, close byte) bool {
	depth := 0
	for i := 0; i < len(tail); i++ {
		switch tail[i] {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return true
			}
		}
	}
	return false
}

// apostrophe is the glyph an apostrophe renders as.
const apostrophe = "’"

// apostropheAfter reports whether a ' written after prev, a letter, a digit
// or _, reads back as an apostrophe. It cannot open: canOpenSingleQuote
// allows none of them before it. As a plain closer it matches only a plain '
// opener: text ' is always escaped and a lone ’ is written as '} (loneQuote),
// so the only plain ' openers are SingleQuoted delimiters, and delimited
// braces every SingleQuoted that encloses such a ', through brackets too
// (inlineWriter.rawQuote). In escapeText's output the character before the
// ' is prev itself (a letter or digit is never escaped; _ is written \_), or
// the ' or ’ just written, which does not qualify.
func apostropheAfter(prev rune) bool {
	return unicode.IsLetter(prev) || unicode.IsDigit(prev) || prev == '_'
}
