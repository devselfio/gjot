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
		{"~~~ a`b", fenceInfo{length: 3, char: '~', lang: "a`b"}, true},
		{"``", fenceInfo{}, false},
		{"``` two words", fenceInfo{}, false},
		{"``` a`b", fenceInfo{}, false},
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
		{"[foo]: http://x", definitionLabel{label: "foo", rest: "http://x", restCol: 7}, true},
		{"[foo]:", definitionLabel{label: "foo", restCol: 6}, true},
		{"[foo]:\t/u  ", definitionLabel{label: "foo", rest: "/u  ", restCol: 7}, true},
		{"[a b]: /u", definitionLabel{label: "a b", rest: "/u", restCol: 7}, true},
		{"[]: /u", definitionLabel{label: "", rest: "/u", restCol: 4}, true},
		{"[^]: /u", definitionLabel{label: "^", rest: "/u", restCol: 5}, true},
		{"[^note]: The text", definitionLabel{label: "note", rest: "The text", restCol: 9, isFootnote: true}, true},
		{"[^note]:", definitionLabel{label: "note", restCol: 8, isFootnote: true}, true},
		{"[^note]:\tx", definitionLabel{label: "note", rest: "x", restCol: 9, isFootnote: true}, true},
		{"[foo]:x", definitionLabel{}, false},
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
		{"| `|` |", true}, {"| a \\| b |", true},
		{"| a", false}, {"| a \\|", false}, {"| `a |", false}, {"a | b |", false}, {"|", false}, {"", false},
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
