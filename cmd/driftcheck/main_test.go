package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

// buildTar makes a gzipped tar like GitHub's: a pax global header first,
// then entries in sorted order. Paths ending in "/" are directories.
func buildTar(t *testing.T, files map[string][]byte, order []string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeXGlobalHeader, Name: "pax_global_header", PAXRecords: map[string]string{"comment": "abc"}}); err != nil {
		t.Fatal(err)
	}
	for _, p := range order {
		if strings.HasSuffix(p, "/") {
			if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: p, Mode: 0o755}); err != nil {
				t.Fatal(err)
			}
			continue
		}
		b := files[p]
		if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: p, Mode: 0o644, Size: int64(len(b))}); err != nil {
			t.Fatal(err)
		}
		tw.Write(b)
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

// corpusTar builds an archive with root/test/<name> for each corpus file.
func corpusTar(t *testing.T, root string, corpus map[string]string) []byte {
	files := map[string][]byte{}
	order := []string{root + "/", root + "/README.md", root + "/test/"}
	files[root+"/README.md"] = []byte("hi")
	for n, c := range corpus {
		files[root+"/test/"+n] = []byte(c)
		order = append(order, root+"/test/"+n)
	}
	return buildTar(t, files, order)
}

func serve(t *testing.T, refs map[string][]byte) *httptest.Server {
	s, _ := serveCounting(t, refs)
	return s
}

// serveCounting is serve, plus a count of the requests that reached it.
func serveCounting(t *testing.T, refs map[string][]byte) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	hits := new(atomic.Int64)
	mux := http.NewServeMux()
	mux.HandleFunc("/archive/", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		ref := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/archive/"), ".tar.gz")
		b, ok := refs[ref]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(b)
	})
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s, hits
}

// downTransport fails every request the way a machine without a network
// does, without dialling anything.
type downTransport struct{}

func (downTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("network is unreachable")}
}

func offlineClient() *http.Client { return &http.Client{Transport: downTransport{}} }

// writeDir writes ORIGIN.md pinning ref, the corpus, and a SHA256SUMS
// recording the corpus at ref.
func writeDir(t *testing.T, repo, ref string, corpus map[string]string) string {
	t.Helper()
	dir := writeDirNoRecord(t, repo, ref, corpus)
	writeRecord(t, dir, ref, corpus)
	return dir
}

// writeRecord writes dir/SHA256SUMS recording corpus at ref.
func writeRecord(t *testing.T, dir, ref string, corpus map[string]string) {
	t.Helper()
	files := map[string][]byte{}
	for n, c := range corpus {
		files[n] = []byte(c)
	}
	if err := os.WriteFile(filepath.Join(dir, recordName), formatRecord(ref, digest(files)), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeDirNoRecord writes ORIGIN.md pinning ref and the corpus, no record.
func writeDirNoRecord(t *testing.T, repo, ref string, corpus map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	origin := "# Corpus origin\n\nRepository: " + repo + "\nRef: " + ref + "\n"
	if err := os.WriteFile(filepath.Join(dir, "ORIGIN.md"), []byte(origin), 0o644); err != nil {
		t.Fatal(err)
	}
	for n, c := range corpus {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	m := map[string]string{}
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			b, _ := os.ReadFile(p)
			m[p] = string(b)
		}
		return nil
	})
	return m
}

func TestParseOriginRealFile(t *testing.T) {
	b, err := os.ReadFile("../../testdata/spec/ORIGIN.md")
	if err != nil {
		t.Fatal(err)
	}
	o, err := parseOrigin(b)
	if err != nil {
		t.Fatal(err)
	}
	if o.Repository != "https://github.com/quiteclose/zjot" || o.Ref != "v2.0.0" {
		t.Fatalf("got %+v", o)
	}
}

// TestWorkflowRunsDriftcheck keeps the drift check in CI: the check job
// must run make spec-drift after make spec-count. A plain string search,
// since gjot has no YAML dependency.
func TestWorkflowRunsDriftcheck(t *testing.T) {
	b, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	count := strings.Index(s, "run: make spec-count")
	if count < 0 {
		t.Fatal(`"run: make spec-count" not found in .github/workflows/ci.yml`)
	}
	drift := strings.Index(s, "run: make spec-drift")
	if drift < 0 {
		t.Fatal(`"run: make spec-drift" not found in .github/workflows/ci.yml`)
	}
	if drift < count {
		t.Fatal(`"run: make spec-drift" comes before "run: make spec-count" in .github/workflows/ci.yml`)
	}
}

