package djot

import (
	"strconv"

	"github.com/devselfio/gjot/ast"
)

// ASTText renders doc in djot.js's indented AST text format, the format the
// corpus compares against for its "a" cases.
func ASTText(doc ast.Node) []byte {
	return appendAST(nil, doc, 0, true)
}

func appendAST(out []byte, n ast.Node, indent int, isRoot bool) []byte {
	for i := 0; i < indent; i++ {
		out = append(out, ' ')
	}
	if _, ok := n.(*ast.Section); isRoot && ok {
		out = append(out, "doc"...)
	} else {
		out = append(out, n.Tag()...)
	}
	h := ast.HeaderOf(n)
	if p := h.Pos; p != nil {
		out = append(out, " ("...)
		out = appendPos(out, p.Start)
		out = append(out, '-')
		out = appendPos(out, p.End)
		out = append(out, ')')
	}

	// lang is a code block's language or a raw node's format, printed after
	// the kind's own attributes.
	var lang string
	switch n := n.(type) {
	case *ast.Symb:
		if n.Alias != "" {
			out = append(out, " alias="...)
			out = appendQuoted(out, n.Alias)
		}
	case *ast.Str:
		out = appendText(out, n.Text)
	case *ast.Verbatim:
		out = appendText(out, n.Text)
	case *ast.RawInline:
		out = appendText(out, n.Text)
		lang = n.Format
	case *ast.RawBlock:
		out = appendText(out, n.Text)
		lang = n.Format
	case *ast.CodeBlock:
		out = appendText(out, n.Text)
		lang = n.Lang
	case *ast.FootnoteReference:
		out = appendText(out, n.Label)
	case *ast.Heading:
		out = append(out, " level="...)
		out = strconv.AppendInt(out, int64(n.Level), 10)
	case *ast.Footnote:
		out = append(out, " label="...)
		out = appendQuoted(out, n.Label)
	case *ast.BulletList:
		out = appendListAttrs(out, n.Tight, n.Style)
	case *ast.OrderedList:
		out = appendListAttrs(out, n.Tight, n.Style)
	case *ast.TaskListItem:
		out = appendBool(out, " checked=", n.Checked)
	case *ast.Link:
		out = appendLinkAttrs(out, n.Destination, n.Reference)
	case *ast.Image:
		out = appendLinkAttrs(out, n.Destination, n.Reference)
	case *ast.Row:
		out = appendBool(out, " head=", n.Head)
	case *ast.Cell:
		out = appendBool(out, " head=", n.Head)
		align := n.Align
		if align == "" {
			align = ast.AlignDefault
		}
		out = append(out, " align=\""...)
		out = append(out, align...)
		out = append(out, '"')
	}

	if lang != "" {
		out = append(out, " lang="...)
		out = appendQuoted(out, lang)
	}
	// Attributes print as the corpus's converter orders them: id, class, then
	// the rest in source order. The written id wins over a generated one, an
	// empty `id=""` included (zjot ast.zig).
	attrs := h.Attrs
	if id, written := h.WrittenID(); written || h.ID != "" {
		if !written {
			id = h.ID
		}
		out = append(out, " id="...)
		out = appendQuoted(out, id)
	}
	if class := valueOf(attrs, "class"); hasKey(attrs, "class") {
		out = append(out, " class="...)
		out = appendQuoted(out, class)
	}
	for _, a := range attrs {
		if a.Key == "id" || a.Key == "class" {
			continue
		}
		out = append(out, ' ')
		out = append(out, a.Key...)
		out = append(out, '=')
		out = appendQuoted(out, a.Value)
	}
	out = append(out, '\n')

	for _, c := range ast.Children(n) {
		out = appendAST(out, c, indent+2, false)
	}
	return out
}

// appendText is a text kind's text attribute, none when the text is empty.
func appendText(out []byte, text string) []byte {
	if text == "" {
		return out
	}
	out = append(out, " text="...)
	return appendQuoted(out, text)
}

// appendListAttrs is a bullet or ordered list's tightness and style.
func appendListAttrs(out []byte, tight bool, style string) []byte {
	out = appendBool(out, " tight=", tight)
	if style != "" {
		out = append(out, " style="...)
		out = appendQuoted(out, style)
	}
	return out
}

// appendLinkAttrs is a link's or image's destination and reference.
func appendLinkAttrs(out []byte, dest *string, ref string) []byte {
	if dest != nil {
		out = append(out, " destination="...)
		out = appendQuoted(out, *dest)
	}
	if ref != "" {
		out = append(out, " reference="...)
		out = appendQuoted(out, ref)
	}
	return out
}

func appendPos(out []byte, p ast.SourcePos) []byte {
	out = strconv.AppendInt(out, int64(p.Line), 10)
	out = append(out, ':')
	out = strconv.AppendInt(out, int64(p.Col), 10)
	out = append(out, ':')
	return strconv.AppendInt(out, int64(p.Offset), 10)
}

func appendBool(out []byte, key string, v bool) []byte {
	out = append(out, key...)
	if v {
		return append(out, "true"...)
	}
	return append(out, "false"...)
}

func appendQuoted(out []byte, s string) []byte {
	out = append(out, '"')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '"':
			out = append(out, '\\', '"')
		case '\\':
			out = append(out, '\\', '\\')
		case '\n':
			out = append(out, '\\', 'n')
		case '\r':
			out = append(out, '\\', 'r')
		case '\t':
			out = append(out, '\\', 't')
		default:
			out = append(out, c)
		}
	}
	return append(out, '"')
}
