package gjot

import "strings"

// blockAttrs is a parsed attribute set `{#id .class key="value" %comment%}`.
// Classes are carried as a single "class" entry in attrs; the classes field
// is the fenced-div class that merge preserves alongside them.
type blockAttrs struct {
	id      string
	classes string
	attrs   []Attr
}

func (b blockAttrs) empty() bool {
	return b.id == "" && b.classes == "" && len(b.attrs) == 0
}

// merge layers other over b: a later id wins, classes concatenate, and a
// later key overrides an earlier one. The merged class entry comes last.
func (b blockAttrs) merge(other blockAttrs) blockAttrs {
	result := blockAttrs{id: other.id}
	if result.id == "" {
		result.id = b.id
	}

	var classes []string
	var merged []Attr
	for _, a := range b.attrs {
		if a.Key == "class" {
			classes = append(classes, a.Value)
		}
	}
	for _, a := range other.attrs {
		if a.Key == "class" {
			classes = append(classes, a.Value)
		}
	}
	for _, a := range b.attrs {
		if a.Key == "class" || hasKey(other.attrs, a.Key) {
			continue
		}
		merged = append(merged, a)
	}
	for _, a := range other.attrs {
		if a.Key != "class" {
			merged = append(merged, a)
		}
	}
	if len(classes) > 0 {
		merged = append(merged, Attr{"class", strings.Join(classes, " ")})
	}

	result.classes = other.classes
	if result.classes == "" {
		result.classes = b.classes
	}
	result.attrs = merged
	return result
}

func hasKey(attrs []Attr, key string) bool {
	for _, a := range attrs {
		if a.Key == key {
			return true
		}
	}
	return false
}

type attrStatus int

const (
	attrDone attrStatus = iota
	attrFail
	attrContinue
)

type attrState int

const (
	attrStart attrState = iota
	attrScanning
	attrScanningID
	attrScanningClass
	attrScanningKey
	attrScanningValue
	attrScanningBareValue
	attrScanningQuotedValue
	attrScanningEscaped
	attrScanningComment
	attrStateDone
	attrStateFail
)

// attrParser is the djot.js attribute state machine over src.
type attrParser struct {
	src        string
	state      attrState
	begin      int // -1 when no token is open
	lastpos    int // -1 before the first step
	id         string
	attrs      []Attr
	valueBuf   []byte
	currentKey string
	haveKey    bool
	// idWritten is set once an id token was read, empty or not.
	idWritten bool
}

func newAttrParser(src string) *attrParser {
	return &attrParser{src: src, begin: -1, lastpos: -1}
}

// feed steps the machine over src[start..end] inclusive and reports whether
// the attribute set closed, failed, or needs more input, with the position
// reached.
func (p *attrParser) feed(start, end int) (attrStatus, int) {
	pos := start
	for pos <= end && pos < len(p.src) {
		p.state = p.step(pos)
		if p.state == attrStateDone {
			return attrDone, pos
		}
		if p.state == attrStateFail {
			return attrFail, pos
		}
		p.lastpos = pos
		pos++
	}
	if end < len(p.src) {
		return attrContinue, end
	}
	return attrContinue, max(len(p.src)-1, 0)
}

func (p *attrParser) step(pos int) attrState {
	c := p.src[pos]
	switch p.state {
	case attrStart:
		if c == '{' {
			return attrScanning
		}
		return attrStateFail
	case attrScanning:
		return p.stepScanning(c, pos)
	case attrScanningID:
		return p.stepScanningID(c, pos)
	case attrScanningClass:
		return p.stepScanningClass(c, pos)
	case attrScanningKey:
		return p.stepScanningKey(c, pos)
	case attrScanningValue:
		return p.stepScanningValue(c, pos)
	case attrScanningBareValue:
		return p.stepScanningBareValue(c, pos)
	case attrScanningQuotedValue:
		return p.stepScanningQuotedValue(c, pos)
	case attrScanningEscaped:
		return attrScanningQuotedValue
	case attrScanningComment:
		return stepScanningComment(c)
	}
	return p.state
}

func (p *attrParser) stepScanning(c byte, pos int) attrState {
	switch {
	case isAttrWhitespace(c):
		return attrScanning
	case c == '}':
		return attrStateDone
	case c == '#':
		p.begin = pos
		return attrScanningID
	case c == '%':
		p.begin = pos
		return attrScanningComment
	case c == '.':
		p.begin = pos
		return attrScanningClass
	case isAttrName(c):
		p.begin = pos
		return attrScanningKey
	}
	return attrStateFail
}

