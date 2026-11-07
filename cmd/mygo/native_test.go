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
	"strings"
	"testing"
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

func TestPrebuiltNativeModule(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", "")
	dir := t.TempDir()
	src := filepath.Join(dir, "apple", "Sources", "Thing")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(path, s string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(dir, "apple", "Package.swift"), "// swift-tools-version: 6.0\n")
	write(filepath.Join(src, "Thing.swift"), "func a() {}\n")
	hash, err := swiftSourceHash(dir)
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
	p := &prebuilt{Source: hash, File: nativeFile{Name: "thing.tar.gz", URL: srv.URL + "/thing.tar.gz", SHA256: hex.EncodeToString(sum[:])}}
	if err := writePrebuilt(dir, "apple", p); err != nil {
		t.Fatal(err)
	}
	got, err := readPrebuilt(dir, "apple")
	if err != nil || got == nil || got.Source != hash {
		t.Fatalf("readPrebuilt = %+v, %v", got, err)
	}
	b, err := usePrebuilt(dir, "Thing", got)
	if err != nil {
		t.Fatal(err)
	}
	if lib, _ := os.ReadFile(b.dylib); string(lib) != "a library" || len(b.frameworks) != 1 {
		t.Fatalf("build %+v", b)
	}
	if link, err := os.Readlink(filepath.Join(b.frameworks[0], "Dep")); err != nil || filepath.ToSlash(link) != "Versions/A/Dep" {
		t.Fatalf("the framework's link: %q, %v", link, err)
	}

	// A change of the sources makes it stale.
	write(filepath.Join(src, "Thing.swift"), "func b() {}\n")
	if h, _ := swiftSourceHash(dir); h == hash {
		t.Fatal("the hash did not change with the sources")
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
