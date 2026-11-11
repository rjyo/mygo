package ui_test

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

func wantTold(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s = %q, want %q", what, got, want)
	}
}

// TestTesterStatusRead checks that a status whose text changes is read
// out once, and not again on the frames after it.
func TestTesterStatusRead(t *testing.T) {
	saved := 0
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Text(c, "Saved "+string(rune('0'+saved))+" files").Role(ui.RoleStatus)
	}, 300, 100)
	tt.UseScreenReader()
	wantTold(t, "Announcements at attach", tt.Announcements())
	saved = 2
	tt.Frame()
	wantTold(t, "Announcements", tt.Announcements(), "Saved 2 files")
	tt.Frame()
	tt.Frame()
	wantTold(t, "Announcements later", tt.Announcements())
	wantTold(t, "Alerts", tt.Alerts())
}

// TestTesterStatusBeforeScreenReader checks that what a status showed
// before the screen reader attached is not read.
func TestTesterStatusBeforeScreenReader(t *testing.T) {
	text := "Ready"
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Text(c, text).Role(ui.RoleStatus)
	}, 300, 100)
	tt.Frame()
	tt.UseScreenReader()
	tt.Frame()
	wantTold(t, "Announcements at attach", tt.Announcements())
	text = "Done"
	tt.Frame()
	wantTold(t, "Announcements", tt.Announcements(), "Done")
}

// TestTesterLogRead checks that what is added to a log is read once.
func TestTesterLogRead(t *testing.T) {
	var lines []string
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Role(ui.RoleLog).Label("Chat").Children(func() {
			for _, l := range lines {
				ui.Text(c, l)
			}
		})
	}, 300, 200)
	lines = []string{"hi"}
	tt.Frame()
	tt.UseScreenReader()
	lines = append(lines, "hello again")
	tt.Frame()
	wantTold(t, "Announcements", tt.Announcements(), "hello again")
	tt.Frame()
	wantTold(t, "Announcements later", tt.Announcements())
}

// TestTesterAlertRead checks that an alert is in Alerts, not in
// Announcements.
func TestTesterAlertRead(t *testing.T) {
	show := false
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Text(c, "Page")
		if show {
			ui.Text(c, "Disk full").Role(ui.RoleAlert)
		}
	}, 300, 100)
	tt.UseScreenReader()
	show = true
	tt.Frame()
	wantTold(t, "Announcements", tt.Announcements())
	wantTold(t, "Alerts", tt.Alerts(), "Disk full")
	tt.Frame()
	wantTold(t, "Alerts later", tt.Alerts())
}

// TestTesterAnnounceAndStatus checks that an announcement and a status
// of one frame are each read once.
func TestTesterAnnounceAndStatus(t *testing.T) {
	text, say := "A", false
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Text(c, text).Role(ui.RoleStatus)
		if say {
			c.Announce("Hello")
		}
	}, 300, 100)
	tt.UseScreenReader()
	text, say = "B", true
	tt.Frame()
	got := tt.Announcements()
	slices.Sort(got)
	wantTold(t, "Announcements", got, "B", "Hello")
	say = false
	tt.Frame()
	wantTold(t, "Announcements later", tt.Announcements())
}

// TestTesterAccessKeepsAnnouncements checks that reading the tree between
// frames neither swallows nor repeats a status announcement.
func TestTesterAccessKeepsAnnouncements(t *testing.T) {
	text := "A"
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Text(c, text).Role(ui.RoleStatus)
	}, 300, 100)
	tt.UseScreenReader()
	text = "B"
	tt.Frame()
	tt.AccessAll()
	tt.Access("B")
	tt.Frame()
	tt.AccessAll()
	wantTold(t, "Announcements", tt.Announcements(), "B")
	tt.AccessAll()
	wantTold(t, "Announcements later", tt.Announcements())
}

// TestTesterAnnouncementsWithoutScreenReader checks that Announce texts
// are returned as before, and no alert is.
func TestTesterAnnouncementsWithoutScreenReader(t *testing.T) {
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Text(c, "x").Role(ui.RoleAlert)
		if ui.Button(c, "Go").Clicked() {
			c.Announce("Went")
		}
	}, 300, 100)
	tt.Click("Go")
	wantTold(t, "Announcements", tt.Announcements(), "Went")
	wantTold(t, "Announcements again", tt.Announcements())
	wantTold(t, "Alerts", tt.Alerts())
}
