package gjot

// lineMap maps byte offsets in joined inline text back to source positions.
// Built when a paragraph's lines are joined for inline parsing.
type lineMap struct {
	segments []lineSegment
}

type lineSegment struct {
	joinedStart int
	orig        SourcePos
}

// resolve maps an offset in the joined text to its source position. The
// newline that joins a line to the next is reported as the next line,
// column 0, at the newline's own offset, as zjot reports it.
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
	if best+1 < len(m.segments) && joinedOffset == m.segments[best+1].joinedStart-1 {
		return SourcePos{Line: m.segments[best+1].orig.Line, Col: 0, Offset: seg.orig.Offset + delta}
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

// applyInlinePositions sets positions on inline nodes at any depth: a text
// slice from its offset and length, any other node from the extent the
// inline parser recorded for it.
func applyInlinePositions(nodes []*Node, m lineMap) {
	for _, n := range nodes {
		switch {
		case n.Tag == Str && n.off > 0:
			n.Pos = m.rangeOf(n.off-1, n.off-1+len(n.Text))
		case n.src.end > 0:
			n.Pos = m.rangeOf(n.src.start, n.src.end)
		}
		applyInlinePositions(n.Children, m)
	}
}
