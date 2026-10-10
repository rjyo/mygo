package ui

import "testing"

func TestOnPaste(t *testing.T) {
	text := "hello world"
	var seen []string
	tt := NewTester(func(c *Context) {
		TextArea(c, &text).Width(200).Label("draft").OnPaste(func(s string) bool {
			seen = append(seen, s)
			return len(s) > 5
		})
	}, 300, 100)
	tt.Click("draft")
	tt.Key(Cmd, KeyEnd)

	// Left: the paste inserts as usual.
	tt.SetClipboard("!")
	tt.Key(Cmd, KeyV)
	if text != "hello world!" {
		t.Fatalf("a paste left became %q", text)
	}

	// Taken, by Cmd+V and by the menu: nothing inserted, the selection
	// deleted.
	tt.SetClipboard("a large block")
	tt.Key(Cmd, KeyV)
	tt.Key(Cmd, KeyA)
	tt.Command("paste")
	if text != "" {
		t.Fatalf("a paste taken became %q", text)
	}

	// An empty clipboard, as with an image, is seen too.
	tt.SetClipboard("")
	tt.Command("paste")
	want := []string{"!", "a large block", "a large block", ""}
	if len(seen) != len(want) {
		t.Fatalf("saw %q", seen)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("saw %q", seen)
		}
	}
}
