package gjot

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// specCase is one fenced case from a djot.js .test file: input, a separator
// line ("." for HTML; "!" introduces a Lua filter), expected output. Flags
// after the opening fence: "a" compares AST text, "p" adds source positions.
type specCase struct {
	line      int
	input     string
	expected  string
	ast       bool
	sourcepos bool
	filter    bool
}

func countBackticks(line string) int {
	for i := 0; i < len(line); i++ {
		if line[i] != '`' {
			return i
		}
	}
	return len(line)
}

func parseSpecFile(content string) []specCase {
	lines := strings.Split(content, "\n")
	var cases []specCase
	i := 0
	for i < len(lines) {
		line := strings.TrimSuffix(lines[i], "\r")
		i++
		fence := countBackticks(line)
		if fence < 3 {
			continue
		}
		flags := strings.TrimSpace(line[fence:])
		c := specCase{
			line:      i,
			ast:       strings.Contains(flags, "a"),
			sourcepos: strings.Contains(flags, "p"),
		}

		var input strings.Builder
		sepFound := false
		for i < len(lines) {
			l := strings.TrimSuffix(lines[i], "\r")
			i++
			if l == "." || l == "!" {
				c.filter = l == "!"
				sepFound = true
				break
			}
			input.WriteString(l)
			input.WriteByte('\n')
		}
		if !sepFound {
			break
		}

		var expected strings.Builder
		for i < len(lines) {
			l := strings.TrimSuffix(lines[i], "\r")
			i++
			if countBackticks(l) >= fence {
				break
			}
			expected.WriteString(l)
			expected.WriteByte('\n')
		}
		c.input = input.String()
		c.expected = expected.String()
		cases = append(cases, c)
	}
	return cases
}

// checkCase is the verdict on one non-filter case: its expected output, then
// the writer round trip (written back to Djot and re-rendered, the result must
// not change). A nil error means the case passes.
func checkCase(c specCase) error {
	input := []byte(c.input)
	var got []byte
	if c.ast {
		var posOpts []ParseOption
		if c.sourcepos {
			posOpts = append(posOpts, WithSourcePos())
		}
		got = ASTText(Parse(input, posOpts...))
	} else {
		var err error
		got, err = HTML(Parse(input))
		if err != nil {
			return err
		}
	}
	if string(got) != c.expected {
		return fmt.Errorf("\n---input---\n%s---expected---\n%s---got---\n%s---end---",
			c.input, c.expected, got)
	}

	djot := Djot(Parse(input))
	if c.ast {
		want := ASTText(Parse(input))
		if got := ASTText(Parse(djot)); string(got) != string(want) {
			return fmt.Errorf("round trip: AST changed\n---input---\n%s---written---\n%s---expected ast---\n%s---got---\n%s---end---",
				c.input, djot, want, got)
		}
		return nil
	}
	got, err := HTML(Parse(djot))
	if err != nil {
		return err
	}
	if string(got) != c.expected {
		return fmt.Errorf("round trip: HTML changed\n---input---\n%s---written---\n%s---expected---\n%s---got---\n%s---end---",
			c.input, djot, c.expected, got)
	}
	return nil
}

// TestSpec runs every case of testdata/spec/*.test through checkCase; a
// failing case fails its subtest. It logs the pass count out of the total.
func TestSpec(t *testing.T) {
	files, err := filepath.Glob("testdata/spec/*.test")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no testdata/spec/*.test files found")
	}
	// The counters are only bumped inside subtests that run, so under -run
	// the count line describes the cases selected.
	var total, passing, skipped int
	for _, file := range files {
		base := filepath.Base(file)
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		cases := parseSpecFile(string(content))
		t.Run(strings.TrimSuffix(base, ".test"), func(t *testing.T) {
			for _, c := range cases {
				t.Run(fmt.Sprintf("L%d", c.line), func(t *testing.T) {
					total++
					if c.filter {
						skipped++
						t.Skip("lua filter")
					}
					if err := checkCase(c); err != nil {
						t.Error(err)
						return
					}
					passing++
				})
			}
		})
	}
	t.Logf("corpus: %d of %d cases pass, %d skipped (lua filter)", passing, total, skipped)
}
