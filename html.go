package gjot

import (
	"io"
	"sort"
	"strconv"
	"strings"
)

// WalkStatus is what a Hook's entering call returns to control rendering of
// the node's children.
type WalkStatus int

const (
	// WalkContinue renders the children between the entering and exit calls.
	WalkContinue WalkStatus = iota
	// WalkSkipChildren leaves the children unrendered.
	WalkSkipChildren
)

// Hook renders a div or span in place of the default markup. It is called
// with entering true before the children and entering false after them;
// the exit call's WalkStatus is ignored. Returning an error aborts the
// render and HTML returns it.
type Hook func(w io.Writer, n treeNode, entering bool) (WalkStatus, error)

// WithHook registers h for every div or span that has class. When a node
// has several registered classes, the first in its class list wins.
func WithHook(class string, h Hook) HTMLOption {
	return func(r *renderer) {
		if r.hooks == nil {
			r.hooks = map[string]Hook{}
		}
		r.hooks[class] = h
	}
}

// renderer writes the tree as HTML into out. The first hook error stops
// all further output.
type renderer struct {
	out   []byte
	hooks map[string]Hook
	err   error
	// tight is true while rendering the items of a tight bullet, ordered
	// or task list, the innermost such list winning, and makes every
	// paragraph below them render bare, however deep in block quotes,
	// divs (hooked or not) and definitions. Only lists set it; a
	// definition list inherits the enclosing list's.
	tight bool
	// footnotes numbers the footnotes as djot.js's HTML renderer does:
	// footnoteOrder holds the labels by first reference (a body reference
	// in reading order, an image's alt text excepted, then the references
	// inside the definitions as endnotes walks them), footnoteDefs the
	// Footnote nodes met by label, footnoteLabels those labels in the
	// order first met.
	footnoteOrder  []string
	footnoteDefs   map[string]*treeFootnote
	footnoteLabels []string
}

// footnoteNumber is label's 1-based number, assigned on its first reference.
func (r *renderer) footnoteNumber(label string) int {
	for i, l := range r.footnoteOrder {
		if l == label {
			return i + 1
		}
	}
	r.footnoteOrder = append(r.footnoteOrder, label)
	return len(r.footnoteOrder)
}

// numberFootnotes numbers the footnote references the renderer would write
// in n's tree: an image's children are its alt text, where a reference is
// plain text.
func (r *renderer) numberFootnotes(n treeNode) {
	switch n := n.(type) {
	case *treeImage:
		return
	case *treeFootnoteReference:
		r.footnoteNumber(n.Label)
	}
	for _, c := range childrenOf(n) {
		r.numberFootnotes(c)
	}
}

