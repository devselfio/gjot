// Command driftcheck checks the vendored corpus under testdata/spec against
// its checked-in record (SHA256SUMS) and, when the network answers, against
// zjot's test/ directory at the pinned tag, and exits non-zero when they
// differ.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const usage = `Usage: driftcheck [-dir DIR] [-record | REF]

DIR defaults to testdata/spec. The repository and the pin come from the
"Repository:" and "Ref:" lines of DIR/ORIGIN.md; the pin must be a tag of
the form vN.N.N, never a branch or a commit.

With no argument, check the pin: compare DIR/*.test with DIR/SHA256SUMS,
which must record the pinned tag, then fetch zjot's test/*.test at the tag
and compare with that too. Without a network the fetch is skipped with a
notice and the record alone decides.

-record   fetch zjot at the pinned tag and, if DIR/*.test match it, write
          DIR/SHA256SUMS. Needs the network; writes nothing on any failure.
REF       measure the gap to another ref (any ref, not only a tag) without
          moving the pin. Needs the network; the record is not consulted.

Exit 0 when everything matches, 1 when anything differs (including a pin
that SHA256SUMS does not record), 2 when the check cannot run.
`

// recordName is the record of the corpus at the pin, beside ORIGIN.md.
const recordName = "SHA256SUMS"

var tagRE = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)

// isTag reports whether ref has zjot's tag form, vN.N.N.
func isTag(ref string) bool { return tagRE.MatchString(ref) }

// origin is the pin recorded in ORIGIN.md.
type origin struct {
	Repository string
	Ref        string
}

// parseOrigin reads the "Repository: <url>" and "Ref: <ref>" lines of
// ORIGIN.md. Each key must appear exactly once at the start of a line,
// followed by ": " and a non-empty value with no spaces.
func parseOrigin(content []byte) (origin, error) {
	vals := map[string]string{}
	count := map[string]int{}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimRight(line, "\r")
		for _, key := range []string{"Repository", "Ref"} {
			if !strings.HasPrefix(line, key+":") {
				continue
			}
			count[key]++
			v := strings.TrimPrefix(line, key+": ")
			if !strings.HasPrefix(line, key+": ") || v == "" || strings.ContainsAny(v, " \t") {
				return origin{}, fmt.Errorf("ORIGIN.md: %q line must be %q followed by a value without spaces", key, key+": ")
			}
			vals[key] = v
		}
	}
	for _, key := range []string{"Repository", "Ref"} {
		switch count[key] {
		case 0:
			return origin{}, fmt.Errorf("ORIGIN.md: no %q line", key)
		case 1:
		default:
			return origin{}, fmt.Errorf("ORIGIN.md: %q line appears %d times", key, count[key])
		}
	}
	return origin{Repository: vals["Repository"], Ref: vals["Ref"]}, nil
}

// readLocal returns the regular files named *.test directly in dir.
func readLocal(dir string) (map[string][]byte, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".test") || !e.Type().IsRegular() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		out[e.Name()] = b
	}
	return out, nil
}

// errOffline marks a fetch that got no HTTP response at all: no network, no
// DNS, a refused connection. Any HTTP response, whatever its status, is not
// offline.
var errOffline = errors.New("offline")

func archiveURL(repository, ref string) string {
	return repository + "/archive/" + ref + ".tar.gz"
}

// fetchUpstream downloads the archive of ref and returns its corpus.
func fetchUpstream(client *http.Client, repository, ref string) (map[string][]byte, error) {
	url := archiveURL(repository, ref)
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errOffline, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, fmt.Errorf("ref %s not found at %s", ref, url)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("fetching %s: %s", url, resp.Status)
	}
	return readCorpusTar(resp.Body)
}

// readCorpusTar reads a gzipped tar with one root directory and returns the
// regular files whose path is exactly <root>/test/<name>.test.
func readCorpusTar(r io.Reader) (map[string][]byte, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("bad archive: %w", err)
	}
	tr := tar.NewReader(gz)
	out := map[string][]byte{}
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("bad archive: %w", err)
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		parts := strings.Split(h.Name, "/")
		if len(parts) != 3 || parts[1] != "test" || !strings.HasSuffix(parts[2], ".test") {
			continue
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("bad archive: %w", err)
		}
		out[parts[2]] = b
	}
	// The tar ends before the gzip stream does. Read to gzip's EOF so its
	// trailer (CRC and length) is verified; Close would not do that.
	if _, err := io.Copy(io.Discard, gz); err != nil {
		return nil, fmt.Errorf("bad archive: %w", err)
	}
	return out, nil
}

// change is one difference, seen from the vendored copy.
type change struct {
	Name string
	Kind string // "added" (upstream only), "removed" (local only), "changed"
}

// compare returns the differences sorted by Name.
func compare(local, upstream map[string][]byte) []change {
	var out []change
	for n, lb := range local {
		ub, ok := upstream[n]
		switch {
		case !ok:
			out = append(out, change{n, "removed"})
		case !bytes.Equal(lb, ub):
			out = append(out, change{n, "changed"})
		}
	}
	for n := range upstream {
		if _, ok := local[n]; !ok {
			out = append(out, change{n, "added"})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// record is the parsed SHA256SUMS: the tag it records and name → hex digest.
type record struct {
	Ref  string
	Sums map[string]string
}

var sumLineRE = regexp.MustCompile(`^([0-9a-f]{64})  ([^ \t]+)$`)

// digest returns the lowercase hex SHA-256 of each file.
func digest(files map[string][]byte) map[string]string {
	out := make(map[string]string, len(files))
	for n, b := range files {
		s := sha256.Sum256(b)
		out[n] = hex.EncodeToString(s[:])
	}
	return out
}

// parseRecord reads a record: a first line "# Ref: <ref>", then one
// "<sha256 hex>  <name>" line per file, in sha256sum's output form. Blank
// lines are ignored.
func parseRecord(content []byte) (record, error) {
	rec := record{Sums: map[string]string{}}
	header := false
	for i, line := range strings.Split(string(content), "\n") {
		if line == "" {
			continue
		}
		if !header {
			ref, ok := strings.CutPrefix(line, "# Ref: ")
			if !ok || ref == "" || strings.ContainsAny(ref, " \t") {
				return record{}, fmt.Errorf("%s: first line must be %q followed by the recorded tag", recordName, "# Ref: ")
			}
			rec.Ref = ref
			header = true
			continue
		}
		m := sumLineRE.FindStringSubmatch(line)
		if m == nil {
			return record{}, fmt.Errorf("%s:%d: want %q, got %q", recordName, i+1, "<64 lowercase hex>  <name>", line)
		}
		if _, dup := rec.Sums[m[2]]; dup {
			return record{}, fmt.Errorf("%s:%d: %s listed twice", recordName, i+1, m[2])
		}
		rec.Sums[m[2]] = m[1]
	}
	if !header {
		return record{}, fmt.Errorf("%s: no %q line", recordName, "# Ref: ")
	}
	if len(rec.Sums) == 0 {
		return record{}, fmt.Errorf("%s: records no files", recordName)
	}
	return rec, nil
}

// formatRecord returns the canonical record of sums at ref: the header, then
// the files sorted bytewise by name, two spaces, LF line ends.
func formatRecord(ref string, sums map[string]string) []byte {
	names := make([]string, 0, len(sums))
	for n := range sums {
		names = append(names, n)
	}
	sort.Strings(names)
	var b bytes.Buffer
	fmt.Fprintf(&b, "# Ref: %s\n", ref)
	for _, n := range names {
		fmt.Fprintf(&b, "%s  %s\n", sums[n], n)
	}
	return b.Bytes()
}

// readRecord reads and parses dir/SHA256SUMS.
func readRecord(dir string) (record, error) {
	b, err := os.ReadFile(filepath.Join(dir, recordName))
	if errors.Is(err, os.ErrNotExist) {
		return record{}, fmt.Errorf("no %s in %s; run driftcheck -record to write it from zjot at the pinned tag", recordName, dir)
	}
	if err != nil {
		return record{}, err
	}
	return parseRecord(b)
}

// compareSums is compare over digests: added is record-only, removed is
// local-only, changed is a different digest.
func compareSums(local, rec map[string]string) []change {
	asBytes := func(m map[string]string) map[string][]byte {
		out := make(map[string][]byte, len(m))
		for n, s := range m {
			out[n] = []byte(s)
		}
		return out
	}
	return compare(asBytes(local), asBytes(rec))
}

func run(args []string, client *http.Client, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("driftcheck", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dir := fs.String("dir", "testdata/spec", "")
	doRecord := fs.Bool("record", false, "")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage)
			return 0
		}
		fmt.Fprintf(stderr, "driftcheck: %v\n%s", err, usage)
		return 2
	}
	if fs.NArg() > 1 {
		fmt.Fprintf(stderr, "driftcheck: at most one REF\n%s", usage)
		return 2
	}
	if *doRecord && fs.NArg() == 1 {
		fmt.Fprintf(stderr, "driftcheck: -record takes no REF: a measurement does not move the pin\n%s", usage)
		return 2
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "driftcheck: %v\n", err)
		return 2
	}
	content, err := os.ReadFile(filepath.Join(*dir, "ORIGIN.md"))
	if err != nil {
		return fail(err)
	}
	o, err := parseOrigin(content)
	if err != nil {
		return fail(err)
	}
	if !isTag(o.Ref) {
		return fail(fmt.Errorf("ORIGIN.md: Ref %q is not a tag (zjot's tags are vN.N.N); the pin must be a tag", o.Ref))
	}
	local, err := readLocal(*dir)
	if err != nil {
		return fail(err)
	}
	if len(local) == 0 {
		return fail(fmt.Errorf("no *.test files in %s", *dir))
	}

	// fetch returns zjot's corpus at ref; a non-empty corpus or an error.
	fetch := func(ref string) (map[string][]byte, error) {
		upstream, err := fetchUpstream(client, o.Repository, ref)
		if err != nil {
			return nil, err
		}
		if len(upstream) == 0 {
			return nil, fmt.Errorf("no *.test files found in test/ at %s of %s", ref, o.Repository)
		}
		return upstream, nil
	}
	// report prints the comparison with zjot at ref and reports whether
	// they match.
	report := func(ref string, upstream map[string][]byte) bool {
		changes := compare(local, upstream)
		if len(changes) == 0 {
			fmt.Fprintf(stdout, "driftcheck: %s matches zjot at %s (%d files)\n", *dir, ref, len(local))
			return true
		}
		for _, c := range changes {
			fmt.Fprintf(stdout, "%s\t%s\n", c.Kind, c.Name)
		}
		fmt.Fprintf(stdout, "driftcheck: %d files differ from zjot at %s (upstream view: added is upstream-only, removed is local-only)\n", len(changes), ref)
		return false
	}

	// A measurement of another ref: online only, the record not consulted.
	if fs.NArg() == 1 {
		ref := fs.Arg(0)
		upstream, err := fetch(ref)
		if errors.Is(err, errOffline) {
			return fail(fmt.Errorf("cannot measure %s offline: %v", ref, err))
		}
		if err != nil {
			return fail(err)
		}
		if report(ref, upstream) {
			return 0
		}
		return 1
	}

	// -record: write the record only from a corpus proven equal to the tag.
	if *doRecord {
		upstream, err := fetch(o.Ref)
		if err != nil {
			return fail(err)
		}
		if changes := compare(local, upstream); len(changes) > 0 {
			for _, c := range changes {
				fmt.Fprintf(stdout, "%s\t%s\n", c.Kind, c.Name)
			}
			fmt.Fprintf(stdout, "driftcheck: not recorded: %d files differ from zjot at %s\n", len(changes), o.Ref)
			return 1
		}
		path := filepath.Join(*dir, recordName)
		if err := os.WriteFile(path, formatRecord(o.Ref, digest(local)), 0o644); err != nil {
			return fail(err)
		}
		fmt.Fprintf(stdout, "driftcheck: recorded %d files at %s in %s\n", len(local), o.Ref, path)
		return 0
	}

	// The pin check. Offline part: the record.
	red := false
	rec, err := readRecord(*dir)
	if err != nil {
		return fail(err)
	}
	if rec.Ref != o.Ref {
		fmt.Fprintf(stdout, "driftcheck: %s records %s but ORIGIN.md pins %s; run driftcheck -record after updating the corpus\n", recordName, rec.Ref, o.Ref)
		red = true
	} else if changes := compareSums(digest(local), rec.Sums); len(changes) > 0 {
		for _, c := range changes {
			fmt.Fprintf(stdout, "%s\t%s\n", c.Kind, c.Name)
		}
		fmt.Fprintf(stdout, "driftcheck: %d files differ from %s at %s (added is record-only, removed is local-only)\n", len(changes), recordName, rec.Ref)
		red = true
	} else {
		fmt.Fprintf(stdout, "driftcheck: %s matches %s at %s (%d files)\n", *dir, recordName, rec.Ref, len(local))
	}
	// Online part: zjot at the tag, when the network answers.
	upstream, err := fetch(o.Ref)
	switch {
	case errors.Is(err, errOffline):
		fmt.Fprintf(stderr, "driftcheck: offline (%v); zjot at %s not fetched, record only\n", err, o.Ref)
	case err != nil:
		return fail(err)
	case !report(o.Ref, upstream):
		red = true
	}
	if red {
		return 1
	}
	return 0
}

func main() {
	os.Exit(run(os.Args[1:], &http.Client{Timeout: 2 * time.Minute}, os.Stdout, os.Stderr))
}