func TestParseOriginErrors(t *testing.T) {
	cases := []struct{ name, in, key string }{
		{"missing ref", "Repository: http://x\n", "Ref"},
		{"missing repo", "Ref: v1\n", "Repository"},
		{"duplicate", "Repository: http://x\nRef: a\nRef: b\n", "Ref"},
		{"empty value", "Repository: http://x\nRef: \n", "Ref"},
		{"space in value", "Repository: http://x\nRef: a b\n", "Ref"},
	}
	for _, c := range cases {
		_, err := parseOrigin([]byte(c.in))
		if err == nil || !strings.Contains(err.Error(), c.key) {
			t.Errorf("%s: err = %v, want mention of %s", c.name, err, c.key)
		}
	}
}

func TestReadCorpusTar(t *testing.T) {
	for _, root := range []string{"zjot-2.0.0", "zjot-ae01cd41bbd4a358991711d5ddeae5efb606c5f2"} {
		files := map[string][]byte{
			root + "/README.md":       []byte("r"),
			root + "/test/a.test":     []byte("A"),
			root + "/test/b.test":     []byte("B"),
			root + "/test/cli/x.test": []byte("X"),
			root + "/test/tools/y.dj": []byte("Y"),
			root + "/src/z.test":      []byte("Z"),
		}
		order := []string{root + "/", root + "/README.md", root + "/test/", root + "/test/a.test", root + "/test/b.test", root + "/test/cli/x.test", root + "/test/tools/y.dj", root + "/src/z.test"}
		got, err := readCorpusTar(bytes.NewReader(buildTar(t, files, order)))
		if err != nil {
			t.Fatal(err)
		}
		want := map[string][]byte{"a.test": []byte("A"), "b.test": []byte("B")}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %v", root, got)
		}
	}
}

func TestReadLocal(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.test"), []byte("A"), 0o644)
	os.WriteFile(filepath.Join(dir, "ORIGIN.md"), []byte("o"), 0o644)
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "c.test"), []byte("C"), 0o644)
	os.Mkdir(filepath.Join(dir, "d.test"), 0o755)
	got, err := readLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string][]byte{"a.test": []byte("A")}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestCompare(t *testing.T) {
	local := map[string][]byte{"a": []byte("1"), "b": []byte("1"), "c": []byte("1")}
	up := map[string][]byte{"b": []byte("1"), "c": []byte("2"), "d": []byte("1")}
	want := []change{{"a", "removed"}, {"c", "changed"}, {"d", "added"}}
	if got := compare(local, up); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
	if got := compare(local, local); len(got) != 0 {
		t.Fatalf("identical: got %v", got)
	}
}

func TestRunMatch(t *testing.T) {
	c := map[string]string{"a.test": "A", "b.test": "B"}
	s := serve(t, map[string][]byte{"v1.0.0": corpusTar(t, "zjot-1.0.0", c)})
	dir := writeDir(t, s.URL, "v1.0.0", c)
	var out, errb bytes.Buffer
	if code := run([]string{"-dir", dir}, s.Client(), &out, &errb); code != 0 {
		t.Fatalf("code %d, stderr %q", code, errb.String())
	}
	for _, w := range []string{"matches", "v1.0.0", "(2 files)", "matches SHA256SUMS at v1.0.0", "matches zjot at v1.0.0"} {
		if !strings.Contains(out.String(), w) {
			t.Errorf("stdout %q lacks %q", out.String(), w)
		}
	}
	if errb.Len() != 0 {
		t.Errorf("stderr %q", errb.String())
	}
}

