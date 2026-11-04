package terminal

import (
	"slices"
	"strings"
	"testing"
)

func TestURLAt(t *testing.T) {
	row := "see https://example.com/a_(b)?q=1. and (http://x.org/y), mailto:me@x.org!"
	text := []rune(row)
	cols := make([]int, len(text))
	for i := range cols {
		cols[i] = i
	}
	for col, want := range map[int]string{
		0:  "",
		4:  "https://example.com/a_(b)?q=1",
		20: "https://example.com/a_(b)?q=1",
		33: "", // the period after it
		40: "http://x.org/y",
		58: "mailto:me@x.org",
	} {
		if got, _, _ := urlAt(text, cols, col); got != want {
			t.Errorf("urlAt(%d) = %q, want %q", col, got, want)
		}
	}
}

func TestPathAt(t *testing.T) {
	for _, c := range []struct {
		row  string
		col  int
		want string
	}{
		{"Updated internal/app/embed.go:51 and more", 10, "internal/app/embed.go:51"},
		{"  ⎿  Read /Users/me/repo/main.go (12 lines)", 12, "/Users/me/repo/main.go"},
		{"see ./notes/todo.txt.", 8, "./notes/todo.txt"},
		{"open ~/Downloads/clip.mp4", 10, "~/Downloads/clip.mp4"},
		{"main.go:250:3: undefined: x", 2, "main.go:250:3"},
		{"(src/a.go)", 3, "src/a.go"},
		{"write(notes.md)", 8, "notes.md"},
		{"notes.md (new file +2)", 2, "notes.md"},
		{"See notes.md.", 7, "notes.md"},
		{"notes.md:2:3", 2, "notes.md:2:3"},
		{"just words here", 5, ""},
		{"version 1.2.3 ok", 9, ""},
	} {
		text := []rune(c.row)
		cols := make([]int, len(text))
		for i := range cols {
			cols[i] = i
		}
		if got, _, _ := pathAt(text, cols, c.col); got != c.want {
			t.Errorf("pathAt(%q, %d) = %q, want %q", c.row, c.col, got, c.want)
		}
	}
}

func TestPathAtSpan(t *testing.T) {
	text := []rune("write(notes.md)")
	cols := make([]int, len(text))
	for i := range cols {
		cols[i] = i
	}
	if path, x0, x1 := pathAt(text, cols, 8); path != "notes.md" || x0 != 6 || x1 != 14 {
		t.Errorf("pathAt = %q %d..%d, want notes.md 6..14", path, x0, x1)
	}
}

func TestURLAtSpan(t *testing.T) {
	text := []rune("go to https://x.org/a. now")
	cols := make([]int, len(text))
	for i := range cols {
		cols[i] = i
	}
	if url, x0, x1 := urlAt(text, cols, 10); url != "https://x.org/a" || x0 != 6 || x1 != 21 {
		t.Errorf("urlAt = %q %d..%d, want https://x.org/a 6..21", url, x0, x1)
	}
}

