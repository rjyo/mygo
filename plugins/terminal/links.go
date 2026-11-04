package terminal

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// urlPattern finds the URLs programs print as text, which a Command+click
// opens as it does hyperlinks.
var urlPattern = regexp.MustCompile(`(?:https?|ftp|file)://[^\s"'<>` + "`" + `\x00-\x1f]+|mailto:[^\s"'<>` + "`" + `]+`)

// proseDelimiters end a path printed in Chinese or Japanese prose, which
// puts no space around it; a quoted path may hold them.
const proseDelimiters = "，。；：！？、（）【】《》「」『』"

// pathToken finds what may be a file path programs print as text, unquoted:
// a run of anything but spaces, quotes and proseDelimiters.
var pathToken = regexp.MustCompile(`[^\s"'<>` + "`" + proseDelimiters + `]+`)

// pathInToken finds a path inside a token that is not one as a whole, as
// main.go:12 in grep's main.go:12:code, build/app.js in --out=build/app.js
// or /usr/bin in PATH=/usr/bin:/bin.
var pathInToken = regexp.MustCompile(`(?:(?:~|\.{1,2})?/[^\s:]+|[\p{L}\p{N}_.@+-]+(?:/[\p{L}\p{N}_.@+-]+)+/?)(?::\d+(?::\d+)?)?|[\p{L}\p{N}_@+-][\p{L}\p{N}_.@+-]*\.[\p{L}\p{N}_]+(?::\d+(?::\d+)?)?`)

// quotedPlace finds the place after a quoted path, as "main.py":12.
var quotedPlace = regexp.MustCompile(`^:\d+(?::\d+)?`)

// pathPlace finds the place in the file a path ends with: :line or
// :line:col (compilers, grep), (line,col) (TypeScript, MSBuild), or #Lline
// or #LlineCcol (GitHub).
var pathPlace = regexp.MustCompile(`:(\d+)(?::(\d+))?:?$|\((\d+)(?:,(\d+))?\)$|#L(\d+)(?:C(\d+))?$`)

// pathAt returns the file path of text at column col, as urlAt, or "":
// absolute, from ~, ./ or ../, with a slash, or a file name with an
// extension or a line; the place in the file is written path:line:col
// however it was printed, and a quoted path loses its quotes.
func pathAt(text []rune, cols []int, col int) (path string, x0, x1 int) {
	s := string(text)
	span := func(a, b int) (int, int, bool) {
		start := utf8.RuneCountInString(s[:a])
		end := start + utf8.RuneCountInString(s[a:b])
		if end > start && end <= len(cols) && cols[start] <= col && col <= cols[end-1] {
			return cols[start], cols[end-1] + 1, true
		}
		return 0, 0, false
	}
	// A quoted path first, as it may hold spaces and what the tokens below
	// end at.
	for i := 0; i < len(s); i++ {
		a, b := quotedAt(s, i)
		if b < 0 {
			continue
		}
		p, pa, pb := pathOf(s[a:b])
		if p == "" {
			continue
		}
		if x0, x1, ok := span(a+pa, a+pb); ok {
			return p, x0, x1
		}
		i = b - 1
	}
	for _, m := range pathToken.FindAllStringIndex(s, -1) {
		tok := s[m[0]:m[1]]
		if p, a, b := pathOf(tok); p != "" {
			if x0, x1, ok := span(m[0]+a, m[0]+b); ok {
				return p, x0, x1
			}
			continue
		}
		for _, n := range pathInToken.FindAllStringIndex(tok, -1) {
			if p, a, b := pathOf(tok[n[0]:n[1]]); p != "" {
				if x0, x1, ok := span(m[0]+n[0]+a, m[0]+n[0]+b); ok {
					return p, x0, x1
				}
			}
		}
	}
	return "", 0, 0
}

// quotedAt returns the bytes of s, a up to b, of a quoted run that opens at
// byte i and the place after it, or b < 0. A quote between letters or digits,
// as in don't, is an apostrophe and opens or closes nothing, and the inside
// of a quoted path neither starts nor ends with a space.
func quotedAt(s string, i int) (a, b int) {
	q := s[i]
	if q != '"' && q != '\'' || i > 0 && isAlnum(s[i-1]) {
		return 0, -1
	}
	j := strings.IndexByte(s[i+1:], q)
	if j < 0 {
		return 0, -1
	}
	j += i + 1
	inner := s[i+1 : j]
	if inner == "" || strings.ContainsAny(inner, "\r\n") || strings.TrimSpace(inner) != inner {
		return 0, -1
	}
	b = j + 1
	b += len(quotedPlace.FindString(s[b:]))
	if b < len(s) && isAlnum(s[b]) {
		return 0, -1
	}
	return i, b
}

