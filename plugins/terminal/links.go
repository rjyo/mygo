package terminal

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// urlPattern finds the URLs programs print as text, which a Command+click
// opens as it does hyperlinks.
var urlPattern = regexp.MustCompile(`(?:https?|ftp|file)://[^\s"'<>` + "`" + `\x00-\x1f]+|mailto:[^\s"'<>` + "`" + `]+`)

// pathPattern finds the file paths programs print as text: absolute, from
// ~, ./ or ../, with a slash, or a file name with an extension; each may end in
// :line or :line:col.
var pathPattern = regexp.MustCompile(`(?:(?:~|\.{1,2})?/[^\s"'<>` + "`" + `\x00-\x1f:]+|[\w.@+-]+(?:/[\w.@+-]+)+/?)(?::\d+(?::\d+)?)?|[\w@+-][\w.@+-]*\.\w+(?::\d+(?::\d+)?)?`)

// urlAt returns the URL of text, a row whose runes are in columns cols, at
// column col, and the columns it spans, x0 up to x1, or "".
func urlAt(text []rune, cols []int, col int) (url string, x0, x1 int) {
	return matchAt(urlPattern, text, cols, col)
}

// pathAt returns the file path of text at column col, as urlAt, or "".
func pathAt(text []rune, cols []int, col int) (path string, x0, x1 int) {
	p, x0, x1 := matchAt(pathPattern, text, cols, col)
	// Do not turn dotted numbers such as version 1.2.3 into file links.
	if !strings.ContainsAny(p, "/:") && strings.IndexFunc(p, unicode.IsLetter) < 0 {
		return "", 0, 0
	}
	return p, x0, x1
}

func matchAt(re *regexp.Regexp, text []rune, cols []int, col int) (string, int, int) {
	s := string(text)
	for _, m := range re.FindAllStringIndex(s, -1) {
		u := trimURL(s[m[0]:m[1]])
		if u == "" {
			continue
		}
		start := len([]rune(s[:m[0]]))
		end := start + len([]rune(u))
		if start < len(cols) && cols[start] <= col && col <= cols[end-1] {
			return u, cols[start], cols[end-1] + 1
		}
	}
	return "", 0, 0
}

// trimURL trims what likely ends the sentence around a URL rather than the
// URL: punctuation, and closing brackets that the URL did not open.
func trimURL(u string) string {
	for len(u) > 0 {
		switch last := u[len(u)-1]; last {
		case '.', ',', ';', ':', '!', '?':
			u = u[:len(u)-1]
			continue
		case ')', ']', '}':
			open := map[byte]string{')': "(", ']': "[", '}': "{"}[last]
			if strings.Count(u, open) < strings.Count(u, string(last)) {
				u = u[:len(u)-1]
				continue
			}
		}
		return u
	}
	return u
}

// maxWrappedToken is the most columns a row is taken to have stopped short
// of its edge by because the first word of the row below did not fit.
const maxWrappedToken = 16

// linkRow is a row of the terminal as links read it: the text and columns
// of its cells, and whether it soft-wraps into the next.
type linkRow struct {
	text []rune
	cols []int
	wrap bool
}

// at is the rune of column x, a blank in a cell without one.
func (r linkRow) at(x int) rune {
	if i, ok := slices.BinarySearch(r.cols, x); ok {
		return r.text[i]
	}
	return ' '
}

// linkBlock is the columns x0 up to x1 of the rows that a link under the
// pointer can run through: those between the frames either side of it, so
// that a link in a multiplexer's pane stays in that pane.
//
// A multiplexer redraws its panes cell by cell, which leaves no soft wrap
// for a URL it shows folded over several rows. So rows also join by their
// shape, as they do in Ghostty's wrapped-link matching: see joins.
type linkBlock struct {
	rows   []linkRow
	width  int // the columns of a row
	x0, x1 int
	right  int // past the rightmost cell any row writes in the block
}

// newLinkBlock returns the block around the cell, false on a frame.
func newLinkBlock(rows []linkRow, width, col, row int) (linkBlock, bool) {
	if row < 0 || row >= len(rows) || col < 0 || col >= width || isFrame(rows[row].at(col)) {
		return linkBlock{}, false
	}
	b := linkBlock{rows: rows, width: width, x0: col, x1: col + 1}
	for b.x0 > 0 && !isFrame(rows[row].at(b.x0-1)) {
		b.x0--
	}
	for b.x1 < width && !isFrame(rows[row].at(b.x1)) {
		b.x1++
	}
	for y := range rows {
		b.right = max(b.right, b.end(y))
	}
	return b, true
}

