package editor

import (
	"testing"

	"github.com/egoist/mygo/internal/text"
)

// Core Text leaves room between Han and Latin letters and around CJK
// punctuation in Chinese, the language text without one of its own is
// shaped in when the app shows Chinese: the rows of wrapped lines stay
// within the editor all the same (#1576).
func TestCodeEditorWrapCoreTextChinese(t *testing.T) {
	sys := text.Shared()
	t.Cleanup(func() { sys.SetLanguage("") })
	sys.SetLanguage("zh-Hans")
	ct := newEditorTest(t, "", chineseText)
	ct.st.SoftWrap = true
	v := &ct.st.ed.view
	for w := 300; w <= 700; w += 19 {
		ct.tt.SetSize(w, 400)
		ct.tt.Frame()
		checkRowsFit(t, v)
	}
}