// isAlnum reports whether c is an ASCII letter or digit.
func isAlnum(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9'
}

// pathOf returns the path a token of pathToken, or a quoted run, prints,
// and the bytes of the token it shows in, a up to b, or "".
func pathOf(tok string) (path string, a, b int) {
	a, b = 0, len(tok)
	quoted := tok[0] == '"' || tok[0] == '\''
	if !quoted {
		// Brackets and emphasis around a path are not part of it, nor is
		// the call a path is the argument of, as write(notes.md); a bracket
		// the path opens is, as icon_(copy).png.
		a = len(tok) - len(strings.TrimLeft(tok, "([{*"))
		// The name of an option or a variable the path is the value of,
		// as --out=build/app.js or PATH=/usr/bin, is not part of it.
		if i := strings.IndexByte(tok[a:], '='); i >= 0 && !strings.Contains(tok[a:a+i], "/") {
			a += i + 1
		}
		b = a + len(balanced(tok[a:]))
		if strings.HasSuffix(tok[a:b], ")") && !pathPlace.MatchString(tok[a:b]) {
			if i := opening(tok[a:b]); i > 0 {
				a, b = a+i+1, b-1
			}
		}
		for b > a && strings.IndexByte(",;.!?]", tok[b-1]) >= 0 {
			b--
		}
		if !pathPlace.MatchString(tok[a:b]) {
			b = a + len(trimURL(tok[a:b]))
		}
		if b <= a {
			return "", 0, 0
		}
	}
	name, line, col := tok[a:b], 0, 0
	if loc := pathPlace.FindStringSubmatchIndex(name); loc != nil {
		place := name
		name = place[:loc[0]]
		for i := 2; i < len(loc); i += 4 {
			if loc[i] >= 0 {
				line, _ = strconv.Atoi(place[loc[i]:loc[i+1]])
				if loc[i+2] >= 0 {
					col, _ = strconv.Atoi(place[loc[i+2]:loc[i+3]])
				}
				break
			}
		}
		if strings.HasSuffix(place, ":") {
			b-- // the colon after main.go:12:3: ends a compiler's location
		}
	}
	if quoted {
		name = name[1 : len(name)-1]
	}
	if name == "" || strings.ContainsAny(name, ":\x00\r\n") ||
		strings.IndexFunc(name, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) < 0 {
		return "", 0, 0
	}
	// Words are not paths: a name without a slash takes a letter, and an
	// extension or a line, so that version 1.2.3 and 12:30 are neither.
	if !strings.Contains(name, "/") {
		dot := strings.LastIndexByte(name, '.')
		if strings.IndexFunc(name, unicode.IsLetter) < 0 || line == 0 && (dot < 0 || dot == len(name)-1) {
			return "", 0, 0
		}
	}
	path = name
	if line > 0 {
		path += ":" + strconv.Itoa(line)
		if col > 0 {
			path += ":" + strconv.Itoa(col)
		}
	}
	return path, a, b
}

// balanced returns s up to a closing bracket it did not open, as the ) of
// "(image.png)已保存".
func balanced(s string) string {
	var depth [3]int
	for i, r := range s {
		if k := strings.IndexRune("([{", r); k >= 0 {
			depth[k]++
		} else if k := strings.IndexRune(")]}", r); k >= 0 {
			if depth[k] == 0 {
				return s[:i]
			}
			depth[k]--
		}
	}
	return s
}

// opening returns the index of the ( that opens the ) s ends with, or -1.
func opening(s string) int {
	depth := 0
	for i := len(s) - 1; i >= 0; i-- {
		switch s[i] {
		case ')':
			depth++
		case '(':
			if depth--; depth == 0 {
				return i
			}
		}
	}
	return -1
}

// urlAt returns the URL of text, a row whose runes are in columns cols, at
// column col, and the columns it spans, x0 up to x1, or "".
func urlAt(text []rune, cols []int, col int) (url string, x0, x1 int) {
	return matchAt(urlPattern, text, cols, col)
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