func TestRunDrift(t *testing.T) {
	local := map[string]string{"keep.test": "K", "gone.test": "G", "diff.test": "old"}
	up := map[string]string{"keep.test": "K", "new.test": "N", "diff.test": "new"}
	s := serve(t, map[string][]byte{"v1.0.0": corpusTar(t, "zjot-1.0.0", up)})
	dir := writeDir(t, s.URL, "v1.0.0", local)
	before := snapshot(t, dir)
	var out, errb bytes.Buffer
	if code := run([]string{"-dir", dir}, s.Client(), &out, &errb); code != 1 {
		t.Fatalf("code %d, stderr %q", code, errb.String())
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 5 || !strings.Contains(lines[0], "matches SHA256SUMS at v1.0.0") {
		t.Fatalf("stdout %q", out.String())
	}
	lines = lines[1:]
	if len(lines) != 4 || lines[0] != "changed\tdiff.test" || lines[1] != "removed\tgone.test" || lines[2] != "added\tnew.test" || !strings.HasPrefix(lines[3], "driftcheck: 3 files differ") {
		t.Fatalf("stdout %q", out.String())
	}
	if !reflect.DeepEqual(before, snapshot(t, dir)) {
		t.Error("directory changed")
	}
}

func TestRunOtherRef(t *testing.T) {
	c := map[string]string{"a.test": "A"}
	c2 := map[string]string{"a.test": "A", "extra.test": "E"}
	s := serve(t, map[string][]byte{"v1.0.0": corpusTar(t, "r", c), "v2.0.0": corpusTar(t, "r", c2)})
	dir := writeDir(t, s.URL, "v1.0.0", c)
	before := snapshot(t, dir)
	var out, errb bytes.Buffer
	if code := run([]string{"-dir", dir, "v2.0.0"}, s.Client(), &out, &errb); code != 1 {
		t.Fatalf("code %d, stderr %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "v2.0.0") || !strings.Contains(out.String(), "added\textra.test") {
		t.Errorf("stdout %q", out.String())
	}
	if !reflect.DeepEqual(before, snapshot(t, dir)) {
		t.Error("directory changed")
	}
}

func TestRunRefNotFound(t *testing.T) {
	c := map[string]string{"a.test": "A"}
	s := serve(t, nil)
	dir := writeDir(t, s.URL, "v9.9.9", c)
	var out, errb bytes.Buffer
	if code := run([]string{"-dir", dir}, s.Client(), &out, &errb); code != 2 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(errb.String(), "v9.9.9") || !strings.Contains(errb.String(), "not found") || !recordLineOnly(out.String()) {
		t.Errorf("stderr %q stdout %q", errb.String(), out.String())
	}
}

func TestRunNoOrigin(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"-dir", t.TempDir()}, http.DefaultClient, &out, &errb); code != 2 || !strings.Contains(errb.String(), "ORIGIN.md") {
		t.Errorf("no file: code %d stderr %q", code, errb.String())
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "ORIGIN.md"), []byte("Repository: http://x\n"), 0o644)
	errb.Reset()
	if code := run([]string{"-dir", dir}, http.DefaultClient, &out, &errb); code != 2 || !strings.Contains(errb.String(), "Ref") {
		t.Errorf("no Ref: code %d stderr %q", code, errb.String())
	}
}

func TestRunEmptyUpstream(t *testing.T) {
	s := serve(t, map[string][]byte{"v1.0.0": corpusTar(t, "root", nil)})
	dir := writeDir(t, s.URL, "v1.0.0", map[string]string{"a.test": "A"})
	var out, errb bytes.Buffer
	if code := run([]string{"-dir", dir}, s.Client(), &out, &errb); code != 2 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(errb.String(), "no *.test files") {
		t.Errorf("stderr %q", errb.String())
	}
}

func TestRunUsage(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"-h"}, http.DefaultClient, &out, &errb); code != 0 || !strings.Contains(out.String(), "Usage") {
		t.Errorf("-h: code %d stdout %q", code, out.String())
	}
	out.Reset()
	errb.Reset()
	if code := run([]string{"a", "b"}, http.DefaultClient, &out, &errb); code != 2 || !strings.Contains(errb.String(), "Usage") {
		t.Errorf("two args: code %d stderr %q", code, errb.String())
	}
	if code := run([]string{"-bogus"}, http.DefaultClient, &out, &errb); code != 2 {
		t.Errorf("-bogus: code %d", code)
	}
}

