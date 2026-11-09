package gjot

import (
	"reflect"
	"testing"
)

// The leaf functions are the stack parser's view of one line, ported from
// zjot v2.0.0's leaf.zig. A tab is one column, as in djot.js.

func TestLeafIsBlank(t *testing.T) {
	for _, tc := range []struct {
		line string
		want bool
	}{
		{"", true}, {"   ", true}, {"\t", true}, {" \t \r", true},
		{"a", false}, {"  a", false}, {"\ta", false},
	} {
		if got := isBlank(tc.line); got != tc.want {
			t.Errorf("isBlank(%q) = %v, want %v", tc.line, got, tc.want)
		}
	}
}

// TestLeafCarriageReturn pins zjot v2.0.0's leaf.zig rule for a bare '\r':
// only isBlank counts it as blank; after a block marker only a space, a tab
// or the line's end may follow, so a '\r' there is an ordinary byte and the
// block does not start. A '\r' reaches the leaf functions mid-line or at the
// end of a last line with no '\n'. The .test corpus cannot hold these inputs.
func TestLeafCarriageReturn(t *testing.T) {
	for _, tc := range []struct {
		line string
		want int
	}{{"#\rx", 0}, {"#\r", 0}, {"# \rx", 1}} {
		if got := headingLevel(tc.line); got != tc.want {
			t.Errorf("headingLevel(%q) = %d, want %d", tc.line, got, tc.want)
		}
	}
	for _, tc := range []struct {
		line string
		want bool
	}{{">\rx", false}, {">\r", false}, {"> \rx", true}} {
		if got := startsBlockQuote(tc.line); got != tc.want {
			t.Errorf("startsBlockQuote(%q) = %v, want %v", tc.line, got, tc.want)
		}
	}
	for _, tc := range []struct {
		line string
		want fenceInfo
	}{
		{"```\rx", fenceInfo{length: 3, char: '`', lang: "\rx"}},
		{"```\r", fenceInfo{length: 3, char: '`', lang: "\r"}},
	} {
		if got, ok := isCodeFence(tc.line); !ok || got != tc.want {
			t.Errorf("isCodeFence(%q) = %#v, %v; want %#v, true", tc.line, got, ok, tc.want)
		}
	}
	for _, line := range []string{"- -\r-", "---\r", "--- \r"} {
		if isThematicBreak(line) {
			t.Errorf("isThematicBreak(%q) = true, want false", line)
		}
	}
	if got, ok := isFencedDivStart(":::\rx"); !ok || got != (divInfo{fence: ":::", class: "\rx"}) {
		t.Errorf("isFencedDivStart(%q) = %#v, %v; want class %q, true", ":::\rx", got, ok, "\rx")
	}
	if divClass("\rx") {
		t.Errorf("divClass(%q) = true, want false", "\rx")
	}
	d, ok := parseDefinitionLabel("[^n]:\rx")
	if want := (definitionLabel{label: "n", rest: "\rx", restCol: 5, isFootnote: true}); !ok || d != want {
		t.Errorf("parseDefinitionLabel(%q) = %#v, %v; want %#v, true", "[^n]:\rx", d, ok, want)
	}
	if d.footnote() || d.refLabel() != "^n" {
		t.Errorf("[^n]:\\rx: footnote() = %v, refLabel() = %q; want false, %q", d.footnote(), d.refLabel(), "^n")
	}
	if d, ok := parseDefinitionLabel("[foo]: \rx"); !ok || !d.spaced {
		t.Errorf("parseDefinitionLabel(%q) = %#v, %v; want spaced, true", "[foo]: \rx", d, ok)
	}
	for _, rest := range []string{"[ ]\rx", "[x]\r"} {
		if isTaskItem(rest) {
			t.Errorf("isTaskItem(%q) = true, want false", rest)
		}
	}
	if m, ok := parseBulletMarker("-\rx"); ok {
		t.Errorf("parseBulletMarker(%q) = %+v, true; want false", "-\rx", m)
	}
	if m, ok := parseBulletMarker("- \rx"); !ok || m.rest != "\rx" {
		t.Errorf("parseBulletMarker(%q) = %+v, %v; want rest %q, true", "- \rx", m, ok, "\rx")
	}
	for _, line := range []string{"1.\rx", "(a)\r"} {
		if m, ok := parseOrderedMarker(line); ok {
			t.Errorf("parseOrderedMarker(%q) = %+v, true; want false", line, m)
		}
	}
	for _, tc := range []struct {
		line string
		want bool
	}{{"| a |\r", false}, {"| a |", true}} {
		if got := isTableRow(tc.line); got != tc.want {
			t.Errorf("isTableRow(%q) = %v, want %v", tc.line, got, tc.want)
		}
	}
	for _, tc := range []struct {
		line string
		want bool
	}{{"\r", true}, {" \r\t", true}, {"\rx", false}} {
		if got := isBlank(tc.line); got != tc.want {
			t.Errorf("isBlank(%q) = %v, want %v", tc.line, got, tc.want)
		}
	}
	if !isClosingCodeFence("```\r", fenceInfo{length: 3, char: '`'}) {
		t.Errorf("isClosingCodeFence(%q) = false, want true", "```\r")
	}
	if !closesDiv(":::\r", 3) {
		t.Errorf("closesDiv(%q, 3) = false, want true", ":::\r")
	}
	for _, src := range []string{"#\rx", ">\rx"} {
		doc := Parse([]byte(src))
		var tags []Tag
		for _, c := range doc.Children {
			tags = append(tags, c.Tag)
		}
		if len(tags) != 1 || tags[0] != Para {
			t.Errorf("Parse(%q) children = %v, want [%s]", src, tags, Para)
		}
	}
}

