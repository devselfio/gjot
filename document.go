package gjot

import (
	"strconv"
	"unicode/utf8"
)

// document.go is what block syntax defines for the whole document and what
// the tree needs from all of it, in the shape of zjot v2.0.0's document.zig:
// the definitions (references and footnotes), the ids, sections, reference
// resolution and the footnote section. zjot's log, counters and Reserved
// places exist because it assigns ids after a block is streamed out; gjot
// builds the whole tree, so they are not ported and the container stack
// registers each id where it stands.

// refDefinition is a reference definition: its url, and the block
// attributes written before it, if any.
type refDefinition struct {
	url   string
	attrs *blockAttrs
}

// footnoteDef is a footnote definition: its blocks, the block attributes
// written before it, and its source range when positions are tracked.
type footnoteDef struct {
	blocks []treeNode
	attrs  *blockAttrs
	pos    *Range
}

// definitions holds the reference and footnote definitions by normalized
// label. A label defined again replaces the earlier definition (djot.js) and
// keeps its place in footnoteLabels, the labels in the order they first
// closed (nested definitions inner first), which is the order the tree
// holds the footnotes in.
type definitions struct {
	refDefs        map[string]refDefinition
	footnoteDefs   map[string]*footnoteDef
	footnoteLabels []string
}

// defineFootnote records label's standing definition.
func (d *definitions) defineFootnote(label string, def *footnoteDef) {
	if _, seen := d.footnoteDefs[label]; !seen {
		d.footnoteLabels = append(d.footnoteLabels, label)
	}
	d.footnoteDefs[label] = def
}

// ids is every id in the document as the stack reads it: explicit ones
// registered where they stand, generated ones avoiding every id seen so far,
// and the implicit reference the first heading with a text claims.
type ids struct {
	used       map[string]bool
	autoRefs   map[string]string
	nextSuffix map[string]int
}

func newIDs() ids {
	return ids{used: map[string]bool{}, autoRefs: map[string]string{}, nextSuffix: map[string]int{}}
}

// register keeps an explicit id, read where it stands.
func (t *ids) register(id string) {
	if id != "" {
		t.used[id] = true
	}
}

// registerTree registers every explicit id in nodes' trees, in tree order.
func (t *ids) registerTree(nodes []treeNode) {
	for _, n := range nodes {
		if id := n.header().ID; id != "" && !autoID(n) {
			t.register(id)
		}
		t.registerTree(childrenOf(n))
	}
}

// isIDSeparator is djot.js's id separator set, `[]~!@#$%^&*(){}`,.<>\|=+/?`,
// and JavaScript's \s.
func isIDSeparator(r rune) bool {
	switch r {
	case '[', ']', '~', '!', '@', '#', '$', '%', '^', '&', '*', '(', ')', '{', '}',
		'`', ',', '.', '<', '>', '\\', '|', '=', '+', '/', '?':
		return true
	}
	return isJSSpace(r)
}

// idBase is the id djot.js's getUniqueIdentifier makes of text before any
// suffix: a run of separators becomes one '-', none at either end, every
// other rune kept (an invalid byte as the byte it was); "" for no text.
func idBase(text string) string {
	var buf []byte
	separated := false
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if isIDSeparator(r) {
			separated = true
			i += size
			continue
		}
		if separated && len(buf) > 0 {
			buf = append(buf, '-')
		}
		separated = false
		buf = append(buf, text[i:i+size]...)
		i += size
	}
	return string(buf)
}

