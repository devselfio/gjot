package gjot

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// knownFailuresPath lists the corpus cases gjot does not yet pass. Each line
// is "<file>.test L<line>", the line being that of the case's opening fence;
// "#" comments and blank lines are allowed. Entries are sorted by file name,
// then by line number, without duplicates, so the list only ever shrinks by
// deleting lines.
const knownFailuresPath = "testdata/spec/known-failures.txt"

// caseID names one corpus case by file base name and opening-fence line.
type caseID struct {
	File string
	Line int
}

func (c caseID) String() string { return fmt.Sprintf("%s L%d", c.File, c.Line) }

func (c caseID) less(d caseID) bool {
	if c.File != d.File {
		return c.File < d.File
	}
	return c.Line < d.Line
}

var knownFailureEntry = regexp.MustCompile(`^([a-z_]+\.test) L([1-9][0-9]*)$`)

// parseKnownFailures parses the known-failures list. A malformed, unsorted or
// duplicate entry is an error naming its line.
func parseKnownFailures(content string) ([]caseID, error) {
	var ids []caseID
	for n, line := range strings.Split(content, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := knownFailureEntry.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("line %d: %q is not \"<file>.test L<line>\"", n+1, line)
		}
		num, err := strconv.Atoi(m[2])
		if err != nil {
			return nil, fmt.Errorf("line %d: %v", n+1, err)
		}
		id := caseID{m[1], num}
		if len(ids) > 0 {
			prev := ids[len(ids)-1]
			if id == prev {
				return nil, fmt.Errorf("line %d: duplicate entry %q", n+1, id)
			}
			if id.less(prev) {
				return nil, fmt.Errorf("line %d: %q is out of order after %q (sort by file, then line)", n+1, id, prev)
			}
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// loadKnownFailures reads knownFailuresPath. A missing or malformed list is
// fatal: the file exists until the corpus passes in full.
func loadKnownFailures(t *testing.T) map[caseID]bool {
	t.Helper()
	content, err := os.ReadFile(knownFailuresPath)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := parseKnownFailures(string(content))
	if err != nil {
		t.Fatalf("%s: %v", knownFailuresPath, err)
	}
	known := make(map[caseID]bool, len(ids))
	for _, id := range ids {
		known[id] = true
	}
	return known
}

// outcome is the verdict on one corpus case given the known-failures list.
type outcome int

const (
	passed            outcome = iota // unlisted, passes
	knownFailure                     // listed, fails
	unexpectedPass                   // listed, passes: delete the entry
	unexpectedFailure                // unlisted, fails: add the entry or fix it
)

func (o outcome) String() string {
	return [...]string{"passed", "knownFailure", "unexpectedPass", "unexpectedFailure"}[o]
}

// classify is the verdict on a case from whether it is listed and whether
// checkCase returned an error.
func classify(listed bool, err error) outcome {
	switch {
	case listed && err != nil:
		return knownFailure
	case listed:
		return unexpectedPass
	case err == nil:
		return passed
	default:
		return unexpectedFailure
	}
}

func TestParseKnownFailures(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []caseID
		wantErr string // substring of the error; "" means no error
	}{
		{name: "empty", content: "", want: nil},
		{name: "comments and blanks only", content: "# header\n\n  \n# more\n", want: nil},
		{
			name:    "sorted entries across files",
			content: "# list\ngrid.test L3\ngrid.test L40\n\nzjot.test L1\n",
			want:    []caseID{{"grid.test", 3}, {"grid.test", 40}, {"zjot.test", 1}},
		},
		{
			name:    "numeric line order",
			content: "zjot.test L9\nzjot.test L10\n",
			want:    []caseID{{"zjot.test", 9}, {"zjot.test", 10}},
		},
		{name: "duplicate", content: "zjot.test L9\nzjot.test L9\n", wantErr: "line 2"},
		{name: "out of order by line", content: "zjot.test L10\nzjot.test L9\n", wantErr: "line 2"},
		{name: "out of order by file", content: "zjot.test L1\ngrid.test L1\n", wantErr: "line 2"},
		{name: "line zero", content: "zjot.test L0\n", wantErr: "line 1"},
		{name: "missing .test", content: "zjot L12\n", wantErr: "line 1"},
		{name: "trailing text", content: "# c\nzjot.test L12 roundtrip\n", wantErr: "line 2"},
		{name: "leading zero", content: "zjot.test L012\n", wantErr: "line 1"},
		{name: "no L", content: "zjot.test 12\n", wantErr: "line 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseKnownFailures(tt.content)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("got %v, want an error containing %q", got, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCaseIDString(t *testing.T) {
	if got, want := (caseID{"zjot.test", 1234}).String(), "zjot.test L1234"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestClassify(t *testing.T) {
	fail := errors.New("mismatch")
	tests := []struct {
		listed bool
		err    error
		want   outcome
	}{
		{listed: false, err: nil, want: passed},
		{listed: true, err: fail, want: knownFailure},
		{listed: true, err: nil, want: unexpectedPass},
		{listed: false, err: fail, want: unexpectedFailure},
	}
	for _, tt := range tests {
		if got := classify(tt.listed, tt.err); got != tt.want {
			t.Errorf("classify(%v, %v) = %v, want %v", tt.listed, tt.err, got, tt.want)
		}
	}
}