func TestLeafCountIndent(t *testing.T) {
	for _, tc := range []struct {
		line string
		want int
	}{
		{"", 0}, {"a", 0}, {"  a", 2}, {"\ta", 1}, {"\t\ta", 2}, {" \t a", 3}, {"   ", 3},
	} {
		if got := countIndent(tc.line); got != tc.want {
			t.Errorf("countIndent(%q) = %d, want %d", tc.line, got, tc.want)
		}
	}
}

func TestLeafCountLeadingChar(t *testing.T) {
	for _, tc := range []struct {
		line string
		c    byte
		want int
	}{
		{"", '`', 0}, {"```", '`', 3}, {"``a`", '`', 2}, {" ```", '`', 0}, {":::: x", ':', 4},
	} {
		if got := countLeadingChar(tc.line, tc.c); got != tc.want {
			t.Errorf("countLeadingChar(%q, %q) = %d, want %d", tc.line, tc.c, got, tc.want)
		}
	}
}

func TestLeafIsThematicBreak(t *testing.T) {
	for _, tc := range []struct {
		line string
		want bool
	}{
		{"---", true}, {"***", true}, {"- - -", true}, {"- -\t-", true}, {"*-*", true},
		{"  ---  ", true}, {"\t***", true}, {"-\t-\t-\t", true}, {"----------", true},
		{"--", false}, {"- -", false}, {"-- a", false}, {"___", false}, {"", false}, {"+++", false},
	} {
		if got := isThematicBreak(tc.line); got != tc.want {
			t.Errorf("isThematicBreak(%q) = %v, want %v", tc.line, got, tc.want)
		}
	}
}

func TestLeafIsCodeFence(t *testing.T) {
	for _, tc := range []struct {
		line string
		want fenceInfo
		ok   bool
	}{
		{"```", fenceInfo{length: 3, char: '`'}, true},
		{"~~~~", fenceInfo{length: 4, char: '~'}, true},
		{"``` go", fenceInfo{length: 3, char: '`', lang: "go"}, true},
		{"```\tgo\t", fenceInfo{length: 3, char: '`', lang: "go"}, true},
		{"``` =html", fenceInfo{length: 3, char: '`', lang: "=html"}, true},
		{"  ````python", fenceInfo{length: 4, char: '`', lang: "python", indent: 2}, true},
		{"\t~~~", fenceInfo{length: 3, char: '~', indent: 1}, true},
		{"``", fenceInfo{}, false},
		{"``` two words", fenceInfo{}, false},
		{"``` a`b", fenceInfo{}, false},
		{"~~~ a`b", fenceInfo{}, false},
		{"~~~`", fenceInfo{}, false},
		{"```a```", fenceInfo{}, false},
		{"`~~", fenceInfo{}, false},
		{"text", fenceInfo{}, false},
	} {
		got, ok := isCodeFence(tc.line)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("isCodeFence(%q) = %+v, %v; want %+v, %v", tc.line, got, ok, tc.want, tc.ok)
		}
	}
}

