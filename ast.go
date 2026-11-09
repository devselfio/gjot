package gjot

import "strconv"

// ASTText renders doc in djot.js's indented AST text format, the format the
// corpus compares against for its "a" cases.
func ASTText(doc *Node) []byte {
	return appendAST(nil, doc, 0, true)
}

func appendAST(out []byte, n *Node, indent int, isRoot bool) []byte {
	for i := 0; i < indent; i++ {
		out = append(out, ' ')
	}
	if isRoot && n.Tag == Section {
		out = append(out, "doc"...)
	} else {
		out = append(out, n.Tag...)
	}
	if p := n.Pos; p != nil {
		out = append(out, " ("...)
		out = appendPos(out, p.Start)
		out = append(out, '-')
		out = appendPos(out, p.End)
		out = append(out, ')')
	}

	switch n.Tag {
	case Symb:
		if n.Text != "" {
			out = append(out, " alias="...)
			out = appendQuoted(out, n.Text)
		}
	case Str, Verbatim, RawInline, RawBlock, CodeBlock, FootnoteReference:
		if n.Text != "" {
			out = append(out, " text="...)
			out = appendQuoted(out, n.Text)
		}
	case Heading:
		out = append(out, " level="...)
		out = strconv.AppendInt(out, int64(n.Level), 10)
	case Footnote:
		out = append(out, " label="...)
		out = appendQuoted(out, n.Text)
	case BulletList, OrderedList:
		out = appendBool(out, " tight=", n.Tight)
		if n.Style != "" {
			out = append(out, " style="...)
			out = appendQuoted(out, n.Style)
		}
	case TaskListItem:
		if n.Checked != nil {
			out = appendBool(out, " checked=", *n.Checked)
		}
	case Link, Image, URL, Email:
		if n.Destination != nil {
			out = append(out, " destination="...)
			out = appendQuoted(out, *n.Destination)
		}
		if n.Reference != "" {
			out = append(out, " reference="...)
			out = appendQuoted(out, n.Reference)
		}
	case Row:
		out = appendBool(out, " head=", n.Head)
	case Cell:
		out = appendBool(out, " head=", n.Head)
		align := n.Align
		if align == "" {
			align = AlignDefault
		}
		out = append(out, " align=\""...)
		out = append(out, align...)
		out = append(out, '"')
	}

	if n.Lang != "" {
		out = append(out, " lang="...)
		out = appendQuoted(out, n.Lang)
	}
	// Attributes print as the corpus's converter orders them: id, class, then
	// the rest in source order. The written id wins over a generated one, an
	// empty `id=""` included (zjot ast.zig).
	attrs := n.renderedAttrs()
	if id, written := n.writtenID(); written || n.ID != "" {
		if !written {
			id = n.ID
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

	for _, c := range n.Children {
		out = appendAST(out, c, indent+2, false)
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
