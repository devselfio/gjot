package gjot

import "strconv"

// ASTText renders doc in djot.js's indented AST text format, the format the
// corpus compares against for its "a" cases.
func ASTText(doc *Node) []byte {
	return appendAST(nil, fromLegacy(doc), 0, true)
}

func appendAST(out []byte, n treeNode, indent int, isRoot bool) []byte {
	for i := 0; i < indent; i++ {
		out = append(out, ' ')
	}
	if _, ok := n.(*treeSection); isRoot && ok {
		out = append(out, "doc"...)
	} else {
		out = append(out, n.Tag()...)
	}
	h := n.header()
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
	case *treeSymb:
		if n.Alias != "" {
			out = append(out, " alias="...)
			out = appendQuoted(out, n.Alias)
		}
	case *treeStr:
		out = appendText(out, n.Text)
	case *treeVerbatim:
		out = appendText(out, n.Text)
	case *treeRawInline:
		out = appendText(out, n.Text)
		lang = n.Format
	case *treeRawBlock:
		out = appendText(out, n.Text)
		lang = n.Format
	case *treeCodeBlock:
		out = appendText(out, n.Text)
		lang = n.Lang
	case *treeFootnoteReference:
		out = appendText(out, n.Label)
	case *treeHeading:
		out = append(out, " level="...)
		out = strconv.AppendInt(out, int64(n.Level), 10)
	case *treeFootnote:
		out = append(out, " label="...)
		out = appendQuoted(out, n.Label)
	case *treeBulletList:
		out = appendListAttrs(out, n.Tight, n.Style)
	case *treeOrderedList:
		out = appendListAttrs(out, n.Tight, n.Style)
	case *treeTaskListItem:
		out = appendBool(out, " checked=", n.Checked)
	case *treeLink:
		out = appendLinkAttrs(out, n.Destination, n.Reference)
	case *treeImage:
		out = appendLinkAttrs(out, n.Destination, n.Reference)
	case *treeRow:
		out = appendBool(out, " head=", n.Head)
	case *treeCell:
		out = appendBool(out, " head=", n.Head)
		align := n.Align
		if align == "" {
			align = AlignDefault
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
	if id, written := h.writtenID(); written || h.ID != "" {
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

	for _, c := range childrenOf(n) {
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

func appendPos(out []byte, p SourcePos) []byte {
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