func (p *attrParser) stepScanningID(c byte, pos int) attrState {
	switch {
	case isIDChar(c):
		return attrScanningID
	case c == '}':
		p.emitID(pos)
		return attrStateDone
	case isAttrWhitespace(c):
		p.emitID(pos)
		return attrScanning
	}
	return attrStateFail
}

func (p *attrParser) stepScanningClass(c byte, pos int) attrState {
	switch {
	case isAttrName(c):
		return attrScanningClass
	case c == '}':
		p.emitClass(pos)
		return attrStateDone
	case isAttrWhitespace(c):
		p.emitClass(pos)
		return attrScanning
	}
	return attrStateFail
}

func (p *attrParser) stepScanningKey(c byte, pos int) attrState {
	if c == '=' && p.begin >= 0 {
		p.currentKey = p.src[p.begin:pos]
		p.haveKey = true
		p.begin = -1
		return attrScanningValue
	}
	if isAttrName(c) {
		return attrScanningKey
	}
	return attrStateFail
}

func (p *attrParser) stepScanningValue(c byte, pos int) attrState {
	if c == '"' {
		p.begin = pos
		p.valueBuf = p.valueBuf[:0]
		return attrScanningQuotedValue
	}
	if isAttrName(c) {
		p.begin = pos
		return attrScanningBareValue
	}
	return attrStateFail
}

func (p *attrParser) stepScanningBareValue(c byte, pos int) attrState {
	switch {
	case isAttrName(c):
		return attrScanningBareValue
	case c == '}':
		p.emitBareValue(pos)
		return attrStateDone
	case isAttrWhitespace(c):
		p.emitBareValue(pos)
		return attrScanning
	}
	return attrStateFail
}

func (p *attrParser) stepScanningQuotedValue(c byte, pos int) attrState {
	switch c {
	case '"':
		p.emitQuotedValue(pos)
		return attrScanning
	case '\\':
		return attrScanningEscaped
	case '\n':
		// A line break inside a quoted value becomes one space.
		if p.begin >= 0 {
			p.valueBuf = append(p.valueBuf, p.src[p.begin+1:pos]...)
			p.valueBuf = append(p.valueBuf, ' ')
			p.begin = pos
		}
	}
	return attrScanningQuotedValue
}

func stepScanningComment(c byte) attrState {
	switch c {
	case '%':
		return attrScanning
	case '}':
		return attrStateDone
	}
	return attrScanningComment
}

func (p *attrParser) emitID(pos int) {
	p.idWritten = true
	if p.begin < 0 {
		return
	}
	lp := p.lastpos
	if lp < 0 {
		lp = max(pos-1, 0)
	}
	if lp >= p.begin {
		p.id = p.src[p.begin+1 : pos]
	}
	p.begin = -1
}

func (p *attrParser) emitClass(pos int) {
	if p.begin < 0 {
		return
	}
	p.attrs = append(p.attrs, Attr{"class", p.src[p.begin+1 : pos]})
	p.begin = -1
}

func (p *attrParser) emitBareValue(pos int) {
	if p.begin < 0 {
		return
	}
	if p.haveKey {
		p.attrs = append(p.attrs, Attr{p.currentKey, p.src[p.begin:pos]})
		p.haveKey = false
	}
	p.begin = -1
}

func (p *attrParser) emitQuotedValue(pos int) {
	var raw string
	if len(p.valueBuf) > 0 {
		if p.begin >= 0 {
			p.valueBuf = append(p.valueBuf, p.src[p.begin+1:pos]...)
		}
		raw = string(p.valueBuf)
		p.valueBuf = p.valueBuf[:0]
	} else if p.begin >= 0 {
		raw = p.src[p.begin+1 : pos]
	}
	if p.haveKey {
		p.attrs = append(p.attrs, Attr{p.currentKey, processAttrEscapes(raw)})
		p.haveKey = false
	}
	p.begin = -1
}

func processAttrEscapes(raw string) string {
	if !strings.Contains(raw, "\\") {
		return raw
	}
	buf := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\\' && i+1 < len(raw) {
			i++
		}
		buf = append(buf, raw[i])
	}
	return string(buf)
}