// A gzip stream that is truncated before its trailer or whose CRC is wrong
// must be rejected as a bad archive, not compared as if it were complete.
func TestRunBadGzipTrailer(t *testing.T) {
	good := corpusTar(t, "root", map[string]string{"a.test": "A"})
	missing := good[:len(good)-8]
	badCRC := append([]byte(nil), good...)
	badCRC[len(badCRC)-8] ^= 1
	for name, body := range map[string][]byte{"missing trailer": missing, "bad crc": badCRC} {
		t.Run(name, func(t *testing.T) {
			s := serve(t, map[string][]byte{"v1.0.0": body})
			dir := writeDir(t, s.URL, "v1.0.0", map[string]string{"a.test": "A"})
			var out, errb bytes.Buffer
			if code := run([]string{"-dir", dir}, s.Client(), &out, &errb); code != 2 {
				t.Fatalf("code %d, stdout %q, stderr %q", code, out.String(), errb.String())
			}
			if !strings.Contains(errb.String(), "bad archive") || !recordLineOnly(out.String()) {
				t.Errorf("stderr %q stdout %q", errb.String(), out.String())
			}
		})
	}
}

// recordLineOnly reports whether stdout is just the offline part's match
// line: the pin check got as far as the online part and printed nothing more.
func recordLineOnly(stdout string) bool {
	return strings.Count(stdout, "\n") == 1 && strings.Contains(stdout, "matches SHA256SUMS")
}

func TestIsTag(t *testing.T) {
	for _, ref := range []string{"v2.0.0", "v10.0.1"} {
		if !isTag(ref) {
			t.Errorf("isTag(%q) = false", ref)
		}
	}
	for _, ref := range []string{"main", "ae01cd4", "2.0.0", "v2.0", "v2.0.0-rc1", ""} {
		if isTag(ref) {
			t.Errorf("isTag(%q) = true", ref)
		}
	}
}

func TestRecordRoundTrip(t *testing.T) {
	sums := digest(map[string][]byte{"b.test": []byte("B"), "a.test": []byte("A")})
	got := string(formatRecord("v1.2.3", sums))
	want := "# Ref: v1.2.3\n" +
		"559aead08264d5795d3909718cdd05abd49572e84fe55590eef31a88a08fdffd  a.test\n" +
		"df7e70e5021544f4834bbee64a9e3789febc4be81470df629cad6ddb03320a5c  b.test\n"
	if got != want {
		t.Fatalf("formatRecord:\n%s\nwant\n%s", got, want)
	}
	rec, err := parseRecord([]byte(got))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Ref != "v1.2.3" || !reflect.DeepEqual(rec.Sums, sums) {
		t.Fatalf("parseRecord: %+v", rec)
	}
	// Blank lines are ignored.
	rec, err = parseRecord([]byte(strings.ReplaceAll(got, "\n", "\n\n")))
	if err != nil || !reflect.DeepEqual(rec.Sums, sums) {
		t.Fatalf("with blank lines: %+v, %v", rec, err)
	}
}

func TestParseRecordErrors(t *testing.T) {
	sum := strings.Repeat("ab", 32)
	cases := map[string]string{
		"no header":         sum + "  a.test\n",
		"header not first":  sum + "  a.test\n# Ref: v1.0.0\n",
		"bad digest length": "# Ref: v1.0.0\n" + sum[:62] + "  a.test\n",
		"uppercase hex":     "# Ref: v1.0.0\n" + strings.ToUpper(sum) + "  a.test\n",
		"one space":         "# Ref: v1.0.0\n" + sum + " a.test\n",
		"duplicate name":    "# Ref: v1.0.0\n" + sum + "  a.test\n" + sum + "  a.test\n",
		"empty header ref":  "# Ref: \n" + sum + "  a.test\n",
		"header ref alone":  "# Ref: v1.0.0\n",
		"empty":             "",
	}
	for name, in := range cases {
		if _, err := parseRecord([]byte(in)); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestRunOfflineMatch(t *testing.T) {
	c := map[string]string{"a.test": "A", "b.test": "B"}
	dir := writeDir(t, "https://example.invalid/zjot", "v1.0.0", c)
	before := snapshot(t, dir)
	var out, errb bytes.Buffer
	if code := run([]string{"-dir", dir}, offlineClient(), &out, &errb); code != 0 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "matches SHA256SUMS at v1.0.0 (2 files)") {
		t.Errorf("stdout %q", out.String())
	}
	if !strings.Contains(errb.String(), "offline") {
		t.Errorf("stderr %q", errb.String())
	}
	if !reflect.DeepEqual(before, snapshot(t, dir)) {
		t.Error("directory changed")
	}
}

func TestRunOfflineDrift(t *testing.T) {
	recorded := map[string]string{"keep.test": "K", "gone.test": "G", "diff.test": "old"}
	local := map[string]string{"keep.test": "K", "new.test": "N", "diff.test": "new"}
	dir := writeDirNoRecord(t, "https://example.invalid/zjot", "v1.0.0", local)
	writeRecord(t, dir, "v1.0.0", recorded)
	before := snapshot(t, dir)
	var out, errb bytes.Buffer
	if code := run([]string{"-dir", dir}, offlineClient(), &out, &errb); code != 1 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, out.String(), errb.String())
	}
	want := []string{"changed\tdiff.test", "added\tgone.test", "removed\tnew.test"}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 4 || !reflect.DeepEqual(lines[:3], want) || !strings.HasPrefix(lines[3], "driftcheck: 3 files differ from SHA256SUMS at v1.0.0") {
		t.Fatalf("stdout %q", out.String())
	}
	if !reflect.DeepEqual(before, snapshot(t, dir)) {
		t.Error("directory changed")
	}
}

