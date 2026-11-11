package main

import (
	"regexp"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

var (
	markdownEmphasis = regexp.MustCompile("(\\*\\*|\\*|`)([^*`]+)(\\*\\*|\\*|`)")
	markdownLink     = regexp.MustCompile(`(\[)[^\]]+(\]\([^)]*\))`)
	markdownHeading  = regexp.MustCompile(`^#{1,6} `)
	markdownBullet   = regexp.MustCompile(`^\s*(-) `)
	markdownTask     = regexp.MustCompile(`^\s*(- \[[ x]\])`)
)

// livePreview shows Markdown as it reads, the file's text staying as it
// is: the marks hide, bullets show as dots and tasks as check boxes, but
// on the lines with a caret, whose source shows to edit.
func livePreview(st *editor.State) []editor.Replacement {
	b := st.Buffer
	carets := map[int]bool{}
	for _, s := range st.Selections() {
		for line := b.Pos(s.Start()).Line; line <= b.Pos(s.End()).Line; line++ {
			carets[line] = true
		}
	}
	var out []editor.Replacement
	fenced := false
	for i := range b.LineCount() {
		line := b.Line(i)
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
			continue
		}
		if fenced || carets[i] {
			continue
		}
		start := b.Offset(editor.Pos{Line: i})
		if m := markdownHeading.FindStringIndex(line); m != nil {
			out = append(out, editor.Replacement{Start: start, End: start + m[1]})
		}
		if m := markdownTask.FindStringSubmatchIndex(line); m != nil {
			out = append(out, taskBox(st, start+m[2], start+m[3]))
		} else if m := markdownBullet.FindStringSubmatchIndex(line); m != nil {
			out = append(out, editor.Replacement{Start: start + m[2], End: start + m[3], Text: "•"})
		}
		for _, m := range markdownEmphasis.FindAllStringSubmatchIndex(line, -1) {
			if line[m[2]:m[3]] == line[m[6]:m[7]] {
				out = append(out, editor.Replacement{Start: start + m[2], End: start + m[3]},
					editor.Replacement{Start: start + m[6], End: start + m[7]})
			}
		}
		for _, m := range markdownLink.FindAllStringSubmatchIndex(line, -1) {
			out = append(out, editor.Replacement{Start: start + m[2], End: start + m[3]},
				editor.Replacement{Start: start + m[4], End: start + m[5]})
		}
	}
	return out
}

// taskBox draws a check box in place of a task's "- [ ]", which a click
// toggles in the text.
func taskBox(st *editor.State, start, end int) editor.Replacement {
	done := strings.Contains(st.Buffer.Slice(start, end), "x")
	label, mark := "Not done", "☐"
	if done {
		label, mark = "Done", "☑"
	}
	return editor.Replacement{Start: start, End: end, Label: label, Interactive: true,
		Content: func(c *ui.Context, e ui.Element, r editor.Replacement) {
			e.Children(func() {
				b := ui.ButtonBase(c).Fill().Center().KeepFocus().Cursor(ui.CursorPointer).Label(label)
				b.Children(func() { ui.Text(c, mark) })
				if b.Clicked() {
					text := "x"
					if done {
						text = " "
					}
					st.Buffer.Edit(editor.Edit{Start: r.End - 2, End: r.End - 1, Text: text})
				}
			})
		}}
}
