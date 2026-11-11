package markdown

import (
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestViewCodeLink(t *testing.T) {
	files := map[string]string{"src/app.go:12": "file:src/app.go#L12"}
	var opened []string
	o := &Options{
		CodeLink: func(code string) (string, bool) { d, ok := files[code]; return d, ok },
		OnLink:   func(url string) { opened = append(opened, url) },
	}
	src := "See `src/app.go:12` and `go test`, and @octocat."
	tt := ui.NewTester(func(c *ui.Context) { View(c, src, o) }, 600, 400)
	if err := tt.Click("src/app.go:12"); err != nil {
		t.Fatalf("%v in %q", err, tt.Texts())
	}
	if !slices.Equal(opened, []string{"file:src/app.go#L12"}) {
		t.Errorf("opened %q", opened)
	}
	// Code it doesn't know, and mentions without Resolve, stay text.
	for _, s := range []string{"go test", "@octocat"} {
		if err := tt.Click(s); err == nil {
			t.Errorf("%s is a link", s)
		}
	}
	// It's asked again as the view builds: code it learns of links.
	files["go test"] = "cmd:go-test"
	tt.Frame()
	if err := tt.Click("go test"); err != nil || opened[len(opened)-1] != "cmd:go-test" {
		t.Errorf("go test once known: %v, opened %q", err, opened)
	}
	// Cleared, it's all text again, as one run.
	o.CodeLink = nil
	tt.Frame()
	if err := tt.Click("src/app.go:12"); err == nil {
		t.Error("still a link once CodeLink is cleared")
	}
	if !tt.HasText("See src/app.go:12 and go test, and @octocat.") {
		t.Errorf("text %q", tt.Texts())
	}
}

func TestViewCodeLinkStreaming(t *testing.T) {
	asked := 0
	o := &Options{Reveal: &Reveal{Unit: AtOnce, Fade: -1}, CodeLink: func(code string) (string, bool) {
		asked++
		return "file:" + code, strings.Contains(code, "/")
	}}
	var src string
	tt := ui.NewTester(func(c *ui.Context) { Stream(c, src, o) }, 600, 400)
	for _, part := range []string{"Open `a/b", ".go` now", " and `c/d.go`."} {
		src += part
		tt.Frame()
	}
	if err := tt.Click("c/d.go"); err != nil {
		t.Fatalf("%v in %q", err, tt.Texts())
	}
	if err := tt.Click("a/b.go"); err != nil {
		t.Fatalf("%v in %q", err, tt.Texts())
	}
	if asked == 0 {
		t.Error("never asked")
	}
}