func TestRunBumpedTagOffline(t *testing.T) {
	c := map[string]string{"a.test": "A"}
	dir := writeDirNoRecord(t, "https://example.invalid/zjot", "v1.1.0", c)
	writeRecord(t, dir, "v1.0.0", c)
	var out, errb bytes.Buffer
	if code := run([]string{"-dir", dir}, offlineClient(), &out, &errb); code != 1 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "records v1.0.0 but ORIGIN.md pins v1.1.0") {
		t.Errorf("stdout %q", out.String())
	}
}

func TestRunBumpedTagOnline(t *testing.T) {
	c := map[string]string{"a.test": "A"}
	s := serve(t, map[string][]byte{"v1.1.0": corpusTar(t, "zjot-1.1.0", c)})
	dir := writeDirNoRecord(t, s.URL, "v1.1.0", c)
	writeRecord(t, dir, "v1.0.0", c)
	var out, errb bytes.Buffer
	if code := run([]string{"-dir", dir}, s.Client(), &out, &errb); code != 1 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, out.String(), errb.String())
	}
	for _, w := range []string{"records v1.0.0 but ORIGIN.md pins v1.1.0", "matches zjot at v1.1.0"} {
		if !strings.Contains(out.String(), w) {
			t.Errorf("stdout %q lacks %q", out.String(), w)
		}
	}
}

func TestRunBranchPin(t *testing.T) {
	c := map[string]string{"a.test": "A"}
	s, hits := serveCounting(t, map[string][]byte{"main": corpusTar(t, "zjot-main", c)})
	dir := writeDir(t, s.URL, "main", c)
	var out, errb bytes.Buffer
	if code := run([]string{"-dir", dir}, s.Client(), &out, &errb); code != 2 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, out.String(), errb.String())
	}
	for _, w := range []string{"main", "not a tag", "vN.N.N"} {
		if !strings.Contains(errb.String(), w) {
			t.Errorf("stderr %q lacks %q", errb.String(), w)
		}
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("%d requests reached the server", n)
	}
}

func TestRunOtherRefOffline(t *testing.T) {
	c := map[string]string{"a.test": "A"}
	dir := writeDir(t, "https://example.invalid/zjot", "v1.0.0", c)
	var out, errb bytes.Buffer
	if code := run([]string{"-dir", dir, "v2.0.0"}, offlineClient(), &out, &errb); code != 2 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, out.String(), errb.String())
	}
	if !strings.Contains(errb.String(), "cannot measure") {
		t.Errorf("stderr %q", errb.String())
	}
}

func TestRunOtherRefNotATag(t *testing.T) {
	c := map[string]string{"a.test": "A"}
	s := serve(t, map[string][]byte{"main": corpusTar(t, "zjot-main", c)})
	dir := writeDir(t, s.URL, "v1.0.0", c)
	before := snapshot(t, dir)
	var out, errb bytes.Buffer
	if code := run([]string{"-dir", dir, "main"}, s.Client(), &out, &errb); code != 0 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "matches zjot at main") || strings.Contains(errb.String(), "not a tag") {
		t.Errorf("stdout %q stderr %q", out.String(), errb.String())
	}
	if !reflect.DeepEqual(before, snapshot(t, dir)) {
		t.Error("directory changed")
	}
}

