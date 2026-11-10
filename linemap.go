package gjot

// lineMap maps byte offsets in joined inline text back to source positions.
// Built when a paragraph's lines are joined for inline parsing.
type lineMap struct {
	segments []lineSegment
	// total is the joined text's length: the offset of the newline that
	// would follow the last line, where a hard break ending the leaf
	// stands.
	total int
}

// lineSegment is one source line's slice of the joined text: where it
// starts in the joined text, where it starts in the source, and where that
// source line's newline is (srcLine.eol).
type lineSegment struct {
	joinedStart int
	orig        SourcePos
	eol         SourcePos
}

// resolve maps an offset in the joined text to its source position. The
// newline that joins a line to the next is reported as that line's own
// newline, as zjot reports it, even when a source line with no slice (a
// heading's bare marker line) stands between the two. This relies on
// parseParagraph joining lines with exactly one "\n" byte: the byte just
// before a segment's joinedStart is taken to be that newline. The offset
// one past the joined text (total) is the last line's newline.
func (m lineMap) resolve(joinedOffset int) SourcePos {
	if len(m.segments) == 0 {
		return SourcePos{Line: 1, Col: 1, Offset: joinedOffset}
	}
	best := 0
	for i, seg := range m.segments {
		if seg.joinedStart > joinedOffset {
			break
		}
		best = i
	}
	seg := m.segments[best]
	delta := joinedOffset - seg.joinedStart
	if best+1 < len(m.segments) && joinedOffset == m.segments[best+1].joinedStart-1 || joinedOffset == m.total {
		return seg.eol
	}
	return SourcePos{Line: seg.orig.Line, Col: seg.orig.Col + delta, Offset: seg.orig.Offset + delta}
}

// rangeOf is the source range of joined bytes [start, end), inclusive at
// both ends as zjot prints it; nil when the span is empty.
func (m lineMap) rangeOf(start, end int) *Range {
	if end <= start {
		return nil
	}
	return &Range{Start: m.resolve(start), End: m.resolve(end - 1)}
}

// applyStrPositions sets the position of every text slice among the inline
// nodes, at any depth, from its offset and length, once the leaf is parsed
// and its text final. Every other inline node had its position set as it
// was made (inlineParser.at), a literal text node's (a smart quote's)
// included.
func applyStrPositions(nodes []Node, m *lineMap) {
	for _, n := range nodes {
		if s, ok := n.(*Str); ok {
			if s.off > 0 {
				s.Pos = m.rangeOf(s.off-1, s.off-1+len(s.Text))
			}
			continue
		}
		applyStrPositions(childrenOf(n), m)
	}
}