func TestLeafIsClosingCodeFence(t *testing.T) {
	backticks := fenceInfo{length: 3, char: '`'}
	tildes := fenceInfo{length: 4, char: '~'}
	for _, tc := range []struct {
		line string
		f    fenceInfo
		want bool
	}{
		{"```", backticks, true}, {"`````", backticks, true}, {"  ```  ", backticks, true},
		{"\t```\t", backticks, true}, {"``", backticks, false}, {"``` x", backticks, false},
		{"~~~", backticks, false}, {"~~~~", tildes, true}, {"~~~", tildes, false}, {"", backticks, false},
	} {
		if got := isClosingCodeFence(tc.line, tc.f); got != tc.want {
			t.Errorf("isClosingCodeFence(%q, %+v) = %v, want %v", tc.line, tc.f, got, tc.want)
		}
	}
}

func TestLeafHeadingLevel(t *testing.T) {
	for _, tc := range []struct {
		trimmed string
		want    int
	}{
		{"# a", 1}, {"### a", 3}, {"#\ta", 1}, {"#", 1}, {"##", 2}, {"# ", 1},
		{"###### a", 6}, {"######", 6}, {"####### a", 0}, {"#######", 0}, {"########## a", 0},
		{"#a", 0}, {"a #", 0}, {"", 0}, {"\\# a", 0},
	} {
		if got := headingLevel(tc.trimmed); got != tc.want {
			t.Errorf("headingLevel(%q) = %d, want %d", tc.trimmed, got, tc.want)
		}
	}
}

func TestLeafStartsBlockQuote(t *testing.T) {
	for _, tc := range []struct {
		line string
		want bool
	}{
		{">", true}, {"> a", true}, {">\ta", true}, {"  > a", true}, {"\t>", true},
		{">a", false}, {"a > b", false}, {"", false},
	} {
		if got := startsBlockQuote(tc.line); got != tc.want {
			t.Errorf("startsBlockQuote(%q) = %v, want %v", tc.line, got, tc.want)
		}
	}
}

func TestLeafIsFencedDivStart(t *testing.T) {
	for _, tc := range []struct {
		line string
		want divInfo
		ok   bool
	}{
		{":::", divInfo{fence: ":::"}, true},
		{"::::: warning", divInfo{fence: ":::::", class: "warning"}, true},
		{"  :::\tnote\t", divInfo{fence: ":::", class: "note"}, true},
		{"::: two words", divInfo{fence: ":::", class: "two words"}, true},
		{"::", divInfo{}, false},
		{": a", divInfo{}, false},
	} {
		got, ok := isFencedDivStart(tc.line)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("isFencedDivStart(%q) = %+v, %v; want %+v, %v", tc.line, got, ok, tc.want, tc.ok)
		}
	}
}

func TestLeafClosesDiv(t *testing.T) {
	for _, tc := range []struct {
		line     string
		fenceLen int
		want     bool
	}{
		{":::", 3, true}, {"::::", 3, true}, {"  :::\t", 3, true}, {":::", 4, false},
		{"::: note", 3, false}, {"::", 3, false}, {"", 3, false},
	} {
		if got := closesDiv(tc.line, tc.fenceLen); got != tc.want {
			t.Errorf("closesDiv(%q, %d) = %v, want %v", tc.line, tc.fenceLen, got, tc.want)
		}
	}
}

func TestLeafDivClass(t *testing.T) {
	for _, tc := range []struct {
		class string
		want  bool
	}{
		{"", true}, {"note", true}, {"a-b_c9", true}, {"Warning", true},
		{"two words", false}, {"a.b", false}, {"{.x}", false}, {"é", false},
	} {
		if got := divClass(tc.class); got != tc.want {
			t.Errorf("divClass(%q) = %v, want %v", tc.class, got, tc.want)
		}
	}
}