// isFrame reports whether r draws a pane's border or scroll bar: box
// drawing and block elements, which no link holds. Reading the pointer's
// row alone is enough, and unlike a column of the whole screen it holds
// across a border's corners, a tab bar above it, and a split below it.
func isFrame(r rune) bool { return r >= 0x2500 && r <= 0x259f }

func isBlank(r rune) bool { return r == ' ' || r == 0 }

// end is the column past the last cell row y writes in the block, x0 for
// none.
func (b linkBlock) end(y int) int {
	r := b.rows[y]
	for i := len(r.cols) - 1; i >= 0; i-- {
		if c := r.cols[i]; c >= b.x0 && c < b.x1 && !isBlank(r.text[i]) {
			return c + 1
		}
	}
	return b.x0
}

// token returns the first word of row y in the block and how far it is
// indented.
func (b linkBlock) token(y int) (word []rune, indent int) {
	r := b.rows[y]
	x := b.x0
	for x < b.x1 && isBlank(r.at(x)) {
		x++
	}
	indent = x - b.x0
	for ; x < b.x1 && !isBlank(r.at(x)); x++ {
		word = append(word, r.at(x))
	}
	return word, indent
}

// wrapped reports whether the terminal soft-wrapped row y here: a wrap
// flag says so for the whole width, and only if the row's last cell is
// written, as a space there ends a link.
func (b linkBlock) wrapped(y int) bool {
	return b.rows[y].wrap && b.x1 == b.width && b.end(y) == b.width
}

// joins reports whether row y goes on in the row below though the terminal
// did not wrap it: it fills the block but for the gutter a multiplexer
// keeps, or, when loose, it stops short by no more than the word below,
// which no plain word could be. loose finds what a program itself folded
// inside its own margins, and suits URLs only: a list of paths has rows of
// that shape that do not join.
func (b linkBlock) joins(y int, loose bool) bool {
	if b.rows[y].wrap && b.x1 == b.width || y+1 >= len(b.rows) {
		return false
	}
	end := b.end(y)
	word, indent := b.token(y + 1)
	if end == b.x0 || len(word) == 0 || startsLink(word) {
		return false
	}
	if indent == 0 && b.x1-end <= 1 {
		return true
	}
	return loose && slices.ContainsFunc(word, isLinkFragment) && b.right-end <= min(len(word), maxWrappedToken)
}

// startsLink reports whether a word starts a link of its own, as the next
// of a list of URLs does, however close to the edge the one before it ends.
func startsLink(word []rune) bool {
	s := strings.ToLower(string(word))
	for _, p := range []string{"http://", "https://", "ftp://", "file://", "mailto:"} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// isLinkFragment reports whether r marks a word as a piece of a link: a
// digit, a separator of URLs and paths, or a bracket closing a citation.
func isLinkFragment(r rune) bool {
	return r >= '0' && r <= '9' || strings.ContainsRune("/.-_=&?%:#~+@)]", r)
}

// line returns the text of the rows that join into the one of row, and
// the position of each rune, its row times the width plus its column: a
// position goes up along the text as a column does along a row.
func (b linkBlock) line(row int, loose bool) (text []rune, pos []int) {
	goesOn := func(y int) bool { return b.wrapped(y) || b.joins(y, loose) }
	first, last := row, row
	for first > 0 && goesOn(first-1) {
		first--
	}
	for last+1 < len(b.rows) && goesOn(last) {
		last++
	}
	for y := first; y <= last; y++ {
		start := b.x0
		if y > first && !b.wrapped(y-1) {
			_, indent := b.token(y)
			start += indent // only a row joined by its shape is indented
		}
		end, r := b.end(y), b.rows[y]
		for i, c := range r.cols {
			if c >= start && c < end {
				text, pos = append(text, r.text[i]), append(pos, y*b.width+c)
			}
		}
	}
	return text, pos
}

// find returns what match finds at the cell in the line of its row, and
// the cells of it on each row, or "".
func (b linkBlock) find(match func(text []rune, cols []int, col int) (string, int, int), loose bool, col, row int) (string, []linkSpan) {
	text, pos := b.line(row, loose)
	link, from, to := match(text, pos, row*b.width+col)
	if link == "" {
		return "", nil
	}
	var spans []linkSpan
	for _, p := range pos {
		if p < from || p >= to {
			continue
		}
		y, x := p/b.width, p%b.width
		if n := len(spans) - 1; n >= 0 && spans[n].row == y {
			spans[n].x1 = x + 1
		} else {
			spans = append(spans, linkSpan{y, x, x + 1})
		}
	}
	// The indent of a joined row, or the gutter before it, is inside the
	// positions of the link but is not on it.
	if !slices.ContainsFunc(spans, func(s linkSpan) bool { return s.row == row && col >= s.x0 && col < s.x1 }) {
		return "", nil
	}
	return link, spans
}
