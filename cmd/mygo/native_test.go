package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/egoist/mygo/internal/nativegen"
	"github.com/egoist/mygo/internal/nativelib"
)

func TestMergeModulePlists(t *testing.T) {
	a, err := parsePlist([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
	<key>NSSpeechRecognitionUsageDescription</key><string>Recognize speech.</string>
	<key>com.apple.security.application-groups</key><array><string>group.a</string></array>
	<key>Count</key><integer>3</integer>
	<key>On</key><true/>
</dict></plist>`))
	if err != nil {
		t.Fatal(err)
	}
	b, err := parsePlist([]byte(`<plist version="1.0"><dict>
	<key>com.apple.security.application-groups</key><array><string>group.a</string><string>group.b</string></array>
	<key>NSSpeechRecognitionUsageDescription</key><string>Recognize speech.</string>
</dict></plist>`))
	if err != nil {
		t.Fatal(err)
	}
	merged := map[string]any{}
	owners := map[string]string{}
	if err := mergePlist(merged, a, "modules/a", owners, nil, ""); err != nil {
		t.Fatal(err)
	}
	if err := mergePlist(merged, b, "modules/b", owners, nil, ""); err != nil {
		t.Fatal(err)
	}
	if groups := merged["com.apple.security.application-groups"].([]any); len(groups) != 2 {
		t.Errorf("arrays merge by their values: %v", groups)
	}
	if merged["Count"] != 3.0 || merged["On"] != true {
		t.Errorf("merged = %v", merged)
	}
	c := map[string]any{"NSSpeechRecognitionUsageDescription": "Other words."}
	err = mergePlist(merged, c, "modules/c", owners, nil, "")
	if err == nil || !strings.Contains(err.Error(), "native modules modules/a and modules/c set NSSpeechRecognitionUsageDescription") {
		t.Errorf("conflicting modules: %v", err)
	}
	// The app setting the key settles it.
	app := map[string]bool{"NSSpeechRecognitionUsageDescription": true}
	if err := mergePlist(merged, c, "modules/c", owners, app, ""); err != nil {
		t.Errorf("a key the app sets still conflicts: %v", err)
	}
}

func TestPublishedSwiftLibrary(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", "")
	dir := t.TempDir()
	src := filepath.Join(dir, "swift", "Sources", "Thing")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(path, s string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(dir, "swift", "Package.swift"), "// swift-tools-version: 6.0\n")
	write(filepath.Join(src, "Thing.swift"), "func a() {}\n")
	m := &nativegen.Module{Dir: dir, Name: "Thing"}
	hash, err := moduleSourceHash(m)
	if err != nil {
		t.Fatal(err)
	}

	// The archive: the library and a framework, with a link in it.
	built := t.TempDir()
	write(filepath.Join(built, "libThing.dylib"), "a library")
	fw := filepath.Join(built, "Dep.framework")
	if err := os.MkdirAll(filepath.Join(fw, "Versions", "A"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(fw, "Versions", "A", "Dep"), "a framework")
	if err := os.Symlink("Versions/A/Dep", filepath.Join(fw, "Dep")); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "thing.tar.gz")
	if err := tarDir(archive, []string{filepath.Join(built, "libThing.dylib"), fw}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(data) }))
	defer srv.Close()
	sum := sha256.Sum256(data)
	f := nativelib.File{Name: "Thing-darwin-universal.tar.gz", URL: srv.URL + "/thing.tar.gz", SHA256: hex.EncodeToString(sum[:]), SourceHash: hash}

	// The module's manifest names it, under the universal build, with no
	// build: the module's languages are how it builds.
	lib := nativelib.Library{Name: "Thing", Files: map[string]nativelib.File{nativelib.Universal: f}}
	if err := writeManifest(filepath.Join(dir, pluginFile), nativelib.Manifest{Libraries: []nativelib.Library{lib}}); err != nil {
		t.Fatal(err)
	}
	got, err := moduleLibrary(m, "swift")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Build.IsSwift() {
		t.Fatalf("the build: %+v", got.Build)
	}
	for _, arch := range []string{"arm64", "amd64"} {
		if g, ok := got.File("darwin", arch); !ok || g != f {
			t.Fatalf("the build for darwin/%s: %+v, %v", arch, g, ok)
		}
	}
	b, err := usePublishedSwift(dir, "Thing", f)
	if err != nil {
		t.Fatal(err)
	}
	if lib, _ := os.ReadFile(b.dylib); string(lib) != "a library" || len(b.frameworks) != 1 {
		t.Fatalf("build %+v", b)
	}
	if link, err := os.Readlink(filepath.Join(b.frameworks[0], "Dep")); err != nil || filepath.ToSlash(link) != "Versions/A/Dep" {
		t.Fatalf("the framework's link: %q, %v", link, err)
	}

	// A checkout with CRLF line endings, as git makes on Windows, has the
	// same sources.
	write(filepath.Join(src, "Thing.swift"), "func a() {}\r\n")
	if h, _ := moduleSourceHash(m); h != hash {
		t.Fatal("the hash changed with the line endings")
	}

	// A change of the sources makes it stale.
	write(filepath.Join(src, "Thing.swift"), "func b() {}\n")
	if h, _ := moduleSourceHash(m); h == hash {
		t.Fatal("the hash did not change with the sources")
	}
}

// A module's published build goes stale with the modules it uses, whose
// Swift it imports and links.
func TestModuleSourceHashDeps(t *testing.T) {
	swift := func(dir, src string) {
		t.Helper()
		path := filepath.Join(dir, "swift", "Sources", "X", "X.swift")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a, b := t.TempDir(), t.TempDir()
	swift(a, "func a() {}\n")
	swift(b, "func b() {}\n")
	dep := &nativegen.Module{Dir: b}
	m := &nativegen.Module{Dir: a, Deps: map[string]*nativegen.Module{"example.com/b": dep}}
	alone, err := moduleSourceHash(&nativegen.Module{Dir: a})
	if err != nil {
		t.Fatal(err)
	}
	if own, _ := nativelib.SourceHash(filepath.Join(a, "swift"), &nativelib.Build{Swift: "./swift"}); own != alone {
		t.Fatalf("a module that uses none: %s, its package's %s", alone, own)
	}
	before, err := moduleSourceHash(m)
	if err != nil {
		t.Fatal(err)
	}
	if before == alone {
		t.Fatal("the hash does not cover the module it uses")
	}
	swift(b, "func c() {}\n")
	if after, _ := moduleSourceHash(m); after == before {
		t.Fatal("the hash did not change with the module it uses")
	}
}

// fakeGH puts a gh on PATH that keeps releases in a directory, logs its
// arguments, and fails on anything but release view, create and upload.
func fakeGH(t *testing.T) (log func() []string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake gh is a shell script")
	}
	bin, state := t.TempDir(), t.TempDir()
	script := `#!/bin/sh
echo "$*" >> "$STATE/log"
case "$1 $2" in
"release view")
	[ -d "$STATE/$3" ] || { echo "release not found" >&2; exit 1; }
	ls "$STATE/$3" ;;
"release create")
	mkdir "$STATE/$3" ;;
"release upload")
	tag="$3"
	[ -d "$STATE/$tag" ] || exit 1
	for a in "$@"; do [ "$a" = --clobber ] && clobber=1; done
	shift 5 # release upload <tag> --repo <repo>
	for f in "$@"; do
		[ "$f" = --clobber ] && continue
		n=$(basename "$f")
		if [ -e "$STATE/$tag/$n" ] && [ -z "$clobber" ]; then echo "asset under the same name already exists" >&2; exit 1; fi
		cp "$f" "$STATE/$tag/$n"
	done ;;