func TestRunNoRecord(t *testing.T) {
	c := map[string]string{"a.test": "A"}
	s := serve(t, map[string][]byte{"v1.0.0": corpusTar(t, "zjot-1.0.0", c)})
	dir := writeDirNoRecord(t, s.URL, "v1.0.0", c)
	var out, errb bytes.Buffer
	if code := run([]string{"-dir", dir}, s.Client(), &out, &errb); code != 2 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, out.String(), errb.String())
	}
	for _, w := range []string{"SHA256SUMS", "-record"} {
		if !strings.Contains(errb.String(), w) {
			t.Errorf("stderr %q lacks %q", errb.String(), w)
		}
	}
}

func TestRunRecord(t *testing.T) {
	c := map[string]string{"a.test": "A", "b.test": "B"}
	s := serve(t, map[string][]byte{"v1.0.0": corpusTar(t, "zjot-1.0.0", c)})
	dir := writeDirNoRecord(t, s.URL, "v1.0.0", c)
	var out, errb bytes.Buffer
	if code := run([]string{"-dir", dir, "-record"}, s.Client(), &out, &errb); code != 0 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "recorded 2 files at v1.0.0") {
		t.Errorf("stdout %q", out.String())
	}
	got, err := os.ReadFile(filepath.Join(dir, recordName))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{"a.test": []byte("A"), "b.test": []byte("B")}
	if want := formatRecord("v1.0.0", digest(files)); !bytes.Equal(got, want) {
		t.Errorf("record %q, want %q", got, want)
	}
	out.Reset()
	errb.Reset()
	if code := run([]string{"-dir", dir}, offlineClient(), &out, &errb); code != 0 {
		t.Fatalf("offline after record: code %d, stdout %q, stderr %q", code, out.String(), errb.String())
	}
}

func TestRunRecordRefuses(t *testing.T) {
	c := map[string]string{"a.test": "A"}
	up := map[string]string{"a.test": "changed"}
	s := serve(t, map[string][]byte{"v1.0.0": corpusTar(t, "zjot-1.0.0", up)})
	for _, tc := range []struct {
		name   string
		args   []string
		client *http.Client
		code   int
		stream string
		want   string
	}{
		{"drift", []string{"-record"}, s.Client(), 1, "stdout", "not recorded"},
		{"offline", []string{"-record"}, offlineClient(), 2, "stderr", "offline"},
		{"with REF", []string{"-record", "v2.0.0"}, s.Client(), 2, "stderr", "measurement"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeDirNoRecord(t, s.URL, "v1.0.0", c)
			before := snapshot(t, dir)
			var out, errb bytes.Buffer
			if code := run(append([]string{"-dir", dir}, tc.args...), tc.client, &out, &errb); code != tc.code {
				t.Fatalf("code %d, stdout %q, stderr %q", code, out.String(), errb.String())
			}
			got := out.String()
			if tc.stream == "stderr" {
				got = errb.String()
			}
			if !strings.Contains(got, tc.want) {
				t.Errorf("%s %q lacks %q", tc.stream, got, tc.want)
			}
			if !reflect.DeepEqual(before, snapshot(t, dir)) {
				t.Error("directory changed")
			}
		})
	}
}

// TestRecordRealCorpus is the offline pin check over the checked-in corpus.
func TestRecordRealCorpus(t *testing.T) {
	const dir = "../../testdata/spec"
	ob, err := os.ReadFile(filepath.Join(dir, "ORIGIN.md"))
	if err != nil {
		t.Fatal(err)
	}
	o, err := parseOrigin(ob)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := readRecord(dir)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Ref != "v2.0.0" || rec.Ref != o.Ref {
		t.Fatalf("record Ref %q, ORIGIN.md Ref %q", rec.Ref, o.Ref)
	}
	local, err := readLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Sums) != 28 {
		t.Errorf("record lists %d files, want 28", len(rec.Sums))
	}
	if got := digest(local); !reflect.DeepEqual(got, rec.Sums) {
		t.Errorf("corpus differs from SHA256SUMS: %v", compareSums(got, rec.Sums))
	}
}
