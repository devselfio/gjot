package gjot

import "strconv"

// document.go is what block syntax defines for the whole document and what
// the tree needs from all of it, in the shape of zjot v2.0.0's document.zig:
// the definitions (references and footnotes), the ids, sections, reference
// resolution and the footnote section. zjot's log, counters and Reserved
// places exist because it assigns ids after a block is streamed out; gjot
// builds the whole tree, so the container stack registers each id where it
// stands (farraway/gridapp#12, Decision 2).

// refDefinition is a reference definition: its url, and the block
// attributes written before it, if any.
type refDefinition struct {
	url   string
	attrs *blockAttrs
}

// footnoteDef is a footnote definition: its blocks, the block attributes
// written before it, and its source range when positions are tracked.
type footnoteDef struct {
	blocks []*Node
	attrs  *blockAttrs
	pos    *Range
}

// definitions holds the reference and footnote definitions by normalized
// label. A label defined again replaces the earlier definition (djot.js).
type definitions struct {
	refDefs      map[string]refDefinition
	footnoteDefs map[string]*footnoteDef
}

// ids is every id in the document as the stack reads it: explicit ones
// registered where they stand, generated ones avoiding every id seen so far,
// and the implicit reference the first heading with a text claims.
type ids struct {
	used       map[string]bool
	autoRefs   map[string]string
	nextSuffix map[string]int
}

// register keeps an explicit id, read where it stands.
func (t *ids) register(id string) {
	if id != "" {
		t.used[id] = true
	}
}

// registerTree registers every explicit id in nodes' trees, in tree order.
func (t *ids) registerTree(nodes []*Node) {
	for _, n := range nodes {
		if n.ID != "" && !n.AutoID {
			t.register(n.ID)
		}
		t.registerTree(n.Children)
	}
}

// isIDSeparator is djot.js's id separator set, `[]~!@#$%^&*(){}`,.<>\|=+/?`
// and whitespace.
func isIDSeparator(c byte) bool {
	switch c {
	case '[', ']', '~', '!', '@', '#', '$', '%', '^', '&', '*', '(', ')', '{', '}',
		'`', ',', '.', '<', '>', '\\', '|', '=', '+', '/', '?', ' ', '\t', '\n', '\r':
		return true
	}
	return false
}

// generate is djot.js's getUniqueIdentifier, as zjot's Ids.generate: a run
// of separators becomes one '-', none at either end, every other byte kept;
// an empty text gives s-1, s-2, ...; a taken id gets the next free suffix.
func (t *ids) generate(text string) string {
	var buf []byte
	separated := false
	for i := 0; i < len(text); i++ {
		c := text[i]
		if isIDSeparator(c) {
			separated = true
			continue
		}
		if separated && len(buf) > 0 {
			buf = append(buf, '-')
		}
		separated = false
		buf = append(buf, c)
	}
	base := string(buf)
	empty := base == ""
	if empty {
		base = "s"
	}
	if !empty && !t.used[base] {
		t.used[base] = true
		return base
	}
	counter := t.nextSuffix[base]
	if counter == 0 {
		counter = 1
	}
	for ; ; counter++ {
		candidate := base + "-" + strconv.Itoa(counter)
		if !t.used[candidate] {
			t.used[candidate] = true
			t.nextSuffix[base] = counter + 1
			return candidate
		}
	}
}

// heading gives a heading that has just closed its id: explicit when one
// was written for it (already registered as it opened), else generated from
// its text. The first heading with a given text claims it as an implicit
// reference.
func (t *ids) heading(n *Node, explicit string) {
	if explicit != "" {
		n.ID, n.AutoID = explicit, false
	} else {
		n.ID, n.AutoID = t.generate(n.idText), true
	}
	label := normalizeLabel(n.idText)
	if _, ok := t.autoRefs[label]; !ok {
		t.autoRefs[label] = "#" + n.ID
	}
}

// document is the state the stack shares across every block it builds.
type document struct {
	defs          definitions
	ids           ids
	footnoteOrder []string
	trackPos      bool
}

func newDocument(trackPos bool) *document {
	return &document{
		defs: definitions{refDefs: map[string]refDefinition{}, footnoteDefs: map[string]*footnoteDef{}},
		ids: ids{
			used:       map[string]bool{},
			autoRefs:   map[string]string{},
			nextSuffix: map[string]int{},
		},
		trackPos: trackPos,
	}
}