// screen makes rows of text, a row ending in \ soft-wrapped.
func screen(lines ...string) ([]linkRow, int) {
	rows, width := make([]linkRow, len(lines)), 0
	for y, l := range lines {
		l, wrap := strings.CutSuffix(l, `\`)
		rows[y].text, rows[y].wrap = []rune(l), wrap
		rows[y].cols = make([]int, len(rows[y].text))
		for i := range rows[y].cols {
			rows[y].cols[i] = i
		}
		width = max(width, len(rows[y].text))
	}
	return rows, width
}

func urlIn(t *testing.T, rows []linkRow, width, col, row int) (string, []linkSpan) {
	t.Helper()
	b, ok := newLinkBlock(rows, width, col, row)
	if !ok {
		return "", nil
	}
	return b.find(urlAt, true, col, row)
}

func TestWrappedURLs(t *testing.T) {
	const want = "https://x.org/pull/12/files/abcdef/ghijkl?case=9"
	for _, c := range []struct {
		name     string
		lines    []string
		col, row int
		want     string
		spans    []linkSpan
	}{
		{
			name:  "soft-wrapped, from its first row",
			lines: []string{`https://x.org/pull/12/\`, `files/abcdef/ghijkl?ca\`, `se=9`},
			want:  want,
			spans: []linkSpan{{0, 0, 22}, {1, 0, 22}, {2, 0, 4}},
		},
		{
			name:  "soft-wrapped, from its last row",
			lines: []string{`https://x.org/pull/12/\`, `files/abcdef/ghijkl?ca\`, `se=9`},
			col:   1, row: 2,
			want:  want,
			spans: []linkSpan{{0, 0, 22}, {1, 0, 22}, {2, 0, 4}},
		},
		{
			// Rows as herdr draws them, with no wrap flag: a blank gutter at
			// the edge, the last row starting with a plain word.
			name: "redrawn by a multiplexer, ending in a plain word",
			lines: []string{
				"https://x.org/pull/12/ ",
				"files/abcdef/ghijkl?ca ",
				"se                     ",
			},
			col: 1, row: 1,
			want:  "https://x.org/pull/12/files/abcdef/ghijkl?case",
			spans: []linkSpan{{0, 0, 22}, {1, 0, 22}, {2, 0, 2}},
		},
		{
			// From the capture of a split in herdr 0.9.5: its corners in the
			// border's columns, and the sidebar and the next pane in the rows.
			name: "herdr panes",
			lines: []string{
				" spaces    │   1      +                     ",
				"           │┌────────────────┐┌────────────┐",
				" · moshi   ││https://x.org/p ││me@host %   │",
				"           ││ull/12/files/ab ││            │",
				"           ││cdef/ghijkl?cas ││            │",
				"           ││e=9             ││            │",
				" new   menu││aa              ││            │",
				"───────────││actool          ││            │",
				"          «│└────────────────┘└────────────┘",
			},
			col: 15, row: 3,
			want:  want,
			spans: []linkSpan{{2, 13, 28}, {3, 13, 28}, {4, 13, 28}, {5, 13, 16}},
		},
		{
			name: "a right-edge scroll bar",
			lines: []string{
				"https://x.org/pull/12/▐",
				"files/abcdef/ghijkl?ca▐",
				"se=9                  ▕",
			},
			want:  want,
			spans: []linkSpan{{0, 0, 22}, {1, 0, 22}, {2, 0, 4}},
		},
		{
			name: "a program's own wrap, indented, short of the edge",
			lines: []string{
				`  see https://x.org/pull/12/          `,
				`  files/abcdef/ghijkl?case=9, for it. `,
				`  and this row is the widest of them all`,
			},
			col: 12, row: 0,
			want:  want,
			spans: []linkSpan{{0, 6, 28}, {1, 2, 28}},
		},
		{
			name:  "a quoted string",
			lines: []string{`"self": "https://x.org/pull/12/ `, `files/abcdef/ghijkl?case=9",`},
			col:   12,
			want:  want,
			spans: []linkSpan{{0, 9, 31}, {1, 0, 26}},
		},
		{
			name:  "a URL ending its line is not glued to the word below (#549)",
			lines: []string{"https://x.org/a", "hello there you", "a much wider row than the two above it"},
			want:  "https://x.org/a",
			spans: []linkSpan{{0, 0, 15}},
		},
		{
			name:  "the widest row is not at the edge",
			lines: []string{"https://x.org/a      ", "hello"},
			want:  "https://x.org/a",
			spans: []linkSpan{{0, 0, 15}},
		},
		{
			name:  "adjacent URLs stay apart (#593)",
			lines: []string{"https://x.org/first", "https://x.org/other", "https://x.org/third"},
			row:   1,
			want:  "https://x.org/other",
			spans: []linkSpan{{1, 0, 19}},
		},
		{
			name:  "a space ending a soft-wrapped row ends the link",
			lines: []string{`https://x.org/a \`, "files/b"},
			want:  "https://x.org/a",
			spans: []linkSpan{{0, 0, 15}},
		},
		{
			name:  "an indented soft-wrapped row starts apart",
			lines: []string{`https://x.org/ab\`, "  files/b"},
			want:  "https://x.org/ab",
			spans: []linkSpan{{0, 0, 16}},
		},
		{
			name:  "the indent of a joined row is not on the link",
			lines: []string{"https://x.org/pull/12/ ", "   files/abc          "},
			col:   1, row: 1,
		},
		{
			name:  "a frame is not on a link",
			lines: []string{"│https://x.org/a │", "│                │"},
			col:   0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			rows, width := screen(c.lines...)
			got, spans := urlIn(t, rows, width, c.col, c.row)
			if got != c.want || !slices.Equal(spans, c.spans) {
				t.Errorf("got %q %v, want %q %v", got, spans, c.want, c.spans)
			}
		})
	}
}

// Rows of a list of paths have the shape of a program's own wrap, so a path
// joins only across the edge.
func TestWrappedPaths(t *testing.T) {
	rows, _ := screen("internal/logic/a.go ", "internal/logic/bb.go", "note")
	b, _ := newLinkBlock(rows, 80, 2, 0)
	if p, spans := b.find(pathAt, false, 2, 0); p != "internal/logic/a.go" || !slices.Equal(spans, []linkSpan{{0, 0, 19}}) {
		t.Errorf("a list of paths read %q %v", p, spans)
	}
	rows, width := screen(`src/plugins/term\`, "inal/view.go:12")
	b, _ = newLinkBlock(rows, width, 2, 1)
	if p, spans := b.find(pathAt, false, 2, 1); p != "src/plugins/terminal/view.go:12" || !slices.Equal(spans, []linkSpan{{0, 0, 16}, {1, 0, 15}}) {
		t.Errorf("a soft-wrapped path read %q %v", p, spans)
	}
}
