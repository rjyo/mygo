package pacing

import "testing"

// TestSetting checks the values of MYGO_UNCAPPED: those that turn it on,
// those that leave it off, and mistakes, which leave frames paced.
func TestSetting(t *testing.T) {
	for _, tc := range []struct {
		v    string
		want bool
	}{
		{"", false}, {"0", false}, {"false", false}, {"off", false}, {"No", false},
		{"1", true}, {"true", true}, {" on ", true}, {"YES", true},
		{"2", false}, {"uncapped", false},
	} {
		if got := Setting(tc.v); got != tc.want {
			t.Errorf("Setting(%q) = %v, want %v", tc.v, got, tc.want)
		}
	}
}

// TestUncappedIgnoredInBrowsers checks that browsers, whose
// requestAnimationFrame cannot be uncapped, stay paced, and that the
// desktop platforms take the setting.
func TestUncappedIgnoredInBrowsers(t *testing.T) {
	if uncapped("1", "js") {
		t.Error("uncapped in a browser")
	}
	for _, goos := range []string{"darwin", "linux", "windows"} {
		if !uncapped("1", goos) || uncapped("0", goos) || uncapped("", goos) {
			t.Errorf("%s does not take MYGO_UNCAPPED as it is", goos)
		}
	}
}
