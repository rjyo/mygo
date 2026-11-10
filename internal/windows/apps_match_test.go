package windows

import (
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

func TestApplicationByExecutable(t *testing.T) {
	apps := []platform.Application{
		{Name: "Other", ID: `C:\Start Menu\Other.lnk`, Path: `shell:AppsFolder\Other`},
		{Name: "Editor", ID: `C:\Start Menu\Editor.lnk`, Path: `shell:AppsFolder\Editor`},
	}
	targets := map[string]string{
		apps[0].ID: `C:\Apps\Other.exe`,
		apps[1].ID: `C:\Apps\Editor.exe`,
	}
	got, ok := applicationByExecutable(apps, `c:\apps\EDITOR.EXE`, func(app platform.Application) string {
		return targets[app.ID]
	})
	if !ok || got != apps[1] {
		t.Fatalf("applicationByExecutable = %#v, %v; want %#v, true", got, ok, apps[1])
	}
}

func TestAssocStrAppID(t *testing.T) {
	if assocstrAppID != 21 {
		t.Fatalf("ASSOCSTR_APPID = %d, want 21", assocstrAppID)
	}
}

func TestIsOpenWithExecutable(t *testing.T) {
	for _, path := range []string{`C:\Windows\System32\OpenWith.exe`, `openwith.EXE`} {
		if !isOpenWithExecutable(path) {
			t.Errorf("isOpenWithExecutable(%q) = false, want true", path)
		}
	}
	if isOpenWithExecutable(`C:\Apps\Excel.exe`) {
		t.Error("isOpenWithExecutable matched a real application")
	}
}