// finish returns the parsed set. All class entries fold into one "class"
// attribute at the position of the first.
func (p *attrParser) finish() blockAttrs {
	result := blockAttrs{id: p.id}
	if len(p.attrs) == 0 {
		return result
	}
	var classes []string
	classAt := -1
	var merged []Attr
	for _, a := range p.attrs {
		if a.Key == "class" {
			classes = append(classes, a.Value)
			if classAt < 0 {
				classAt = len(merged)
				merged = append(merged, Attr{Key: "class"})
			}
			continue
		}
		merged = append(merged, a)
	}
	if classAt >= 0 {
		merged[classAt].Value = strings.Join(classes, " ")
	}
	result.attrs = merged
	return result
}

func isAttrName(c byte) bool {
	return isAlphanumeric(c) || c == '_' || c == '-' || c == ':'
}

func isIDChar(c byte) bool {
	if c <= ' ' {
		return false
	}
	switch c {
	case ']', '[', '~', '!', '@', '#', '$', '%', '^', '&', '*', '(', ')', '{', '}',
		'`', ',', '.', '<', '>', '\\', '|', '=', '+', '/', '?', '"', '\'':
		return false
	}
	return true
}

func isAttrWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func isAlphanumeric(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// tryParseBlockAttr parses a line that is nothing but an attribute set.
func tryParseBlockAttr(line string) (blockAttrs, bool) {
	trimmed := strings.Trim(line, " \t")
	if len(trimmed) < 2 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
		return blockAttrs{}, false
	}
	return parseAttrsFromStr(trimmed)
}

// parseAttrsFromStr parses src, which must begin with '{' and close within it.
func parseAttrsFromStr(src string) (blockAttrs, bool) {
	if len(src) < 2 || src[0] != '{' {
		return blockAttrs{}, false
	}
	p := newAttrParser(src)
	if status, _ := p.feed(0, len(src)-1); status != attrDone {
		return blockAttrs{}, false
	}
	return p.finish(), true
}

// parseInlineAttrs parses an attribute set starting at src[pos] and returns
// the index just past its closing brace.
func parseInlineAttrs(src string, pos int) (blockAttrs, int, bool) {
	if pos >= len(src) || src[pos] != '{' {
		return blockAttrs{}, 0, false
	}
	p := newAttrParser(src)
	status, end := p.feed(pos, len(src)-1)
	if status != attrDone {
		return blockAttrs{}, 0, false
	}
	return p.finish(), end + 1, true
}

// feedLine feeds the next line of a block attribute that may span lines, as
// djot.js feeds its attribute container's parser line by line: the line and
// its line end go after what was fed before, and the machine runs over them.
// It reports attrDone with the position of the closing '}' in line,
// attrFail with the failing position (len(line) when the line end failed
// it), or attrContinue.
func (p *attrParser) feedLine(line string) (attrStatus, int) {
	start := len(p.src)
	p.src += line + "\n"
	status, pos := p.feed(start, len(p.src)-1)
	if status == attrContinue {
		return attrContinue, len(line)
	}
	return status, pos - start
}

// writesID reports whether an id was written, "#x" or empty: a later one
// overrides an id waiting before it, which was registered all the same.
func (p *attrParser) writesID() bool { return p.idWritten }

// feedAttr is the attribute parser's status after src, which starts with
// '{': done, failed, or still open at its last byte.
func feedAttr(src string) attrStatus {
	status, _ := newAttrParser(src).feed(0, len(src)-1)
	return status
}

// opensAttr reports whether line, a block attribute's first line, leaves the
// attribute unfinished: '{', then attribute syntax still open at the line's
// last byte, its line end not fed (zjot attributes.zig opensAttr).
func opensAttr(line string) bool {
	trimmed := strings.Trim(line, " \t")
	if trimmed == "" || trimmed[0] != '{' {
		return false
	}
	status, _ := newAttrParser(trimmed).feed(0, len(trimmed)-1)
	return status == attrContinue
}

// opensAttrBlock reports whether a line opens a block attribute container:
// its first line parses so far, or completes at a '}' with nothing but
// blanks after it (djot.js; zjot Parser.zig tryOpen).
func opensAttrBlock(line string) bool {
	if line == "" || line[0] != '{' {
		return false
	}
	status, pos := newAttrParser("").feedLine(line)
	switch status {
	case attrContinue:
		return true
	case attrDone:
		return pos >= len(line) || isBlank(line[pos+1:])
	}
	return false
}
