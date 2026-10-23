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

func TestSpec(t *testing.T) {
	files, err := filepath.Glob("testdata/spec/*.test")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no testdata/spec/*.test files found")
	}
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".test")
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(name, func(t *testing.T) {
			for _, c := range parseSpecFile(string(content)) {
				t.Run(fmt.Sprintf("L%d", c.line), func(t *testing.T) {
					if c.filter {
						t.Skip("lua filter")
					}
					var got []byte
					if c.ast {
						var opts []ParseOption
						if c.sourcepos {
							opts = append(opts, WithSourcePos())
						}
						got = ASTText(Parse([]byte(c.input), opts...))
					} else {
						got, err = HTML(Parse([]byte(c.input)))
						if err != nil {
							t.Fatal(err)
						}
					}
					if string(got) != c.expected {
						t.Errorf("%s:%d\n---input---\n%s---expected---\n%s---got---\n%s---end---",
							file, c.line, c.input, c.expected, got)
					}
				})
			}
		})
	}
}
