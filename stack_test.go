package gjot

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// stackFiles names the corpus files the container stack is claimed on: in
// each, every case the window parser passes must pass under withStack().
// Slice 2 of farraway/gridapp#12: the files whose inputs hold no list marker
// and no table row. Cases in other files are tallied only.
var stackFiles = map[string]bool{
	"block_quote.test":        true,
	"code_blocks.test":        true,
	"emphasis.test":           true,
	"escapes.test":            true,
	"fenced_divs.test":        true,
	"footnotes.test":          true,
	"headings.test":           true,
	"insert_delete_mark.test": true,
	"math.test":               true,
	"para.test":               true,
	"raw.test":                true,
	"smart.test":              true,
	"spans.test":              true,
	"super_subscript.test":    true,
	"symb.test":               true,
	"verbatim.test":           true,
}

// TestSpecStack runs every non-filter corpus case through checkCase under
// the container stack and logs a tally per file and in total. A case in a
// stackFiles file that the window parser passes (it is not on the
// known-failures list) must pass under the stack too.
func TestSpecStack(t *testing.T) {
	files, err := filepath.Glob("testdata/spec/*.test")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no testdata/spec/*.test files found")
	}
	sort.Strings(files)
	known := loadKnownFailures(t)
	var total, passing int
	for _, file := range files {
		base := filepath.Base(file)
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var fileTotal, filePassing int
		for _, c := range parseSpecFile(string(content)) {
			if c.filter {
				continue
			}
			fileTotal++
			err := checkStackCase(c)
			if err == nil {
				filePassing++
				continue
			}
			id := caseID{base, c.line}
			if stackFiles[base] && !known[id] {
				t.Errorf("%s passes under the window parser and fails under the stack:%v", id, err)
			}
		}
		total += fileTotal
		passing += filePassing
		t.Logf("stack: %s: %d of %d pass", base, filePassing, fileTotal)
	}
	t.Logf("stack: total: %d of %d pass", passing, total)
}

// checkStackCase is checkCase under the stack, a panic reported as a failure
// so one case cannot hide the tally of the rest.
func checkStackCase(c specCase) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return checkCase(c, withStack())
}

func TestStackUnits(t *testing.T) {
	t.Run("splitLines", func(t *testing.T) {
		pos := func(line, col, off int) SourcePos { return SourcePos{Line: line, Col: col, Offset: off} }
		cases := []struct {
			name  string
			input string
			want  []srcLine
		}{
			{"empty input", "", nil},
			{"no trailing newline", "a\nbc", []srcLine{
				{text: "a", pos: pos(1, 1, 0), eol: pos(2, 0, 1), newline: true},
				{text: "bc", pos: pos(2, 1, 2), eol: pos(2, 3, 4), newline: false, newlineBefore: 1},
			}},
			{"CRLF", "a\r\n\r\nb\r\n", []srcLine{
				{text: "a", pos: pos(1, 1, 0), eol: pos(2, 0, 2), newline: true},
				{text: "", pos: pos(2, 1, 3), eol: pos(3, 0, 4), newline: true, newlineBefore: 2},
				{text: "b", pos: pos(3, 1, 5), eol: pos(4, 0, 7), newline: true, newlineBefore: 4},
			}},
		}
		for _, c := range cases {
			got := splitLines(c.input)
			if fmt.Sprint(got) != fmt.Sprint(c.want) {
				t.Errorf("%s: splitLines(%q)\n got %+v\nwant %+v", c.name, c.input, got, c.want)
			}
		}
	})

	// Each case is a corpus case's input and expected HTML, verbatim.
	html := []struct {
		name, from, input, want string
	}{
		{"attributes-complete-on-one-line", "attributes.test L146",
			"{#id}\n{key=val}\n{.foo .bar}\n{key=val2}\n{.baz}\n{#id2}\nOkay\n",
			"<p id=\"id2\" key=\"val2\" class=\"foo bar baz\">Okay</p>\n"},
		{"attributes-continued-over-indented-lines", "attributes.test L124",
			"{#id .class\n  style=\"color:red\"}\nA paragraph\n",
			"<p id=\"id\" class=\"class\" style=\"color:red\">A paragraph</p>\n"},
		{"attributes-failed-become-a-paragraph", "zjot.test L5797",
			"{.a\n.b}\npara\n",
			"<p>{.a\n.b}\npara</p>\n"},
		{"attributes-failed-alone-in-a-paragraph", "zjot.test L5829",
			"{.a\n.b}\n\npara\n",
			"<p>{.a\n.b}</p>\n<p>para</p>\n"},
		{"ids-where-they-stand", "zjot.test L206",
			"# Foo bar\n\n{#Foo-bar}\nParagraph\n",
			"<section id=\"Foo-bar\">\n<h1>Foo bar</h1>\n<p id=\"Foo-bar\">Paragraph</p>\n</section>\n"},
	}
	for _, c := range html {
		t.Run(c.name, func(t *testing.T) {
			got, err := HTML(Parse([]byte(c.input), withStack()))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != c.want {
				t.Errorf("%s\n---input---\n%s---want---\n%s---got---\n%s", c.from, c.input, c.want, got)
			}
		})
	}
}

// TestStackOffIsWindow pins that Parse without withStack() is the window
// parser: the default path is untouched by the switch.
func TestStackOffIsWindow(t *testing.T) {
	in := "# Foo bar\n\n{#Foo-bar}\nParagraph\n"
	a := string(ASTText(Parse([]byte(in))))
	b := string(ASTText(newParser(in, newSharedState()).parseDoc()))
	if a != b {
		t.Errorf("Parse without withStack differs from the window parser:\n%s\n%s", a, b)
	}
}
