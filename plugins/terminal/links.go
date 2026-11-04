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

// has reports whether column x has a rune of its own, as all but the
// spacers around a wide character do, a blank cell's being a space.
func (r linkRow) has(x int) bool {
	_, ok := slices.BinarySearch(r.cols, x)
	return ok
}

// past is the column past the character in column x: two on if it is wide.
// The spacer a wide character leaves at the end of a row, as it wraps, reads
// as the tail of the one before it, which only covers that empty cell too.
func (r linkRow) past(x, width int) int {
	if x+1 < width && !r.has(x+1) {
		return x + 2
	}
	return x + 1
}

// linkBlock is the columns x0 up to x1 of the rows that a link under the
// pointer can run through: those between the frames either side of it, so
// that a link in a multiplexer's pane stays in that pane.
//
// A multiplexer redraws its panes cell by cell, which leaves no soft wrap
// for a URL it shows folded over several rows. So rows also join by their
// shape, as they do in Ghostty's wrapped-link matching: see joins.
// Ghostty bounds a pane by whole columns of frames instead, which a
// border's corners and a tab bar above it break.
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
			return min(r.past(c, b.width), b.x1)
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

// wrapped reports whether the terminal soft-wrapped row y here. Its flag
// holds for the whole width, and a space in the last cell ends a link, where
// a wide character that did not fit leaves a spacer.
func (b linkBlock) wrapped(y int) bool {
	r, last := b.rows[y], b.width-1
	return r.wrap && b.x1 == b.width && (!r.has(last) || !isBlank(r.at(last)))
}

// joins reports whether row y goes on in the row below though the terminal
// did not wrap it, which only a URL does: a list of paths has rows of the
// same shape that do not join. The rows must be of one pane. The row then
// stops short of the right by no more than the word below, which holds a
// mark no plain word does: that finds what a program folded inside its own
// margins. Or that word is alone on its row, and this row fills the block
// but for the gutter a multiplexer keeps: a link cut there goes on in any
// letters, at the risk of taking in a lone word below a URL that ended in
// the very last column.
func (b linkBlock) joins(y int) bool {
	if b.rows[y].wrap && b.x1 == b.width || y+1 >= len(b.rows) || !b.samePane(y) {
		return false
	}
	end := b.end(y)
	word, indent := b.token(y + 1)
	if end == b.x0 || len(word) == 0 || startsLink(word) || slices.ContainsFunc(word, isFrame) {
		return false
	}
	if indent == 0 && b.x1-end <= 1 && b.end(y+1) <= b.x0+len(word) {
		return true
	}
	return slices.ContainsFunc(word, isLinkFragment) && b.right-end <= min(len(word), maxWrappedToken)
}

// samePane reports whether rows y and y+1 have the same frames either side
// of the block: a pane below a split has its own.
func (b linkBlock) samePane(y int) bool {
	r, next := b.rows[y], b.rows[y+1]
	return (b.x0 == 0 || isFrame(r.at(b.x0-1)) == isFrame(next.at(b.x0-1))) &&
		(b.x1 == b.width || isFrame(r.at(b.x1)) == isFrame(next.at(b.x1)))
}

// runsOn reports whether a hyperlink that stops before column end of row y
// can go on at the start of the next row: the terminal wrapped it there, or
// it reaches the frame of a pane or a blank gutter before one. Without a
// frame or a gutter, only the wrap flag tells.
func (b linkBlock) runsOn(y, end int) bool {
	if y+1 >= len(b.rows) {
		return false
	}
	r, last := b.rows[y], b.width-1
	if r.wrap && b.x1 == b.width {
		// Unlike a URL's text, a hyperlink's can hold a space there.
		return end == b.width || end == last && !r.has(last)
	}
	return b.samePane(y) && (end == b.x1 && b.x1 < b.width || end == b.x1-1 && r.has(end) && isBlank(r.at(end)))
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
// position goes up along the text as a column does along a row. Only a URL
// follows the rows' shape (joins).
func (b linkBlock) line(row int, shape bool) (text []rune, pos []int) {
	goesOn := func(y int) bool { return b.wrapped(y) || shape && b.joins(y) }
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
func (b linkBlock) find(match func(text []rune, cols []int, col int) (string, int, int), shape bool, col, row int) (string, []linkSpan) {
	if col > 0 && !b.rows[row].has(col) {
		col-- // the tail of a wide character is the character
	}
	text, pos := b.line(row, shape)
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
			spans[n].x1 = b.rows[y].past(x, b.width)
		} else {
			spans = append(spans, linkSpan{y, x, b.rows[y].past(x, b.width)})
		}
	}
	// The indent of a joined row, or the gutter before it, is inside the
	// positions of the link but is not on it.
	if !slices.ContainsFunc(spans, func(s linkSpan) bool { return s.row == row && col >= s.x0 && col < s.x1 }) {
		return "", nil
	}
	return link, spans
}
