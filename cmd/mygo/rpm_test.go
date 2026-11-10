package main

import (
	"archive/tar"
	"crypto/sha256"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// rpmQuery reads a package with the system RPM tool, using stable output.
func rpmQuery(t *testing.T, rpm, file string, args ...string) string {
	t.Helper()
	cmd := exec.Command(rpm, append(args, file)...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rpm %q: %v\n%s", cmd.Args[1:], err, out)
	}
	return string(out)
}

// rpmRequirements returns application requirements, apart from the RPM
// format features the package manager itself provides.
func rpmRequirements(t *testing.T, rpm, file string) []string {
	t.Helper()
	var depends []string
	for _, dep := range strings.Split(strings.TrimSpace(rpmQuery(t, rpm, file, "-qp", "--requires")), "\n") {
		if !strings.HasPrefix(dep, "rpmlib(") {
			depends = append(depends, dep)
		}
	}
	slices.Sort(depends)
	return depends
}

func TestRpmVersion(t *testing.T) {
	for v, want := range map[string]string{
		"1.2.0":                "1.2.0",
		"v1.2.0":               "1.2.0",
		"1.2.0-beta.1":         "1.2.0~beta.1",
		"1.2.0-rc.1":           "1.2.0~rc.1",
		"1.2.0-alpha-2":        "1.2.0~alpha_2",
		"1.2.0-1":              "1.2.0~1",
		"1.2.0-0.3.7":          "1.2.0~0.3.7",
		"1.2.0+build.5":        "1.2.0+build.5",
		"1.2.0-beta.1+exp.sha": "1.2.0~beta.1+exp.sha",
		"1.2 final":            "1.2_final",
		"":                     "0",
	} {
		if got := rpmVersion(v); got != want {
			t.Errorf("rpmVersion(%q) = %q, want %q", v, got, want)
		}
	}
	// rpm, when it is there, checks the order of pre-releases and releases.
	rpm, err := exec.LookPath("rpm")
	if err != nil {
		return
	}
	sorted := []string{"1.1.9", "1.2.0-alpha", "1.2.0-beta", "1.2.0-beta.1", "1.2.0-beta.2", "1.2.0-beta.10", "1.2.0-rc.1", "1.2.0", "1.2.0+build.1", "1.2.1"}
	for i := 1; i < len(sorted); i++ {
		a, b := rpmVersion(sorted[i-1]), rpmVersion(sorted[i])
		out, err := exec.Command(rpm, "--eval", fmt.Sprintf(`%%{lua:print(rpm.vercmp(%q, %q))}`, a, b)).CombinedOutput()
		if err != nil || strings.TrimSpace(string(out)) != "-1" {
			t.Errorf("rpm.vercmp(%q, %q) = %s, %v; want -1", a, b, out, err)
		}
	}
}

func TestRpmDepends(t *testing.T) {
	t.Run("legacy", func(t *testing.T) {
		for _, tc := range []struct {
			arch              string
			webview           *bool
			deb               []string
			want, wantUnknown []string
		}{
			{
				arch:    "amd64",
				webview: nil,
				deb:     nil,
				want:    []string{"libgtk-3.so.0()(64bit)", "libwebkit2gtk-4.1.so.0()(64bit)"},
			},
			{
				arch:    "386",
				webview: new(false),
				deb:     nil,
				want:    []string{"libgtk-3.so.0"},
			},
			{
				arch:        "amd64",
				webview:     new(false),
				deb:         []string{"libnotify4", "libasound2t64 (>= 1.2)", "libfoo1 | libsecret-1-0:any", "libgtk-3-0", "libfoo2", "curl"},
				want:        []string{"libgtk-3.so.0()(64bit)", "libnotify.so.4()(64bit)", "libasound.so.2()(64bit)", "libsecret-1.so.0()(64bit)", "curl"},
				wantUnknown: []string{"libfoo2"},
			},
			{
				arch:        "arm",
				webview:     new(false),
				deb:         []string{"libnotify4", "curl", "unknown-pkg"},
				want:        []string{"libgtk-3.so.0", "libnotify.so.4", "curl"},
				wantUnknown: []string{"unknown-pkg"},
			},
			{
				arch:    "amd64",
				webview: new(false),
				deb:     []string{"libglib2.0-0t64", "libglib2.0-0", "ffmpeg", "libayatana-appindicator3-1"},
				want: []string{
					"libgtk-3.so.0()(64bit)", "libglib-2.0.so.0()(64bit)",
					"libgio-2.0.so.0()(64bit)", "libgobject-2.0.so.0()(64bit)",
					"libgmodule-2.0.so.0()(64bit)", "libgthread-2.0.so.0()(64bit)",
					"/usr/bin/ffmpeg", "libayatana-appindicator3.so.1()(64bit)",
				},
			},
			{
				arch:    "arm",
				webview: new(false),
				deb:     []string{"libglib2.0-0", "ffmpeg"},
				want: []string{
					"libgtk-3.so.0", "libglib-2.0.so.0", "libgio-2.0.so.0",
					"libgobject-2.0.so.0", "libgmodule-2.0.so.0", "libgthread-2.0.so.0",
					"/usr/bin/ffmpeg",
				},
			},
			{
				arch:    "amd64",
				webview: new(false),
				deb: []string{
					"gstreamer1.0-plugins-base", "gstreamer1.0-plugins-good",
					"gstreamer1.0-plugins-bad", "gstreamer1.0-plugins-ugly",
					"gstreamer1.0-libav", "gstreamer1.0-plugins-base",
				},
				want: []string{"libgtk-3.so.0()(64bit)"},
				wantUnknown: []string{
					"gstreamer1.0-plugins-base", "gstreamer1.0-plugins-good",
					"gstreamer1.0-plugins-bad", "gstreamer1.0-plugins-ugly",
					"gstreamer1.0-libav", "gstreamer1.0-plugins-base",
				},
			},
		} {
			l := Linux{WebView: tc.webview, Depends: linuxDepends{Deb: tc.deb, legacy: true}}
			got, unknown := l.rpmDepends(tc.arch)
			if !slices.Equal(got, tc.want) || !slices.Equal(unknown, tc.wantUnknown) {
				t.Errorf("arch %s, webview %v, deb %q: got %q, %q; want %q, %q", tc.arch, l.webView(), tc.deb, got, unknown, tc.want, tc.wantUnknown)
			}
		}
	})

	t.Run("object", func(t *testing.T) {
		for _, tc := range []struct {
			arch    string
			webview *bool
			rpm     []string
			want    []string
		}{
			{"amd64", nil, nil, []string{"libgtk-3.so.0()(64bit)", "libwebkit2gtk-4.1.so.0()(64bit)"}},
			{"arm64", new(true), nil, []string{"libgtk-3.so.0()(64bit)", "libwebkit2gtk-4.1.so.0()(64bit)"}},
			{"riscv64", new(false), nil, []string{"libgtk-3.so.0()(64bit)"}},
			{"386", nil, nil, []string{"libgtk-3.so.0", "libwebkit2gtk-4.1.so.0"}},
			{"arm", new(false), nil, []string{"libgtk-3.so.0"}},
			{"amd64", new(false), []string{"libnotify >= 0.8", "libasound.so.2()(64bit)"}, []string{"libgtk-3.so.0()(64bit)", "libnotify >= 0.8", "libasound.so.2()(64bit)"}},
			{"amd64", new(false), []string{"ffmpeg", "gstreamer1-plugins-base"}, []string{"libgtk-3.so.0()(64bit)", "ffmpeg", "gstreamer1-plugins-base"}},
		} {
			l := Linux{WebView: tc.webview, Depends: linuxDepends{RPM: tc.rpm, legacy: false}}
			got, unknown := l.rpmDepends(tc.arch)
			if !slices.Equal(got, tc.want) || len(unknown) != 0 {
				t.Errorf("arch %s, webview %v, rpm %q: got %q, unknown %q; want %q", tc.arch, l.webView(), tc.rpm, got, unknown, tc.want)
			}
		}
	})
}

func TestRPMMetadata(t *testing.T) {
	rpm, _ := exec.LookPath("rpm")
	for _, tc := range []struct {
		goarch, arch string
	}{
		{"arm64", "aarch64"},
		{"386", "i386"},
		{"arm", "armv7hl"},
		{"riscv64", "riscv64"},
	} {
		c := &Config{Name: "My App", Version: "1.0.0", License: "MIT"}
		c.Linux.WebView = new(false)
		dir := t.TempDir()
		written, err := writeRpm(c, dir, "my-app", tc.goarch, nil)
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(dir, "my-app-1.0.0-1."+tc.arch+".rpm"); written != want {
			t.Errorf("arch %s: wrote %q, want %q", tc.goarch, written, want)
		}
		if rpm == "" {
			continue
		}
		out := rpmQuery(t, rpm, written, "-qp", "--qf", "%{ARCH}\n%{SUMMARY}\n%{DESCRIPTION}\n%{LICENSE}\n")
		if want := tc.arch + "\nMy App\nMy App\nMIT\n"; out != want {
			t.Errorf("arch %s: RPM metadata %q, want %q", tc.goarch, out, want)
		}
	}
}

func TestRPMLegacyDepends(t *testing.T) {
	c := &Config{Name: "My App", Version: "1.0.0", License: "MIT"}
	c.Linux.WebView = new(false)
	c.Linux.Depends = linuxDepends{
		Deb:    []string{"libglib2.0-0", "ffmpeg", "libayatana-appindicator3-1"},
		legacy: true,
	}
	written, err := writeRpm(c, t.TempDir(), "my-app", "amd64", nil)
	if err != nil {
		t.Fatal(err)
	}
	rpm, err := exec.LookPath("rpm")
	if err != nil {
		t.Skip("no rpm to read the package")
	}
	want := []string{
		"libgtk-3.so.0()(64bit)", "libglib-2.0.so.0()(64bit)",
		"libgio-2.0.so.0()(64bit)", "libgobject-2.0.so.0()(64bit)",
		"libgmodule-2.0.so.0()(64bit)", "libgthread-2.0.so.0()(64bit)",
		"/usr/bin/ffmpeg", "libayatana-appindicator3.so.1()(64bit)",
	}
	slices.Sort(want)
	if got := rpmRequirements(t, rpm, written); !slices.Equal(got, want) {
		t.Errorf("Requires %q, want %q", got, want)
	}
}

// TestRPM packages the same app as the Debian package, checking metadata,
// file modes, link targets and content digests with the system's RPM reader.
func TestRPM(t *testing.T) {
	root := t.TempDir()
	f, err := os.Create(filepath.Join(root, "icon.png"))
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(f, image.NewNRGBA(image.Rect(0, 0, 300, 300)))
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	c := &Config{
		root: root, Name: "Note's Pad", Slug: "note-pad", Version: "1.2.0-beta.1", Icon: "icon.png",
		License:          "MIT",
		URLSchemes:       []string{"notepad"},
		FileAssociations: []FileAssociation{{Ext: []string{"npad"}, Name: "Note <Pad>"}},
	}
	c.Linux.Comment = "Take $notes `now` & 'then'"
	c.Linux.Maintainer = "Jane Doe <jane@example.com>"
	c.Linux.Command = commandNames{"note-pad", "np"}
	c.Linux.Depends = linuxDepends{
		Deb: []string{"libnotify4"},
		RPM: []string{"libnotify >= 0.8"},
	}
	stage := t.TempDir()
	for name, mode := range map[string]fs.FileMode{"note-pad": 0o755, "tool.sh": 0o700, "data/it's a file.txt": 0o600, "-v": 0o644} {
		p := filepath.Join(stage, name)
		if os.MkdirAll(filepath.Dir(p), 0o755) != nil || os.WriteFile(p, []byte(name), mode) != nil {
			t.Fatal("cannot write " + name)
		}
	}
	if err := os.Mkdir(filepath.Join(stage, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("data", filepath.Join(stage, "data-link")); err != nil {
		t.Fatal(err)
	}
	if _, err := writeLinuxDesktop(c, stage, "note-pad"); err != nil {
		t.Fatal(err)
	}
	app, err := entryNames(stage)
	if err != nil {
		t.Fatal(err)
	}
	deb, err := writeDeb(c, stage, "note-pad", "amd64", app)
	if err != nil {
		t.Fatal(err)
	}
	written, err := writeRpm(c, stage, "note-pad", "amd64", app)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(stage, "note-pad-1.2.0~beta.1-1.x86_64.rpm"); written != want {
		t.Errorf("wrote %q, want %q", written, want)
	}
	rpm, err := exec.LookPath("rpm")
	if err != nil {
		t.Skip("no rpm to read the package")
	}
	fields := rpmQuery(t, rpm, written, "-qp", "--qf", "%{NAME}\n%{VERSION}\n%{RELEASE}\n%{ARCH}\n%{PACKAGER}\n%{SUMMARY}\n%{DESCRIPTION}\n%{LICENSE}\n%{FILEDIGESTALGO}\n")
	wantFields := "note-pad\n1.2.0~beta.1\n1\nx86_64\nJane Doe <jane@example.com>\n" + c.Linux.Comment + "\n" + c.Linux.Comment + "\nMIT\n8\n"
	if fields != wantFields {
		t.Errorf("RPM fields %q, want %q", fields, wantFields)
	}
	depends := rpmRequirements(t, rpm, written)
	wantDepends := []string{"libgtk-3.so.0()(64bit)", "libwebkit2gtk-4.1.so.0()(64bit)", "libnotify >= 0.8"}
	slices.Sort(wantDepends)
	if !slices.Equal(depends, wantDepends) {
		t.Errorf("Requires %q, want %q", depends, wantDepends)
	}
	if out := rpmQuery(t, rpm, written, "-K"); !strings.Contains(strings.ToLower(out), "digests ok") {
		t.Errorf("RPM digests: %s", out)
	}

	// Shared parent directories belong to system packages. Keep only the
	// app's own directories when comparing the DEB's directory entries.
	wantFiles := map[string]string{}
	debEntries(t, deb, "data.tar.gz", func(hdr *tar.Header, content []byte) {
		name := "/" + strings.TrimSuffix(strings.TrimPrefix(hdr.Name, "./"), "/")
		mode, link, digest := hdr.Mode, "", ""
		switch hdr.Typeflag {
		case tar.TypeDir:
			if name != "/opt/note-pad" && !strings.HasPrefix(name, "/opt/note-pad/") {
				return
			}
			mode |= 0o40000
		case tar.TypeSymlink:
			mode |= 0o120000
			link = hdr.Linkname
		default:
			mode |= 0o100000
			digest = fmt.Sprintf("%x", sha256.Sum256(content))
		}
		wantFiles[name] = fmt.Sprintf("%o\t%s\t%s\troot\troot", mode, link, digest)
	})
	files := map[string]string{}
	entries := rpmQuery(t, rpm, written, "-qp", "--qf", "[%{FILENAMES}\t%{FILEMODES:octal}\t%{FILELINKTOS}\t%{FILEDIGESTS}\t%{FILEUSERNAME}\t%{FILEGROUPNAME}\n]")
	for _, line := range strings.Split(strings.TrimSuffix(entries, "\n"), "\n") {
		name, attrs, _ := strings.Cut(line, "\t")
		files[name] = attrs
	}
	if !maps.Equal(files, wantFiles) {
		for _, name := range slices.Sorted(maps.Keys(wantFiles)) {
			if files[name] != wantFiles[name] {
				t.Errorf("%s: RPM has %q, want %q", name, files[name], wantFiles[name])
			}
		}
		for _, name := range slices.Sorted(maps.Keys(files)) {
			if _, ok := wantFiles[name]; !ok {
				t.Errorf("unexpected RPM entry %s", name)
			}
		}
	}
}