// generate is djot.js's getUniqueIdentifier, as zjot's Ids.generate: the
// base idBase gives when it is free; an empty text gives s-1, s-2, ...; a
// taken id gets the next free suffix.
func (t *ids) generate(text string) string {
	base := idBase(text)
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
// its rendered text (stringContent, without footnote labels). The first
// heading with a given text claims it as an implicit reference; a heading
// whose text normalizes to nothing claims none.
func (t *ids) heading(n *treeHeading, explicit string) {
	text := stringContent(n.Children, false)
	if explicit != "" {
		n.ID, n.AutoID = explicit, false
	} else {
		n.ID, n.AutoID = t.generate(text), true
	}
	label := normalizeLabel(text)
	if _, ok := t.autoRefs[label]; label != "" && !ok {
		t.autoRefs[label] = "#" + n.ID
	}
}

// document is the state the stack shares across every block it builds.
type document struct {
	defs     definitions
	ids      ids
	trackPos bool
}

func newDocument(trackPos bool) *document {
	return &document{
		defs:     definitions{refDefs: map[string]refDefinition{}, footnoteDefs: map[string]*footnoteDef{}},
		ids:      newIDs(),
		trackPos: trackPos,
	}
}

// finish builds the root from the top-level blocks once every definition is
// known: sections, then the footnote definitions after them, then
// references. end is where a section still open at the end ends (zjot
// Sections: the last block's end, or the input's when lines follow it).
func (d *document) finish(blocks []treeNode, end *SourcePos) *treeSection {
	blocks = d.sections(blocks, end)
	blocks = append(blocks, d.footnotes()...)
	d.resolve(blocks)
	return &treeSection{Children: blocks}
}

// footnotes is every footnote definition as a Footnote node, as djot.js's
// JSON holds them and zjot's AST prints them: its label, its blocks
// as children, its span, and the block attributes written before it; in
// the order the labels first closed. The HTML renderer numbers the endnotes
// from these where the body references them.
func (d *document) footnotes() []treeNode {
	var nodes []treeNode
	for _, label := range d.defs.footnoteLabels {
		def := d.defs.footnoteDefs[label]
		n := &treeFootnote{treeHeader: treeHeader{Pos: def.pos}, Label: label, Children: def.blocks}
		if def.attrs != nil {
			applyBlockAttrs(n, *def.attrs)
		}
		nodes = append(nodes, n)
	}
	return nodes
}

// sections nests the top-level blocks by heading level (zjot Sections.feed):
// a heading closes the open sections of its level or deeper and opens its
// own. The heading's id moves to its section, and its written attributes
// move only when they hold a non-empty id; otherwise they stay on the
// heading. A section runs from its heading's start to the end of the
// heading that closes it, or to end.
func (d *document) sections(blocks []treeNode, end *SourcePos) []treeNode {
	type open struct {
		sec *treeSection
	}
	var result []treeNode
	var stack []open
	add := func(n treeNode) {
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
	for _, n := range blocks {
		b, ok := n.(*treeHeading)
		if !ok {
			add(n)
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
		sec := &treeSection{treeHeader: treeHeader{ID: b.ID}, Level: b.Level, AutoID: b.AutoID}
		heading := *b
		heading.ID, heading.AutoID = "", false
		// A written non-empty id moves the heading's attributes to the
		// section; else the heading keeps them, an `id=""` included, and
		// the section takes the generated id alone (zjot document.zig).
		if b.ID != "" && !b.AutoID {
			sec.Attrs, heading.Attrs = b.Attrs, nil
		}
		if b.Pos != nil {
			sec.Pos = &Range{Start: b.Pos.Start, End: b.Pos.End}
		}
		sec.Children = []treeNode{&heading}
		stack = append(stack, open{sec})
	}
	for len(stack) > 0 {
		closeTop(end)
	}
	return result
}

// resolve fills in reference links and images from the definitions, else
// from a heading's implicit reference. Nodes are updated in place.
func (d *document) resolve(nodes []treeNode) {
	for _, n := range nodes {
		if dest, ref, children, ok := linkFields(n); ok && *dest == nil {
			raw := *ref
			if raw == "" {
				raw = stringContent(children, true)
			}
			label := normalizeLabel(raw)
			if label == "" {
				// An empty label resolves nothing (djot.js).
			} else if def, ok := d.defs.refDefs[label]; ok {
				url := def.url
				*dest = &url
				if def.attrs != nil {
					mergeRefAttrs(n, *def.attrs)
				}
			} else if auto, ok := d.ids.autoRefs[label]; ok {
				*dest = &auto
			}
		}
		d.resolve(childrenOf(n))
	}
}
