package djot

import (
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/devselfio/gjot/ast"
)

// Hook renders a div or span in place of the default markup. It is called
// with entering true before the children and entering false after them;
// the exit call's WalkStatus is ignored. Returning an error aborts the
// render and HTML returns it. gjot.Hook is the public form.
type Hook func(w io.Writer, n ast.Node, entering bool) (ast.WalkStatus, error)

// HTML renders doc as HTML, byte for byte as djot.js does, rendering a div
// or span with the hook of its first class that has one in hooks (nil for
// none). The error is nil unless a hook returns one, and the output is then
// nil.
func HTML(doc ast.Node, hooks map[string]Hook) ([]byte, error) {
	r := &renderer{hooks: hooks}
	r.render(doc)
	r.endnotes()
	if r.err != nil {
		return nil, r.err
	}
	return r.out, nil
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
	footnoteDefs   map[string]*ast.Footnote
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
func (r *renderer) numberFootnotes(n ast.Node) {
	switch n := n.(type) {
	case *ast.Image:
		return
	case *ast.FootnoteReference:
		r.footnoteNumber(n.Label)
	}
	for _, c := range ast.Children(n) {
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
		var blocks []ast.Node
		if def := r.footnoteDefs[r.footnoteOrder[i]]; def != nil {
			blocks = def.Children
		}
		last := len(blocks) - 1
		var lastPara *ast.Para
		if last >= 0 {
			lastPara, _ = blocks[last].(*ast.Para)
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

func (r *renderer) children(n ast.Node) {
	for _, c := range ast.Children(n) {
		r.render(c)
	}
}

func (r *renderer) hookFor(n ast.Node) Hook {
	for _, c := range ast.HeaderOf(n).ClassList() {
		if h, ok := r.hooks[c]; ok {
			return h
		}
	}
	return nil
}

// renderHooked renders n through h.
func (r *renderer) renderHooked(h Hook, n ast.Node) {
	status, err := h(r, n, true)
	if err != nil {
		r.err = err
		return
	}
	if status != ast.WalkSkipChildren {
		r.children(n)
	}
	if r.err != nil {
		return
	}
	if _, err := h(r, n, false); err != nil {
		r.err = err
	}
}

func (r *renderer) render(n ast.Node) {
	if r.err != nil {
		return
	}
	switch n.(type) {
	case *ast.Div, *ast.Span:
		if h := r.hookFor(n); h != nil {
			r.renderHooked(h, n)
			return
		}
	}
	switch n := n.(type) {
	case *ast.Section:
		if n.Level > 0 {
			r.w("<section")
			r.attrs(n)
			r.w(">\n")
			r.children(n)
			r.w("</section>\n")
		} else {
			r.children(n)
		}
	case *ast.Para:
		r.para(n, "")
	case *ast.Heading:
		tag := "h6"
		if n.Level >= 1 && n.Level <= 6 {
			tag = "h" + string(rune('0'+n.Level))
		}
		r.w("<" + tag)
		r.attrs(n)
		r.w(">")
		r.children(n)
		r.w("</" + tag + ">\n")
	case *ast.ThematicBreak:
		r.w("<hr")
		r.attrs(n)
		r.w(">\n")
	case *ast.CodeBlock:
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
	case *ast.BlockQuote:
		r.w("<blockquote")
		r.attrs(n)
		r.w(">\n")
		r.children(n)
		r.w("</blockquote>\n")
	case *ast.Div:
		r.w("<div")
		r.attrs(n)
		r.w(">\n")
		r.children(n)
		r.w("</div>\n")
	case *ast.BulletList:
		r.w("<ul")
		r.attrs(n)
		r.w(">\n")
		r.listItems(n, n.Tight)
		r.w("</ul>\n")
	case *ast.OrderedList:
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
	case *ast.TaskList:
		r.w("<ul")
		r.classed("task-list", n)
		r.w(">\n")
		r.listItems(n, n.Tight)
		r.w("</ul>\n")
	case *ast.ListItem:
		r.w("<li")
		if n.ID != "" {
			r.w(" id=\"")
			r.attrEsc(n.ID)
			r.w("\"")
		}
		r.w(">\n")
		r.children(n)
		r.w("</li>\n")
	case *ast.TaskListItem:
		r.w("<li>\n")
		r.children(n)
		r.w("</li>\n")
	case *ast.DefinitionList:
		r.w("<dl")
		r.attrs(n)
		r.w(">\n")
		r.children(n)
		r.w("</dl>\n")
	case *ast.Term:
		r.w("<dt>")
		r.children(n)
		r.w("</dt>\n")
	case *ast.Definition:
		r.w("<dd>\n")
		r.children(n)
		r.w("</dd>\n")
	case *ast.Table:
		r.w("<table")
		r.attrs(n)
		r.w(">\n")
		r.children(n)
		r.w("</table>\n")
	case *ast.Caption:
		// Every table has one; an empty caption is not rendered.
		if len(n.Children) > 0 {
			r.w("<caption")
			r.attrs(n)
			r.w(">")
			r.children(n)
			r.w("</caption>\n")
		}
	case *ast.Row:
		r.w("<tr>\n")
		r.children(n)
		r.w("</tr>\n")
	case *ast.Cell:
		tag := "td"
		if n.Head {
			tag = "th"
		}
		r.w("<" + tag)
		if n.Align != "" && n.Align != ast.AlignDefault {
			r.w(" style=\"text-align: " + string(n.Align) + ";\"")
		}
		r.w(">")
		r.children(n)
		r.w("</" + tag + ">\n")
	case *ast.Footnote:
		// A definition renders nothing where it stands; its content is
		// endnote N when the body references it (endnotes). A label defined
		// again replaces the earlier content.
		if r.footnoteDefs == nil {
			r.footnoteDefs = map[string]*ast.Footnote{}
		}
		if _, seen := r.footnoteDefs[n.Label]; !seen {
			r.footnoteLabels = append(r.footnoteLabels, n.Label)
		}
		r.footnoteDefs[n.Label] = n
	case *ast.Str:
		r.esc(n.Text)
	case *ast.SoftBreak:
		r.w("\n")
	case *ast.HardBreak:
		r.w("<br>\n")
	case *ast.Emph:
		r.wrap("em", n)
	case *ast.Strong:
		r.wrap("strong", n)
	case *ast.Verbatim:
		r.w("<code")
		r.attrs(n)
		r.w(">")
		r.esc(n.Text)
		r.w("</code>")
	case *ast.Link:
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
	case *ast.Image:
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
	case *ast.Span:
		r.w("<span")
		r.attrs(n)
		r.w(">")
		r.children(n)
		r.w("</span>")
	case *ast.FootnoteReference:
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
	case *ast.Superscript:
		r.wrap("sup", n)
	case *ast.Subscript:
		r.wrap("sub", n)
	case *ast.Insert:
		r.wrap("ins", n)
	case *ast.Delete:
		r.wrap("del", n)
	case *ast.Mark:
		r.wrap("mark", n)
	case *ast.InlineMath:
		r.classed("math inline", n)
		r.w(">\\(")
		r.esc(n.Text)
		r.w("\\)</span>")
	case *ast.DisplayMath:
		r.classed("math display", n)
		r.w(">\\[")
		r.esc(n.Text)
		r.w("\\]</span>")
	case *ast.URL:
		r.w("<a href=\"")
		r.attrEsc(n.Text)
		r.w("\"")
		r.attrs(n)
		r.w(">")
		r.esc(n.Text)
		r.w("</a>")
	case *ast.Email:
		r.w("<a href=\"mailto:")
		r.attrEsc(n.Text)
		r.w("\"")
		r.attrs(n)
		r.w(">")
		r.esc(n.Text)
		r.w("</a>")
	case *ast.Symb:
		// zjot's rendering (djot.js prints the `:alias:` text; difference 4).
		r.classed("symbol", n)
		r.w(">")
		r.esc(n.Alias)
		r.w("</span>")
	case *ast.DoubleQuoted:
		r.w("\u201c")
		r.children(n)
		r.w("\u201d")
	case *ast.SingleQuoted:
		r.w("\u2018")
		r.children(n)
		r.w("\u2019")
	case *ast.NonBreakingSpace:
		r.w("&nbsp;")
	case *ast.Ellipsis:
		r.w("\u2026")
	case *ast.EmDash:
		r.w("\u2014")
	case *ast.EnDash:
		r.w("\u2013")
	case *ast.RawInline:
		if n.Format == "html" {
			r.w(n.Text)
		}
	case *ast.RawBlock:
		if n.Format == "html" {
			r.w(n.Text)
		}
	default:
		r.children(n)
	}
}

func (r *renderer) wrap(tag string, n ast.Node) {
	r.w("<" + tag)
	r.attrs(n)
	r.w(">")
	r.children(n)
	r.w("</" + tag + ">")
}

func (r *renderer) altText(children []ast.Node) {
	r.attrEsc(stringContent(children, true))
}

// listItems writes a list's items with tight in force below them.
func (r *renderer) listItems(list ast.Node, tight bool) {
	old := r.tight
	r.tight = tight
	for _, item := range ast.Children(list) {
		r.w("<li>\n")
		if t, ok := item.(*ast.TaskListItem); ok {
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
func (r *renderer) para(n *ast.Para, trailer string) {
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
func (r *renderer) attrs(n ast.Node) {
	r.filteredAttrs(ast.HeaderOf(n), false)
}

// classed opens a span (or, for a task list, a ul) whose intrinsic class
// the node's classes join, then writes its other attributes, as djot.js.
func (r *renderer) classed(intrinsic string, n ast.Node) {
	if _, ok := n.(*ast.TaskList); ok {
		r.w(" class=\"")
	} else {
		r.w("<span class=\"")
	}
	r.w(intrinsic)
	h := ast.HeaderOf(n)
	for _, c := range h.ClassList() {
		r.w(" ")
		r.attrEsc(c)
	}
	r.w("\"")
	r.filteredAttrs(h, true)
}

func (r *renderer) filteredAttrs(h *ast.Header, skipClass bool) {
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