func TestLeafParseDefinitionLabel(t *testing.T) {
	for _, tc := range []struct {
		line string
		want definitionLabel
		ok   bool
	}{
		{"[foo]: http://x", definitionLabel{label: "foo", rest: "http://x", restCol: 7, spaced: true}, true},
		{"[foo]:", definitionLabel{label: "foo", restCol: 6, spaced: true}, true},
		{"[foo]:\t/u  ", definitionLabel{label: "foo", rest: "/u  ", restCol: 7, spaced: true}, true},
		{"[a b]: /u", definitionLabel{label: "a b", rest: "/u", restCol: 7, spaced: true}, true},
		{"[]: /u", definitionLabel{label: "", rest: "/u", restCol: 4, spaced: true}, true},
		{"[^]: /u", definitionLabel{label: "^", rest: "/u", restCol: 5, spaced: true}, true},
		{"[^note]: The text", definitionLabel{label: "note", rest: "The text", restCol: 9, isFootnote: true, spaced: true}, true},
		{"[^note]:", definitionLabel{label: "note", restCol: 8, isFootnote: true, spaced: true}, true},
		{"[^note]:\tx", definitionLabel{label: "note", rest: "x", restCol: 9, isFootnote: true, spaced: true}, true},
		{"[foo]:x", definitionLabel{label: "foo", rest: "x", restCol: 6}, true},
		{"[^n]:x", definitionLabel{label: "n", rest: "x", restCol: 5, isFootnote: true}, true},
		{"[^]:x", definitionLabel{label: "^", rest: "x", restCol: 4}, true},
		{"[foo] : x", definitionLabel{}, false},
		{"[foo", definitionLabel{}, false},
		{"foo]: x", definitionLabel{}, false},
	} {
		got, ok := parseDefinitionLabel(tc.line)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("parseDefinitionLabel(%q) = %+v, %v; want %+v, %v", tc.line, got, ok, tc.want, tc.ok)
		}
	}
}

func TestLeafIsTableRow(t *testing.T) {
	for _, tc := range []struct {
		line string
		want bool
	}{
		{"| a |", true}, {"|a|b|", true}, {"||", true}, {"  | a |  ", true}, {"| a |\t", true},
		{"| `|` |", true}, {"| a \\| b |", true}, {"| a \\\\| b |", true},
		{"| a", false}, {"| a \\|", false}, {"| a\\\\|", false}, {`| a\\\|`, false}, {"| `a |", false}, {"a | b |", false}, {"|", false}, {"", false},
	} {
		if got := isTableRow(tc.line); got != tc.want {
			t.Errorf("isTableRow(%q) = %v, want %v", tc.line, got, tc.want)
		}
	}
}

func TestLeafIsTableSep(t *testing.T) {
	for _, tc := range []struct {
		line string
		want bool
	}{
		{"|---|---|", true}, {"| --- | --- |", true}, {"|:--|--:|:-:|", true}, {"| :--- |", true},
		{"|-|", true}, {"|\t-\t|", true},
		{"| a |", false}, {"|---", false}, {"||", false}, {"| |", false}, {"|:|", false},
		{"|-:-|", false}, {"|- -|", false},
	} {
		if got := isTableSep(tc.line); got != tc.want {
			t.Errorf("isTableSep(%q) = %v, want %v", tc.line, got, tc.want)
		}
	}
}

func TestLeafParseSepAligns(t *testing.T) {
	got := parseSepAligns("| --- | :-- | --: | :-: |")
	want := []CellAlign{AlignDefault, AlignLeft, AlignRight, AlignCenter}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseSepAligns = %v, want %v", got, want)
	}
}

func TestLeafRowCells(t *testing.T) {
	for _, tc := range []struct {
		line string
		want []string
	}{
		{"| a | b |", []string{" a ", " b "}},
		{"|a|", []string{"a"}},
		{"||", []string{""}},
		{"  | a |  ", []string{" a "}},
		{"| `|` | c |", []string{" `|` ", " c "}},
		{"| a \\| b | c |", []string{" a \\| b ", " c "}},
		{"| ``x|y`` |", []string{" ``x|y`` "}},
		{`| a\\| b |`, []string{` a\\| b `}},
		{`| a\\\| b | c |`, []string{` a\\\| b `, ` c `}},
		{"| \\\\`|` |", []string{" \\\\`|` "}},
		{"| \\` | `b` |", []string{" \\` ", " `b` "}},
	} {
		if got := rowCells(tc.line); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("rowCells(%q) = %q, want %q", tc.line, got, tc.want)
		}
	}
}

func TestLeafListStyleHTMLType(t *testing.T) {
	for s, want := range map[listStyle]string{
		styleDecimal: "", styleLowerAlpha: "a", styleUpperAlpha: "A", styleLowerRoman: "i", styleUpperRoman: "I",
	} {
		if got := s.htmlType(); got != want {
			t.Errorf("%d.htmlType() = %q, want %q", s, got, want)
		}
	}
}