// finish builds the root from the top-level blocks once every definition is
// known: sections, then references, then the footnote section. end is where
// a section still open at the end ends (zjot Sections: the last block's end,
// or the input's when lines follow it).
func (d *document) finish(blocks []*Node, end *SourcePos) *Node {
	blocks = d.sections(blocks, end)
	d.resolve(blocks)
	if len(d.footnoteOrder) > 0 {
		blocks = append(blocks, d.footnoteSection())
	}
	return &Node{Tag: Section, Children: blocks}
}

// sections nests the top-level blocks by heading level (zjot Sections.feed):
// a heading closes the open sections of its level or deeper and opens its
// own. The heading's id moves to its section, and its written attributes
// move only when they hold a non-empty id; otherwise they stay on the
// heading. A section runs from its heading's start to the end of the
// heading that closes it, or to end.
func (d *document) sections(blocks []*Node, end *SourcePos) []*Node {
	type open struct {
		sec *Node
	}
	var result []*Node
	var stack []open
	add := func(n *Node) {
		if len(stack) > 0 {
			top := stack[len(stack)-1].sec
			top.Children = append(top.Children, n)
		} else {
			result = append(result, n)
		}
	}
	closeTop := func(at *SourcePos) {
		top := stack[len(stack)-1].sec
		stack = stack[:len(stack)-1]
		if top.Pos != nil && at != nil {
			top.Pos.End = *at
		}
		add(top)
	}
	for _, b := range blocks {
		if b.Tag != Heading {
			add(b)
			continue
		}
		var closedAt *SourcePos
		if b.Pos != nil {
			e := b.Pos.End
			closedAt = &e
		}
		for len(stack) > 0 && stack[len(stack)-1].sec.Level >= b.Level {
			closeTop(closedAt)
		}
		sec := &Node{Tag: Section, Level: b.Level, ID: b.ID, AutoID: b.AutoID}
		heading := *b
		heading.ID, heading.AutoID = "", false
		if b.ID != "" && !b.AutoID {
			sec.Attrs, sec.Classes = b.Attrs, b.Classes
			heading.Attrs, heading.Classes = nil, ""
		}
		if b.Pos != nil {
			sec.Pos = &Range{Start: b.Pos.Start, End: b.Pos.End}
		}
		sec.Children = []*Node{&heading}
		stack = append(stack, open{sec})
	}
	for len(stack) > 0 {
		closeTop(end)
	}
	return result
}

// resolve fills in reference links and images from the definitions, else
// from a heading's implicit reference, and numbers footnote references in
// order of first use. Nodes are updated in place.
func (d *document) resolve(nodes []*Node) {
	for _, n := range nodes {
		if (n.Tag == Link || n.Tag == Image) && n.Destination == nil {
			raw := n.Reference
			if raw == "" {
				raw = getPlainText(n)
			}
			label := normalizeLabel(raw)
			if def, ok := d.defs.refDefs[label]; ok {
				url := def.url
				n.Destination = &url
				if def.attrs != nil {
					mergeRefAttrs(n, *def.attrs)
				}
			} else if dest, ok := d.ids.autoRefs[label]; ok {
				n.Destination = &dest
			}
		}
		if n.Tag == FootnoteReference {
			num := d.footnoteNumber(normalizeLabel(n.Text))
			n.Text = strconv.Itoa(num)
			n.ID = "fnref" + strconv.Itoa(num)
			dest := "#fn" + strconv.Itoa(num)
			n.Destination = &dest
		}
		d.resolve(n.Children)
	}
}

func (d *document) footnoteNumber(label string) int {
	for i, existing := range d.footnoteOrder {
		if existing == label {
			return i + 1
		}
	}
	d.footnoteOrder = append(d.footnoteOrder, label)
	return len(d.footnoteOrder)
}

// footnoteSection is the endnotes, in order of first reference, each with a
// backlink after its last paragraph, as today's tree has them. The block
// attributes written before a definition are kept with it (footnoteDef) but
// not put on the endnote: neither djot.js's HTML nor zjot's renders them.
func (d *document) footnoteSection() *Node {
	var items []*Node
	// footnoteOrder can grow while resolving a note that references another.
	for i := 0; i < len(d.footnoteOrder); i++ {
		label := d.footnoteOrder[i]
		var content []*Node
		if def := d.defs.footnoteDefs[label]; def != nil {
			content = def.blocks
		}
		d.resolve(content)
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
