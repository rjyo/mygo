package terminal

import (
	"testing"

	"github.com/egoist/mygo/plugins/terminal/internal/vt"
	"github.com/egoist/mygo/ui"
)

// AltGr's text on Windows, which comes with Control and Alt, reaches a
// program in the Kitty keyboard protocol as the text, not as Ctrl+Alt and
// the key: AltGr and + type a ~ on a German keyboard, AltGr and Q an @.
func TestAltGrText(t *testing.T) {
	loadLib(t)
	term, err := vt.NewTerminal(20, 3, vt.Effects{})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Free()
	term.Write([]byte("\x1b[>1u")) // disambiguate, as Claude Code asks
	e, err := vt.NewKeyEncoder()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Free()
	altGr := ui.Ctrl | ui.Alt
	for _, c := range []struct {
		key       vt.Key
		unshifted rune
		text      string
	}{{vt.KeyBracketRight, ']', "~"}, {vt.KeyQ, 'q', "@"}} {
		r := []rune(c.text)[0]
		e.Sync(term, false)
		ev := vt.KeyEvent{Action: vt.KeyPress, Key: c.key, Mods: vtMods(altGr), Text: c.text, Unshifted: c.unshifted,
			Consumed: consumedMods(altGr, r, c.unshifted, true)}
		if got := string(e.Encode(ev)); got != c.text {
			t.Errorf("AltGr typing %q sends %q", c.text, got)
		}
	}
	// Elsewhere Control stays: Ctrl+Option+] is no AltGr on macOS.
	if m := consumedMods(altGr, '~', ']', false); m != vtMods(ui.Alt) {
		t.Errorf("consumed off Windows = %d, want Alt", m)
	}
}