func TestLeafIsTaskItem(t *testing.T) {
	for _, tc := range []struct {
		rest string
		want bool
	}{
		{"[ ] a", true}, {"[x] a", true}, {"[X]\ta", true}, {"[ ]", true},
		{"[ ]a", false}, {"[y] a", false}, {"[] a", false}, {"a", false}, {"", false},
	} {
		if got := isTaskItem(tc.rest); got != tc.want {
			t.Errorf("isTaskItem(%q) = %v, want %v", tc.rest, got, tc.want)
		}
	}
}

func TestLeafParseBulletMarker(t *testing.T) {
	for _, tc := range []struct {
		line string
		want marker
		ok   bool
	}{
		{"- a", marker{rest: "a", bullet: '-', markerText: "-"}, true},
		{"-\ta", marker{rest: "a", bullet: '-', markerText: "-"}, true},
		{"+ a", marker{rest: "a", bullet: '+', markerText: "+"}, true},
		{"* [ ] task", marker{rest: "[ ] task", bullet: '*', markerText: "*"}, true},
		{"  - a", marker{rest: "a", bullet: '-', markerText: "-"}, true},
		{"-", marker{bullet: '-', markerText: "-"}, true},
		{"-  ", marker{rest: " ", bullet: '-', markerText: "-"}, true},
		{"+ + +", marker{rest: "+ +", bullet: '+', markerText: "+"}, true},
		{"- - -", marker{}, false},
		{"* * *", marker{}, false},
		{"-a", marker{}, false},
		{"a", marker{}, false},
		{"", marker{}, false},
	} {
		got, ok := parseBulletMarker(tc.line)
		if ok != tc.ok || (ok && !reflect.DeepEqual(got, tc.want)) {
			t.Errorf("parseBulletMarker(%q) = %+v, %v; want %+v, %v", tc.line, got, ok, tc.want, tc.ok)
		}
	}
}

func TestLeafParseOrderedMarker(t *testing.T) {
	for _, tc := range []struct {
		line string
		want marker
		ok   bool
	}{
		{"1. a", marker{rest: "a", markerText: "1", styles: []listStyle{styleDecimal}, delim: delimPeriod}, true},
		{"12) a", marker{rest: "a", markerText: "12", styles: []listStyle{styleDecimal}, delim: delimParen}, true},
		{"(3) a", marker{rest: "a", markerText: "3", styles: []listStyle{styleDecimal}, delim: delimParens}, true},
		{"1.\ta", marker{rest: "a", markerText: "1", styles: []listStyle{styleDecimal}, delim: delimPeriod}, true},
		{"1.", marker{markerText: "1", styles: []listStyle{styleDecimal}, delim: delimPeriod}, true},
		{"a. x", marker{rest: "x", markerText: "a", styles: []listStyle{styleLowerAlpha}, delim: delimPeriod}, true},
		{"B) x", marker{rest: "x", markerText: "B", styles: []listStyle{styleUpperAlpha}, delim: delimParen}, true},
		{"i. x", marker{rest: "x", markerText: "i", styles: []listStyle{styleLowerRoman, styleLowerAlpha}, delim: delimPeriod}, true},
		{"(I) x", marker{rest: "x", markerText: "I", styles: []listStyle{styleUpperRoman, styleUpperAlpha}, delim: delimParens}, true},
		{"iv. x", marker{rest: "x", markerText: "iv", styles: []listStyle{styleLowerRoman}, delim: delimPeriod}, true},
		{"  2. x", marker{rest: "x", markerText: "2", styles: []listStyle{styleDecimal}, delim: delimPeriod}, true},
		{"1.a", marker{}, false},
		{"(1. a", marker{}, false},
		{"1 a", marker{}, false},
		{"ab. x", marker{}, false},
		{"iiv x", marker{}, false},
		{"a", marker{}, false},
		{"", marker{}, false},
	} {
		got, ok := parseOrderedMarker(tc.line)
		if ok != tc.ok || (ok && !reflect.DeepEqual(got, tc.want)) {
			t.Errorf("parseOrderedMarker(%q) = %+v, %v; want %+v, %v", tc.line, got, ok, tc.want, tc.ok)
		}
	}
}

