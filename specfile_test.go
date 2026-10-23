package gjot

import "testing"

func TestParseSpecFile(t *testing.T) {
	fixture := "prose before\r\n" +
		"```\r\n" +
		"para\r\n" +
		".\r\n" +
		"<p>para</p>\r\n" +
		"```\r\n" +
		"\n" +
		"```` ap\n" +
		"```\n" +
		"inner fence\n" +
		"```\n" +
		".\n" +
		"doc\n" +
		"  code_block text=\"inner fence\\n\"\n" +
		"```\n" + // three ticks do not close a four-tick fence
		"````\n" +
		"\n" +
		"```\n" +
		"*x*\n" +
		"!\n" +
		"return {}\n" +
		".\n" +
		"<p><strong>X</strong></p>\n" +
		"```\n"

	got := parseSpecFile(fixture)
	want := []specCase{
		{line: 2, input: "para\n", expected: "<p>para</p>\n"},
		{line: 8, input: "```\ninner fence\n```\n", expected: "doc\n  code_block text=\"inner fence\\n\"\n```\n", ast: true, sourcepos: true},
		{line: 18, input: "*x*\n", expected: "return {}\n.\n<p><strong>X</strong></p>\n", filter: true},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d cases, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("case %d:\n got  %+v\n want %+v", i, got[i], want[i])
		}
	}
}
