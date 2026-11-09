package gjot

import "strings"

// blockAttrs is a parsed attribute set `{#id .class key="value" %comment%}`.
// attrs holds every attribute as djot.js's attribute object holds them: a
// key keeps its first position and takes its last value, `id=x` is the same
// key as `#x`, a `.c` joins the classes before it and `class=c` starts them
// over. id is the non-empty `id` among them: an empty `id=""` stays an
// attribute and names nothing, as djot.js tests the id's truthiness.
type blockAttrs struct {
	id    string
	attrs []Attr
}

// attrsOf is attrs as the block attributes they are, the id read off them.
func attrsOf(attrs []Attr) blockAttrs {
	return blockAttrs{id: valueOf(attrs, "id"), attrs: attrs}
}

func (b blockAttrs) empty() bool {
	return len(b.attrs) == 0
}

// merge applies other over a copy of b, as djot.js applies one attribute's
// object to the one before it: each key set as attrObject.set sets it, a
// class joined to the classes before it (a later block attribute, or a later
// inline attribute on the same node; zjot.test difference 16).
func (b blockAttrs) merge(other blockAttrs) blockAttrs {
	obj := make(attrObject, len(b.attrs), len(b.attrs)+len(other.attrs))
	copy(obj, b.attrs)
	for _, a := range other.attrs {
		obj.set(a.Key, a.Value, a.Key == "class")
	}
	return attrsOf(obj)
}

// attrObject is djot.js's attribute object under construction: set keeps a
// key's first position and gives it the last value; with join, the value
// goes after the one there with a space (a `.class`, and a class merged from
// a later attribute), unless there is none yet.
type attrObject []Attr

func (o *attrObject) set(key, value string, join bool) {
	for i := range *o {
		if (*o)[i].Key != key {
			continue
		}
		if join && (*o)[i].Value != "" {
			(*o)[i].Value += " " + value
		} else {
			(*o)[i].Value = value
		}
		return
	}
	*o = append(*o, Attr{key, value})
}

// writesID reports whether an id was written, `#x` or `id=`, empty included:
// a later one overrides an id waiting before it, which djot.js registered all
// the same.
func (b blockAttrs) writesID() bool { return hasKey(b.attrs, "id") }

func hasKey(attrs []Attr, key string) bool {
	for _, a := range attrs {
		if a.Key == key {
			return true
		}
	}
	return false
}

// valueOf is the value of the key among attrs, or "" when absent.
func valueOf(attrs []Attr, key string) string {
	for _, a := range attrs {
		if a.Key == key {
			return a.Value
		}
	}
	return ""
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
	obj        attrObject
	valueBuf   []byte
	currentKey string
	haveKey    bool
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

// emitID sets the `id` key, so renderers write it where it was written; a
// second `#id` keeps the first's place with the last value, as djot.js's
// object does. An empty `#` names nothing (djot.js).
func (p *attrParser) emitID(pos int) {
	if p.begin < 0 {
		return
	}
	if p.tokenEnd(pos) > p.begin {
		p.obj.set("id", p.src[p.begin+1:pos], false)
	}
	p.begin = -1
}

// emitClass joins the class to the classes before it. An empty `.` names
// nothing (djot.js).
func (p *attrParser) emitClass(pos int) {
	if p.begin < 0 {
		return
	}
	if p.tokenEnd(pos) > p.begin {
		p.obj.set("class", p.src[p.begin+1:pos], true)
	}
	p.begin = -1
}

// tokenEnd is the last byte of the token closing at pos: the byte before it
// that the machine stepped over, or pos-1 when none was.
func (p *attrParser) tokenEnd(pos int) int {
	if p.lastpos < 0 {
		return max(pos-1, 0)
	}
	return p.lastpos
}

func (p *attrParser) emitBareValue(pos int) {
	if p.begin < 0 {
		return
	}
	if p.haveKey {
		p.obj.set(p.currentKey, p.src[p.begin:pos], false)
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
		p.obj.set(p.currentKey, processAttrEscapes(collapseBlanks(raw)), false)
		p.haveKey = false
	}
	p.begin = -1
}

// collapseBlanks makes a quoted value's runs of spaces and line ends one
// space (djot.js's `[ \r\n]+`); an escaped line end is among them, its
// backslash left for processAttrEscapes.
func collapseBlanks(raw string) string {
	if !strings.ContainsAny(raw, "\r\n") && !strings.Contains(raw, "  ") {
		return raw
	}
	buf := make([]byte, 0, len(raw))
	inRun := false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c == ' ' || c == '\r' || c == '\n' {
			if !inRun {
				buf = append(buf, ' ')
			}
			inRun = true
			continue
		}
		buf = append(buf, c)
		inRun = false
	}
	return string(buf)
}

// attrEscapable reports whether a backslash before c goes in an attribute
// value: djot.js's set, which lacks "<", ">", "@" and the space, so those
// keep their backslash.
func attrEscapable(c byte) bool {
	return strings.IndexByte(".,\\/#!$%^&*;:{}=-_`~+[]()'\"?|", c) >= 0
}

func processAttrEscapes(raw string) string {
	if !strings.Contains(raw, "\\") {
		return raw
	}
	buf := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\\' && i+1 < len(raw) && attrEscapable(raw[i+1]) {
			i++
		}
		buf = append(buf, raw[i])
	}
	return string(buf)
}

// finish returns the parsed set.
func (p *attrParser) finish() blockAttrs {
	return attrsOf(p.obj)
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
