//go:build linux && (amd64 || arm64)

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/linux"
	"github.com/egoist/mygo/internal/wait"
)

// programName is the helper process of TestProgramNameByALink: it prints
// the name GLib has for the program once the app runs.
func programName() {
	mygo.App.WhenReady(func() {
		fmt.Println("program:", linux.TestProgramName())
		go mygo.App.Quit()
	})
	if err := mygo.App.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// TestProgramNameByALink starts the app through a link of another name, as
// linux.command puts in /usr/bin: it is the program of its executable all
// the same, after which GTK names the class of its windows, as the desktop
// entry does, and WebKitGTK the directories of its pages' data. It was the
// program of the link, whose window no desktop entry named and whose pages
// found another localStorage.
func TestProgramNameByALink(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "another-name")
	if err := os.Symlink(exe, link); err != nil {
		t.Fatal(err)
	}
	ctx := wait.Context(t)
	for _, name := range []string{exe, link} {
		cmd := exec.CommandContext(ctx, name, "-test.run=^$")
		cmd.Env = append(os.Environ(), "MYGO_E2E_PROGRAM_NAME=1")
		out, err := cmd.Output()
		if want := "program: " + filepath.Base(exe); err != nil || strings.TrimSpace(string(out)) != want {
			t.Errorf("started as %s: output %q, %v, want %q", name, out, err, want)
		}
	}
}
