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
	return SourcePos{Line: seg.orig.Line, Col: seg.orig.Col + delta, Offset: seg.orig.Offset + delta}
}