// jsArrayIndex is label as the array index JavaScript orders an object's
// keys by first: the canonical decimal of an integer below 2^32-1, no sign
// or leading zero; false for any other key.
func jsArrayIndex(label string) (int, bool) {
	if label == "" || len(label) > 10 || (len(label) > 1 && label[0] == '0') {
		return 0, false
	}
	v := 0
	for i := 0; i < len(label); i++ {
		c := label[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		v = v*10 + int(c-'0')
	}
	if v >= 4294967295 {
		return 0, false
	}
	return v, true
}

// endnotes writes the footnote section once the body has referenced a note
// (djot.js writes it only then): every definition's references take their
// numbers first, in the order djot.js walks its definitions object (labels
// that are array indexes ascending, then the rest as first defined), so a
// note first referenced inside another is numbered at the turn of the
// definition that references it; then each numbered label's endnote, with
// its backlink ending its last paragraph or in a paragraph of its own, and
// only the backlink when nothing defines it.
func (r *renderer) endnotes() {
	if len(r.footnoteOrder) == 0 {
		return
	}
	var indexes, rest []string
	for _, label := range r.footnoteLabels {
		if _, ok := jsArrayIndex(label); ok {
			indexes = append(indexes, label)
		} else {
			rest = append(rest, label)
		}
	}
	sort.SliceStable(indexes, func(i, j int) bool {
		a, _ := jsArrayIndex(indexes[i])
		b, _ := jsArrayIndex(indexes[j])
		return a < b
	})
	for _, label := range append(indexes, rest...) {
		for _, b := range r.footnoteDefs[label].Children {
			r.numberFootnotes(b)
		}
	}
	r.w("<section role=\"doc-endnotes\">\n<hr>\n<ol>\n")
	for i := 0; i < len(r.footnoteOrder); i++ {
		num := strconv.Itoa(i + 1)
		backlink := "<a href=\"#fnref" + num + "\" role=\"doc-backlink\">↩︎</a>"
		r.w("<li id=\"fn" + num + "\">\n")
		var blocks []treeNode
		if def := r.footnoteDefs[r.footnoteOrder[i]]; def != nil {
			blocks = def.Children
		}
		last := len(blocks) - 1
		var lastPara *treePara
		if last >= 0 {
			lastPara, _ = blocks[last].(*treePara)
		}
		if lastPara == nil {
			last = len(blocks)
		}
		for _, b := range blocks[:last] {
			r.render(b)
		}
		if lastPara != nil {
			r.para(lastPara, backlink)
		} else {
			r.w("<p>" + backlink + "</p>\n")
		}
		r.w("</li>\n")
	}
	r.w("</ol>\n</section>\n")
}

func (r *renderer) w(s string) { r.out = append(r.out, s...) }

// Write makes the renderer the io.Writer hooks render into.
func (r *renderer) Write(p []byte) (int, error) {
	r.out = append(r.out, p...)
	return len(p), nil
}

func (r *renderer) children(n treeNode) {
	for _, c := range childrenOf(n) {
		r.render(c)
	}
}

func (r *renderer) hookFor(n treeNode) Hook {
	for _, c := range n.header().classList() {
		if h, ok := r.hooks[c]; ok {
			return h
		}
	}
	return nil
}

// renderHooked renders n through h.
func (r *renderer) renderHooked(h Hook, n treeNode) {
	status, err := h(r, n, true)
	if err != nil {
		r.err = err
		return
	}
	if status != WalkSkipChildren {
		r.children(n)
	}
	if r.err != nil {
		return
	}
	if _, err := h(r, n, false); err != nil {
		r.err = err
	}
}

func (r *renderer) render(n treeNode) {
	if r.err != nil {
		return
	}
	switch n.(type) {
	case *treeDiv, *treeSpan:
		if h := r.hookFor(n); h != nil {
			r.renderHooked(h, n)
			return
		}
	}
	switch n := n.(type) {
	case *treeSection:
		if n.Level > 0 {
			r.w("<section")
			r.attrs(n)
			r.w(">\n")
			r.children(n)
			r.w("</section>\n")
		} else {
			r.children(n)
		}
	case *treePara:
		r.para(n, "")
	case *treeHeading:
		tag := "h6"
		if n.Level >= 1 && n.Level <= 6 {
			tag = "h" + string(rune('0'+n.Level))
		}
		r.w("<" + tag)
		r.attrs(n)
		r.w(">")
		r.children(n)
		r.w("</" + tag + ">\n")
	case *treeThematicBreak:
		r.w("<hr")
		r.attrs(n)
		r.w(">\n")
	case *treeCodeBlock:
		r.w("<pre")
		r.attrs(n)
		r.w("><code")
		if n.Lang != "" {
			r.w(" class=\"language-")
			r.attrEsc(n.Lang)
			r.w("\"")
		}
		r.w(">")
		r.esc(n.Text)
		r.w("</code></pre>\n")
	case *treeBlockQuote:
		r.w("<blockquote")
		r.attrs(n)
		r.w(">\n")
		r.children(n)
		r.w("</blockquote>\n")
	case *treeDiv:
		r.w("<div")
		r.attrs(n)
		r.w(">\n")
		r.children(n)
		r.w("</div>\n")
	case *treeBulletList:
		r.w("<ul")
		r.attrs(n)
		r.w(">\n")
		r.listItems(n, n.Tight)
		r.w("</ul>\n")
	case *treeOrderedList:
		// The list's own start and type come before its attributes, as
		// djot.js.
		r.w("<ol")
		if start := orderedStart(n); start != 1 {
			r.w(" start=\"" + strconv.Itoa(start) + "\"")
		}
		if style, _, ok := parseStyleString(n.Style); ok && style.htmlType() != "" {
			r.w(" type=\"" + style.htmlType() + "\"")
		}
		r.attrs(n)
		r.w(">\n")
		r.listItems(n, n.Tight)
		r.w("</ol>\n")
	case *treeTaskList:
		r.w("<ul")
		r.classed("task-list", n)
		r.w(">\n")
		r.listItems(n, n.Tight)
		r.w("</ul>\n")
	case *treeListItem:
		r.w("<li")
		if n.ID != "" {
			r.w(" id=\"")
			r.attrEsc(n.ID)
			r.w("\"")
		}
		r.w(">\n")
		r.children(n)
		r.w("</li>\n")
	case *treeTaskListItem:
		r.w("<li>\n")
		r.children(n)
		r.w("</li>\n")
	case *treeDefinitionList:
		r.w("<dl")
		r.attrs(n)
		r.w(">\n")
		r.children(n)
		r.w("</dl>\n")
	case *treeTerm:
		r.w("<dt>")
		r.children(n)
		r.w("</dt>\n")
	case *treeDefinition:
		r.w("<dd>\n")
		r.children(n)
		r.w("</dd>\n")
	case *treeTable:
		r.w("<table")
		r.attrs(n)
		r.w(">\n")
		r.children(n)
		r.w("</table>\n")
	case *treeCaption:
		// Every table has one; an empty caption is not rendered.
		if len(n.Children) > 0 {
			r.w("<caption")
			r.attrs(n)
			r.w(">")
			r.children(n)
			r.w("</caption>\n")
		}
	case *treeRow:
		r.w("<tr>\n")
		r.children(n)
		r.w("</tr>\n")
	case *treeCell:
		tag := "td"
		if n.Head {
			tag = "th"
		}
		r.w("<" + tag)
		if n.Align != "" && n.Align != AlignDefault {
			r.w(" style=\"text-align: " + string(n.Align) + ";\"")
		}
		r.w(">")
		r.children(n)
		r.w("</" + tag + ">\n")
	case *treeFootnote:
		// A definition renders nothing where it stands; its content is
		// endnote N when the body references it (endnotes). A label defined
		// again replaces the earlier content.
		if r.footnoteDefs == nil {
			r.footnoteDefs = map[string]*treeFootnote{}
		}
		if _, seen := r.footnoteDefs[n.Label]; !seen {
			r.footnoteLabels = append(r.footnoteLabels, n.Label)
		}
		r.footnoteDefs[n.Label] = n
	case *treeStr:
		r.esc(n.Text)
	case *treeSoftBreak:
		r.w("\n")
	case *treeHardBreak:
		r.w("<br>\n")
	case *treeEmph:
		r.wrap("em", n)
	case *treeStrong:
		r.wrap("strong", n)
	case *treeVerbatim:
		r.w("<code")
		r.attrs(n)
		r.w(">")
		r.esc(n.Text)
		r.w("</code>")
	case *treeLink:
		r.w("<a")
		if n.Destination != nil {
			r.w(" href=\"")
			r.attrEsc(*n.Destination)
			r.w("\"")
		}
		r.attrs(n)
		r.w(">")
		r.children(n)
		r.w("</a>")
	case *treeImage:
		r.w("<img alt=\"")
		r.altText(n.Children)
		r.w("\"")
		if n.Destination != nil {
			r.w(" src=\"")
			r.attrEsc(*n.Destination)
			r.w("\"")
		}
		r.attrs(n)
		r.w(">")
	case *treeSpan:
		r.w("<span")
		r.attrs(n)
		r.w(">")
		r.children(n)
		r.w("</span>")
	case *treeFootnoteReference:
		// The tag writes its own id (fnrefN); a written id is dropped, not
		// written twice (zjot.test difference 3).
		num := strconv.Itoa(r.footnoteNumber(n.Label))
		r.w("<a id=\"fnref" + num + "\" href=\"#fn" + num + "\" role=\"doc-noteref\"")
		for _, a := range n.Attrs {
			if a.Key != "id" {
				r.attr(a.Key, a.Value)
			}
		}
		r.w("><sup>" + num + "</sup></a>")
	case *treeSuperscript:
		r.wrap("sup", n)
	case *treeSubscript:
		r.wrap("sub", n)
	case *treeInsert:
		r.wrap("ins", n)
	case *treeDelete:
		r.wrap("del", n)
	case *treeMark:
		r.wrap("mark", n)
	case *treeInlineMath:
		r.classed("math inline", n)
		r.w(">\\(")
		r.esc(n.Text)
		r.w("\\)</span>")
	case *treeDisplayMath:
		r.classed("math display", n)
		r.w(">\\[")
		r.esc(n.Text)
		r.w("\\]</span>")
	case *treeURL:
		r.w("<a href=\"")
		r.attrEsc(n.Text)
		r.w("\"")
		r.attrs(n)
		r.w(">")
		r.esc(n.Text)
		r.w("</a>")
	case *treeEmail:
		r.w("<a href=\"mailto:")
		r.attrEsc(n.Text)
		r.w("\"")
		r.attrs(n)
		r.w(">")
		r.esc(n.Text)
		r.w("</a>")
	case *treeSymb:
		// zjot's rendering (djot.js prints the `:alias:` text; difference 4).
		r.classed("symbol", n)
		r.w(">")
		r.esc(n.Alias)
		r.w("</span>")
	case *treeDoubleQuoted:
		r.w("\u201c")
		r.children(n)
		r.w("\u201d")
	case *treeSingleQuoted:
		r.w("\u2018")
		r.children(n)
		r.w("\u2019")
	case *treeNonBreakingSpace:
		r.w("&nbsp;")
	case *treeEllipsis:
		r.w("\u2026")
	case *treeEmDash:
		r.w("\u2014")
	case *treeEnDash:
		r.w("\u2013")
	case *treeRawInline:
		if n.Format == "html" {
			r.w(n.Text)
		}
	case *treeRawBlock:
		if n.Format == "html" {
			r.w(n.Text)
		}
	default:
		r.children(n)
	}
}

func (r *renderer) wrap(tag string, n treeNode) {
	r.w("<" + tag)
	r.attrs(n)
	r.w(">")
	r.children(n)
	r.w("</" + tag + ">")
}

func (r *renderer) altText(children []treeNode) {
	r.attrEsc(stringContent(children, true))
}

// listItems writes a list's items with tight in force below them.
func (r *renderer) listItems(list treeNode, tight bool) {
	old := r.tight
	r.tight = tight
	for _, item := range childrenOf(list) {
		r.w("<li>\n")
		if t, ok := item.(*treeTaskListItem); ok {
			if t.Checked {
				r.w("<input disabled=\"\" type=\"checkbox\" checked=\"\"/>\n")
			} else {
				r.w("<input disabled=\"\" type=\"checkbox\"/>\n")
			}
		}
		r.children(item)
		r.w("</li>\n")
	}
	r.tight = old
}

// para writes a paragraph with trailer inside it after its content; bare
// inside a tight list.
func (r *renderer) para(n *treePara, trailer string) {
	if r.tight {
		r.children(n)
		r.w(trailer + "\n")
		return
	}
	r.w("<p")
	r.attrs(n)
	r.w(">")
	r.children(n)
	r.w(trailer + "</p>\n")
}

// attrs writes the node's attributes in source order, as djot.js: its id
// where an `id` was written (an empty `id=""` included, over a generated
// id), or first when the document gave it.
func (r *renderer) attrs(n treeNode) {
	r.filteredAttrs(n.header(), false)
}

// classed opens a span (or, for a task list, a ul) whose intrinsic class
// the node's classes join, then writes its other attributes, as djot.js.
func (r *renderer) classed(intrinsic string, n treeNode) {
	if _, ok := n.(*treeTaskList); ok {
		r.w(" class=\"")
	} else {
		r.w("<span class=\"")
	}
	r.w(intrinsic)
	h := n.header()
	for _, c := range h.classList() {
		r.w(" ")
		r.attrEsc(c)
	}
	r.w("\"")
	r.filteredAttrs(h, true)
}

func (r *renderer) filteredAttrs(h *treeHeader, skipClass bool) {
	attrs := h.Attrs
	if h.ID != "" && !hasKey(attrs, "id") {
		r.attr("id", h.ID)
	}
	for _, a := range attrs {
		if skipClass && a.Key == "class" {
			continue
		}
		r.attr(a.Key, a.Value)
	}
}

func (r *renderer) attr(key, value string) {
	r.w(" " + key + "=\"")
	r.attrEsc(value)
	r.w("\"")
}

var htmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
var attrEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;")

func (r *renderer) esc(text string)     { r.w(htmlEscaper.Replace(text)) }
func (r *renderer) attrEsc(text string) { r.w(attrEscaper.Replace(text)) }
