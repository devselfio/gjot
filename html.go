package gjot

import (
	"io"
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
type Hook func(w io.Writer, n *Node, entering bool) (WalkStatus, error)

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

// renderer writes a Node tree as HTML into out. The first hook error stops
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
}

func (r *renderer) w(s string) { r.out = append(r.out, s...) }

// Write makes the renderer the io.Writer hooks render into.
func (r *renderer) Write(p []byte) (int, error) {
	r.out = append(r.out, p...)
	return len(p), nil
}

func (r *renderer) children(n *Node) {
	for _, c := range n.Children {
		r.render(c)
	}
}

func (r *renderer) hookFor(n *Node) Hook {
	for _, c := range n.classList() {
		if h, ok := r.hooks[c]; ok {
			return h
		}
	}
	return nil
}

func (r *renderer) renderHooked(h Hook, n *Node) {
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

func (r *renderer) render(n *Node) {
	if r.err != nil {
		return
	}
	if n.Tag == Div || n.Tag == Span {
		if h := r.hookFor(n); h != nil {
			r.renderHooked(h, n)
			return
		}
	}
	switch n.Tag {
	case Section:
		if n.Level > 0 {
			r.w("<section")
			r.attrs(n)
			r.w(">\n")
			r.children(n)
			r.w("</section>\n")
		} else {
			r.children(n)
		}
	case Para:
		if r.tight {
			r.children(n)
			r.w("\n")
			return
		}
		r.w("<p")
		r.attrs(n)
		r.w(">")
		r.children(n)
		r.w("</p>\n")
	case Heading:
		tag := "h6"
		if n.Level >= 1 && n.Level <= 6 {
			tag = "h" + string(rune('0'+n.Level))
		}
		r.w("<" + tag)
		r.attrs(n)
		r.w(">")
		r.children(n)
		r.w("</" + tag + ">\n")
	case ThematicBreak:
		r.w("<hr")
		r.attrs(n)
		r.w(">\n")
	case CodeBlock:
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
	case BlockQuote:
		r.w("<blockquote")
		r.attrs(n)
		r.w(">\n")
		r.children(n)
		r.w("</blockquote>\n")
	case Div:
		r.w("<div")
		r.attrs(n)
		r.w(">\n")
		r.children(n)
		r.w("</div>\n")
	case BulletList:
		r.w("<ul")
		r.attrs(n)
		r.w(">\n")
		old := r.tight
		r.tight = n.Tight
		r.listItems(n)
		r.tight = old
		r.w("</ul>\n")
	case OrderedList:
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
		old := r.tight
		r.tight = n.Tight
		r.listItems(n)
		r.tight = old
		r.w("</ol>\n")
	case TaskList:
		r.w("<ul")
		r.classed("task-list", n)
		r.w(">\n")
		old := r.tight
		r.tight = n.Tight
		r.listItems(n)
		r.tight = old
		r.w("</ul>\n")
	case ListItem:
		r.w("<li")
		if n.ID != "" {
			r.w(" id=\"")
			r.attrEsc(n.ID)
			r.w("\"")
		}
		r.w(">\n")
		r.children(n)
		r.w("</li>\n")
	case TaskListItem:
		r.w("<li>\n")
		r.children(n)
		r.w("</li>\n")
	case DefinitionList:
		r.w("<dl")
		r.attrs(n)
		r.w(">\n")
		r.children(n)
		r.w("</dl>\n")
	case Term:
		r.w("<dt>")
		r.children(n)
		r.w("</dt>\n")
	case Definition:
		r.w("<dd>\n")
		r.children(n)
		r.w("</dd>\n")
	case Table:
		r.w("<table")
		r.attrs(n)
		r.w(">\n")
		r.children(n)
		r.w("</table>\n")
	case Caption:
		// Every table has one; an empty caption is not rendered.
		if len(n.Children) > 0 {
			r.w("<caption")
			r.attrs(n)
			r.w(">")
			r.children(n)
			r.w("</caption>\n")
		}
	case Row:
		r.w("<tr>\n")
		r.children(n)
		r.w("</tr>\n")
	case Cell:
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
	case Footnote:
		r.w("<section role=\"doc-endnotes\">\n<hr>\n<ol>\n")
		r.children(n)
		r.w("</ol>\n</section>\n")
	case Str, Escape:
		r.esc(n.Text)
	case SoftBreak:
		r.w("\n")
	case HardBreak:
		r.w("<br>\n")
	case Emph:
		r.wrap("em", n)
	case Strong:
		r.wrap("strong", n)
	case Verbatim:
		r.w("<code")
		r.attrs(n)
		r.w(">")
		r.esc(n.Text)
		r.w("</code>")
	case Link:
		r.w("<a")
		if n.Destination != nil {
			r.w(" href=\"")
			r.attrEsc(*n.Destination)
			r.w("\"")
		}
		r.attrs(n)
		r.w(">")
		if len(n.Children) > 0 {
			r.children(n)
		} else {
			r.esc(n.Text)
		}
		r.w("</a>")
	case Image:
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
	case Span:
		r.w("<span")
		r.attrs(n)
		r.w(">")
		r.children(n)
		r.w("</span>")
	case FootnoteReference:
		// The tag writes its own id (fnrefN); a written id is dropped, not
		// written twice (zjot.test difference 3).
		r.w("<a")
		if n.ID != "" {
			r.w(" id=\"")
			r.attrEsc(n.ID)
			r.w("\"")
		}
		if n.Destination != nil {
			r.w(" href=\"")
			r.attrEsc(*n.Destination)
			r.w("\"")
		}
		r.w(" role=\"doc-noteref\"")
		for _, a := range n.renderedAttrs() {
			if a.Key != "id" {
				r.attr(a.Key, a.Value)
			}
		}
		r.w("><sup>")
		r.esc(n.Text)
		r.w("</sup></a>")
	case Superscript:
		r.wrap("sup", n)
	case Subscript:
		r.wrap("sub", n)
	case Insert:
		r.wrap("ins", n)
	case Delete:
		r.wrap("del", n)
	case Mark:
		r.wrap("mark", n)
	case InlineMath:
		r.classed("math inline", n)
		r.w(">\\(")
		r.esc(n.Text)
		r.w("\\)</span>")
	case DisplayMath:
		r.classed("math display", n)
		r.w(">\\[")
		r.esc(n.Text)
		r.w("\\]</span>")
	case URL:
		r.w("<a href=\"")
		r.attrEsc(n.Text)
		r.w("\"")
		r.attrs(n)
		r.w(">")
		r.esc(n.Text)
		r.w("</a>")
	case Email:
		r.w("<a href=\"mailto:")
		r.attrEsc(n.Text)
		r.w("\"")
		r.attrs(n)
		r.w(">")
		r.esc(n.Text)
		r.w("</a>")
	case Symb:
		// zjot's rendering (djot.js prints the `:alias:` text; difference 4).
		r.classed("symbol", n)
		r.w(">")
		r.esc(n.Text)
		r.w("</span>")
	case DoubleQuoted:
		r.w("\u201c")
		r.children(n)
		r.w("\u201d")
	case SingleQuoted:
		r.w("\u2018")
		r.children(n)
		r.w("\u2019")
	case NonBreakingSpace:
		r.w("&nbsp;")
	case LeftSingleQuote:
		r.w("\u2018")
	case RightSingleQuote:
		r.w("\u2019")
	case LeftDoubleQuote:
		r.w("\u201c")
	case RightDoubleQuote:
		r.w("\u201d")
	case Ellipsis:
		r.w("\u2026")
	case EmDash:
		r.w("\u2014")
	case EnDash:
		r.w("\u2013")
	case RawInline, RawBlock:
		if n.Lang == "html" {
			r.w(n.Text)
		}
	default:
		r.children(n)
	}
}

func (r *renderer) wrap(tag string, n *Node) {
	r.w("<" + tag)
	r.attrs(n)
	r.w(">")
	r.children(n)
	r.w("</" + tag + ">")
}

func (r *renderer) altText(children []*Node) {
	r.attrEsc(stringContent(children, true))
}

func (r *renderer) listItems(list *Node) {
	for _, item := range list.Children {
		r.w("<li>\n")
		if item.Tag == TaskListItem {
			if item.Checked != nil && *item.Checked {
				r.w("<input disabled=\"\" type=\"checkbox\" checked=\"\"/>\n")
			} else {
				r.w("<input disabled=\"\" type=\"checkbox\"/>\n")
			}
		}
		r.children(item)
		r.w("</li>\n")
	}
}

// attrs writes the node's attributes in source order, as djot.js: its id
// where an `id` was written (an empty `id=""` included, over a generated
// id), or first when the document gave it.
func (r *renderer) attrs(n *Node) {
	r.filteredAttrs(n, false)
}

// classed opens a span (or, for a task list, a ul) whose intrinsic class
// the node's classes join, then writes its other attributes, as djot.js.
func (r *renderer) classed(intrinsic string, n *Node) {
	if n.Tag == TaskList {
		r.w(" class=\"")
	} else {
		r.w("<span class=\"")
	}
	r.w(intrinsic)
	for _, c := range n.classList() {
		r.w(" ")
		r.attrEsc(c)
	}
	r.w("\"")
	r.filteredAttrs(n, true)
}

func (r *renderer) filteredAttrs(n *Node, skipClass bool) {
	attrs := n.renderedAttrs()
	if n.ID != "" && !hasKey(attrs, "id") {
		r.attr("id", n.ID)
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