func TestLeafOrderedStyles(t *testing.T) {
	first, _ := parseOrderedMarker("i. x")
	s := orderedStyles{possible: first.styles}
	ii, _ := parseOrderedMarker("ii. x")
	j, _ := parseOrderedMarker("j. x")
	one, _ := parseOrderedMarker("1. x")
	if !s.matches(ii) || !s.matches(j) || s.matches(one) {
		t.Fatalf("styles %v: matches(ii)=%v matches(j)=%v matches(1)=%v; want true true false",
			s.possible, s.matches(ii), s.matches(j), s.matches(one))
	}
	s.accept(ii)
	if want := []listStyle{styleLowerRoman}; !reflect.DeepEqual(s.possible, want) {
		t.Fatalf("after accept(ii): %v, want %v", s.possible, want)
	}
	if s.matches(j) {
		t.Errorf("after accept(ii), j. still matches")
	}
}

func TestLeafGetListStart(t *testing.T) {
	for _, tc := range []struct {
		text string
		s    listStyle
		want int
	}{
		{"1", styleDecimal, 1}, {"7", styleDecimal, 7}, {"0", styleDecimal, 0}, {"007", styleDecimal, 7},
		{"99999999999999999999999", styleDecimal, 1},
		{"a", styleLowerAlpha, 1}, {"c", styleLowerAlpha, 3}, {"C", styleUpperAlpha, 3},
		{"i", styleLowerRoman, 1}, {"iv", styleLowerRoman, 4}, {"XIV", styleUpperRoman, 14},
	} {
		if got := getListStart(tc.text, tc.s); got != tc.want {
			t.Errorf("getListStart(%q, %d) = %d, want %d", tc.text, tc.s, got, tc.want)
		}
	}
}

func TestLeafRomanToNumber(t *testing.T) {
	for s, want := range map[string]int{"i": 1, "iv": 4, "ix": 9, "MCMXC": 1990, "": 1} {
		if got := romanToNumber(s); got != want {
			t.Errorf("romanToNumber(%q) = %d, want %d", s, got, want)
		}
	}
}

// A definition opened with "[^label]:" is a footnote only when a space, a
// tab or the line's end follows the colon; otherwise it is a reference
// labelled "^label" (zjot.test L7462).
func TestLeafDefinitionLabelKind(t *testing.T) {
	for _, tc := range []struct {
		line     string
		footnote bool
		label    string
	}{
		{"[^n]: x", true, "n"}, {"[^n]:", true, "n"}, {"[^n]:x", false, "^n"},
		{"[foo]:x", false, "foo"}, {"[foo]: x", false, "foo"}, {"[^]: x", false, "^"},
	} {
		d, ok := parseDefinitionLabel(tc.line)
		if !ok {
			t.Fatalf("parseDefinitionLabel(%q) failed", tc.line)
		}
		if got := d.footnote(); got != tc.footnote {
			t.Errorf("parseDefinitionLabel(%q).footnote() = %v, want %v", tc.line, got, tc.footnote)
		}
		if got := d.refLabel(); !tc.footnote && got != tc.label {
			t.Errorf("parseDefinitionLabel(%q).refLabel() = %q, want %q", tc.line, got, tc.label)
		}
	}
}

// An ordered list's style, as djot.js prints it: the style's first number
// in the marker's delimiter. parseStyleString reads it back.
func TestLeafStyleString(t *testing.T) {
	for _, tc := range []struct {
		s    listStyle
		d    delim
		want string
	}{
		{styleDecimal, delimPeriod, "1."}, {styleDecimal, delimParen, "1)"}, {styleDecimal, delimParens, "(1)"},
		{styleLowerAlpha, delimPeriod, "a."}, {styleUpperAlpha, delimParen, "A)"},
		{styleLowerRoman, delimParens, "(i)"}, {styleUpperRoman, delimPeriod, "I."},
	} {
		if got := styleString(tc.s, tc.d); got != tc.want {
			t.Errorf("styleString(%d, %d) = %q, want %q", tc.s, tc.d, got, tc.want)
		}
		s, d, ok := parseStyleString(tc.want)
		if !ok || s != tc.s || d != tc.d {
			t.Errorf("parseStyleString(%q) = %d, %d, %v; want %d, %d, true", tc.want, s, d, ok, tc.s, tc.d)
		}
	}
	for _, bad := range []string{"", "-", "1", "(1", "b.", "ii.", "1.."} {
		if _, _, ok := parseStyleString(bad); ok {
			t.Errorf("parseStyleString(%q) accepted", bad)
		}
	}
}