*)
	exit 2 ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STATE", state)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func() []string {
		data, _ := os.ReadFile(filepath.Join(state, "log"))
		return strings.Split(strings.TrimSpace(string(data)), "\n")
	}
}

func TestUploadNatives(t *testing.T) {
	log := fakeGH(t)
	dir := t.TempDir()
	a := filepath.Join(dir, "thing-darwin-universal-abababababab.tar.gz")
	b := filepath.Join(dir, "adder-linux-amd64-cdcdcdcdcdcd.so")
	for _, f := range []string{a, b} {
		if err := os.WriteFile(f, []byte(f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	release := "https://github.com/me/app/releases/download/" + defaultReleaseTag("Adder")
	if err := uploadNatives(release, []string{a, b}); err != nil {
		t.Fatal(err)
	}
	got := log()
	if len(got) != 4 || got[0] != "release view native-adder --repo me/app" ||
		!strings.HasPrefix(got[1], "release create native-adder --repo me/app --title native-adder --notes ") ||
		!strings.HasSuffix(got[1], " --latest=false") ||
		!strings.HasPrefix(got[2], "release view native-adder --repo me/app --json assets") ||
		got[3] != "release upload native-adder --repo me/app "+b+" "+a {
		t.Fatalf("gh ran:\n%s", strings.Join(got, "\n"))
	}

	// The same files again are there already: they are neither uploaded
	// nor replaced, and the release is not made again.
	if err := uploadNatives(release, []string{a, b}); err != nil {
		t.Fatal(err)
	}
	if got := log(); len(got) != 6 || !strings.HasPrefix(got[4], "release view ") || !strings.HasPrefix(got[5], "release view ") {
		t.Fatalf("gh ran:\n%s", strings.Join(got, "\n"))
	}
	for _, l := range log() {
		if strings.Contains(l, "--clobber") {
			t.Fatalf("gh replaced a file: %s", l)
		}
	}
}

// A module whose published build matches its sources is not built again,
// nor uploaded, unless -force.
func TestNativeBuildSwiftUpToDate(t *testing.T) {
	log := fakeGH(t)
	dir := t.TempDir()
	src := filepath.Join("..", "..", "examples", "native-module", "modules", "motion")
	if err := os.CopyFS(dir, os.DirFS(src)); err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(filepath.Join(dir, "swift", ".build"))
	m, err := nativegen.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := moduleSourceHash(m)
	if err != nil {
		t.Fatal(err)
	}
	f := nativelib.File{Name: "Motion-darwin-universal.tar.gz", URL: "https://example.com/x.tar.gz", SHA256: strings.Repeat("0", 64), SourceHash: hash}
	lib := nativelib.Library{Name: "Motion", Files: map[string]nativelib.File{nativelib.Universal: f}}
	if err := writeManifest(filepath.Join(dir, pluginFile), nativelib.Manifest{Libraries: []nativelib.Library{lib}}); err != nil {
		t.Fatal(err)
	}
	if err := runNativeBuild([]string{"-upload", "-release", "https://github.com/me/app/releases/download/x", dir}); err != nil {
		t.Fatal(err)
	}
	if got := log(); len(got) != 1 || got[0] != "" {
		t.Fatalf("gh ran:\n%s", strings.Join(got, "\n"))
	}
	if runtime.GOOS != "darwin" {
		err := runNativeBuild([]string{"-force", dir})
		if err == nil || !strings.Contains(err.Error(), "Mac") {
			t.Fatalf("-force off macOS: %v", err)
		}
	}
}

func TestNativePackageDependencies(t *testing.T) {
	var m swiftManifest
	err := json.Unmarshal([]byte(`{
		"dependencies": [
			{"sourceControl": [{"identity": "swift-collections", "location": {"remote": [{"urlString": "https://github.com/apple/swift-collections"}]}, "requirement": {"range": [{"lowerBound": "1.1.0", "upperBound": "2.0.0"}]}}]},
			{"sourceControl": [{"identity": "swift-algorithms", "location": {"remote": [{"urlString": "https://github.com/apple/swift-algorithms"}]}, "requirement": {"exact": ["1.2.0"]}}]},
			{"fileSystem": [{"identity": "local", "path": "/src/local"}]}
		],
		"targets": [{"name": "X", "dependencies": [{"product": ["Collections", "swift-collections", null, null]}, {"byName": ["Algorithms", null]}],
			"settings": [{"tool": "linker", "kind": {"linkedFramework": {"_0": "MapKit"}}}]}]
	}`), &m)
	if err != nil {
		t.Fatal(err)
	}
	var decls []string
	for _, d := range m.Dependencies {
		_, s := d.decl()
		decls = append(decls, s)
	}
	want := []string{
		`.package(url: "https://github.com/apple/swift-collections", "1.1.0"..<"2.0.0")`,
		`.package(url: "https://github.com/apple/swift-algorithms", exact: "1.2.0")`,
		`.package(path: "/src/local")`,
	}
	if strings.Join(decls, "\n") != strings.Join(want, "\n") {
		t.Errorf("dependencies:\n%s\nwant:\n%s", strings.Join(decls, "\n"), strings.Join(want, "\n"))
	}
	x := m.target("X")
	if got := x.Dependencies[0].decl() + " " + x.Dependencies[1].decl(); got != `.product(name: "Collections", package: "swift-collections") "Algorithms"` {
		t.Errorf("target dependencies: %s", got)
	}
}

func TestUntarStaysInside(t *testing.T) {
	outside := t.TempDir()
	for name, entries := range map[string][]tar.Header{
		"absolute link": {
			{Name: "lib", Typeflag: tar.TypeSymlink, Linkname: outside},
			{Name: "lib/authorized_keys", Typeflag: tar.TypeReg, Mode: 0o644},
		},
		"link up": {
			{Name: "a/lib", Typeflag: tar.TypeSymlink, Linkname: "../../.."},
		},
		"path up": {
			{Name: "../x", Typeflag: tar.TypeReg, Mode: 0o644},
		},
	} {
		archive := filepath.Join(t.TempDir(), "a.tar.gz")
		f, err := os.Create(archive)
		if err != nil {
			t.Fatal(err)
		}
		z := gzip.NewWriter(f)
		w := tar.NewWriter(z)
		for _, h := range entries {
			h := h
			if err := w.WriteHeader(&h); err != nil {
				t.Fatal(err)
			}
		}
		w.Close()
		z.Close()
		f.Close()
		if err := untar(archive, filepath.Join(t.TempDir(), "out")); err == nil {
			t.Errorf("%s: unpacked", name)
		}
		if entries, _ := os.ReadDir(outside); len(entries) > 0 {
			t.Fatalf("%s: wrote %s outside", name, entries[0].Name())
		}
	}
	// A link inside, as frameworks have, unpacks.
	archive := filepath.Join(t.TempDir(), "ok.tar.gz")
	f, _ := os.Create(archive)
	z := gzip.NewWriter(f)
	w := tar.NewWriter(z)
	for _, h := range []tar.Header{
		{Name: "F.framework/Versions/A/F", Typeflag: tar.TypeReg, Mode: 0o755},
		{Name: "F.framework/F", Typeflag: tar.TypeSymlink, Linkname: "Versions/A/F"},
	} {
		h := h
		w.WriteHeader(&h)
	}
	w.Close()
	z.Close()
	f.Close()
	if err := untar(archive, filepath.Join(t.TempDir(), "out")); err != nil {
		t.Errorf("a framework's link: %v", err)
	}
}
