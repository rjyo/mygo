package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"

	"github.com/egoist/mygo/internal/download"
	"github.com/egoist/mygo/internal/nativegen"
)

// A prebuilt implementation of a native module, for machines without the
// platform's tools: mygo native prebuild builds it, uploads it as a release
// asset of the app's repository and records it in the module's
// mygo-plugin.json, beside what plugins list there:
//
//	{"native": {"apple": {"source": "<hash of apple/>",
//		"file": {"name": "…tar.gz", "url": "https://…", "sha256": "…"}}}}
//
// The archive holds lib<Module>.dylib, universal, and the frameworks its
// Swift package brings. prepareNative uses it where its source matches
// apple/'s, which keeps it from going stale unnoticed.

type prebuiltFile struct {
	Native map[string]*prebuilt `json:"native"`
}

type prebuilt struct {
	// Source is the hash of the Swift package it was built from.
	Source string     `json:"source"`
	File   nativeFile `json:"file"`
}

// swiftSourceHash hashes what a module's Swift package builds from: its
// manifest, its resolved dependencies and its sources.
func swiftSourceHash(dir string) (string, error) {
	apple := filepath.Join(dir, "apple")
	h := sha256.New()
	var files []string
	for _, f := range []string{"Package.swift", "Package.resolved"} {
		if _, err := os.Stat(filepath.Join(apple, f)); err == nil {
			files = append(files, f)
		}
	}
	err := filepath.WalkDir(filepath.Join(apple, "Sources"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		if !d.IsDir() && !strings.HasPrefix(d.Name(), ".") {
			rel, _ := filepath.Rel(apple, path)
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	slices.Sort(files)
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(apple, filepath.FromSlash(f)))
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", f, len(data))
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

// readPrebuilt returns the prebuilt implementation the module in dir
// records for platform, or nil.
func readPrebuilt(dir, platform string) (*prebuilt, error) {
	data, err := os.ReadFile(filepath.Join(dir, pluginFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var f prebuiltFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Join(dir, pluginFile), err)
	}
	return f.Native[platform], nil
}

// usePrebuilt downloads and unpacks a prebuilt implementation into the
// user's cache, and returns it as a build.
func usePrebuilt(dir, name string, p *prebuilt) (nativeBuild, error) {
	t := con.start("Downloading the prebuilt native module " + filepath.Base(dir))
	archive, err := fetchNative(p.File)
	if err != nil {
		return nativeBuild{}, t.end(err, "")
	}
	out := filepath.Join(filepath.Dir(archive), "unpacked")
	if _, err := os.Stat(filepath.Join(out, "lib"+name+".dylib")); err != nil {
		if err := untar(archive, out); err != nil {
			return nativeBuild{}, t.end(fmt.Errorf("%s: %w", p.File.Name, err), "")
		}
	}
	b := nativeBuild{name: name, dir: dir, dylib: filepath.Join(out, "lib"+name+".dylib")}
	entries, err := os.ReadDir(out)
	if err != nil {
		return nativeBuild{}, t.end(err, "")
	}
	for _, e := range entries {
		if p := filepath.Join(out, e.Name()); p != b.dylib {
			b.frameworks = append(b.frameworks, p)
		}
	}
	if _, err := os.Stat(b.dylib); err != nil {
		return nativeBuild{}, t.end(fmt.Errorf("%s holds no lib%s.dylib", p.File.Name, name), "")
	}
	return b, t.end(nil, "Downloaded the prebuilt native module "+filepath.Base(dir))
}

// untar unpacks a .tar.gz into dir, which it replaces.
func untar(archive, dir string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tmp := dir + ".tmp"
	os.RemoveAll(tmp)
	r := tar.NewReader(z)
	for {
		h, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return err
		}
		name := filepath.Clean(filepath.FromSlash(h.Name))
		if !filepath.IsLocal(name) {
			return fmt.Errorf("the archive holds %s, outside it", h.Name)
		}
		path := filepath.Join(tmp, name)
		switch h.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(path, 0o755)
		case tar.TypeSymlink:
			// The target, from the link's own directory, stays in the
			// archive: an absolute one never does.
			target := filepath.FromSlash(h.Linkname)
			if filepath.IsAbs(target) || h.Linkname == "" || !filepath.IsLocal(filepath.Join(filepath.Dir(name), target)) {
				return fmt.Errorf("the archive links %s to %s, out of it", h.Name, h.Linkname)
			}
			if err = os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
				err = os.Symlink(h.Linkname, path)
			}
		case tar.TypeReg:
			if err = os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				break
			}
			// Nor through a link it made before.
			var real string
			if real, err = filepath.EvalSymlinks(filepath.Dir(path)); err != nil {
				break
			}
			if root, rerr := filepath.EvalSymlinks(tmp); rerr != nil || (real != root && !strings.HasPrefix(real, root+string(filepath.Separator))) {
				err = fmt.Errorf("the archive writes %s out of it", h.Name)
				break
			}
			var w *os.File
			if w, err = os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fs.FileMode(h.Mode)&0o755|0o644); err != nil {
				break
			}
			_, err = io.Copy(w, r)
			if cerr := w.Close(); err == nil {
				err = cerr
			}
		}
		if err != nil {
			return err
		}
	}
	os.RemoveAll(dir)
	return os.Rename(tmp, dir)
}

// tarDir packs the files and directories paths, by their base names, into
// a .tar.gz at out.
func tarDir(out string, paths []string) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	z := gzip.NewWriter(f)
	w := tar.NewWriter(z)
	for _, root := range paths {
		base := filepath.Dir(root)
		err := filepath.Walk(root, func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(base, path)
			link := ""
			if info.Mode()&fs.ModeSymlink != 0 {
				if link, err = os.Readlink(path); err != nil {
					return err
				}
			}
			h, err := tar.FileInfoHeader(info, link)
			if err != nil {
				return err
			}
			h.Name = filepath.ToSlash(rel)
			h.ModTime, h.Uid, h.Gid, h.Uname, h.Gname = h.ModTime.Truncate(0).UTC(), 0, 0, "", ""
			if err := w.WriteHeader(h); err != nil {
				return err
			}
			if info.Mode().IsRegular() {
				src, err := os.Open(path)
				if err != nil {
					return err
				}
				_, err = io.Copy(w, src)
				src.Close()
				return err
			}
			return nil
		})
		if err != nil {
			f.Close()
			return err
		}
	}
	if err := w.Close(); err != nil {
		f.Close()
		return err
	}
	if err := z.Close(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

var githubRemote = regexp.MustCompile(`github\.com[:/]([^/]+/[^/]+?)(?:\.git)?/?$`)

// gitHubRepo returns the GitHub repository of the git checkout dir is in,
// as owner/name, from its remote origin.
func gitHubRepo(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "remote", "get-url", "origin").Output()
	if err != nil {
		return "", errors.New("the module is in no git checkout with a remote origin: pass -repo owner/name")
	}
	m := githubRemote.FindStringSubmatch(strings.TrimSpace(string(out)))
	if m == nil {
		return "", fmt.Errorf("the remote origin %s is not on GitHub: pass -repo owner/name", strings.TrimSpace(string(out)))
	}
	return m[1], nil
}

func runNativePrebuild(args []string) error {
	fs := newFlags("native prebuild", "[-repo owner/name] [-no-upload] [module dir]", `Builds the module's implementations for release, uploads them as release
assets of the app's repository (the GitHub repository of the module's git
checkout, or -repo), with the GitHub CLI, and records them in the module's
mygo-plugin.json, so that machines without the platforms' tools, such as a
Linux runner, build the app with them. Commit mygo-plugin.json. The builds
must change with the sources: mygo build uses them only while they match,
and builds the module itself, or fails, once they do not.`)
	repo := fs.String("repo", "", "the GitHub repository to upload to, owner/name (default: the git checkout's origin)")
	noUpload := fs.Bool("no-upload", false, "build and pack the archive, and print its path, but upload nothing and leave mygo-plugin.json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dir, err := filepath.Abs(dirArg(fs.Args()))
	if err != nil {
		return err
	}
	if _, err := nativegen.Generate(dir); err != nil {
		return err
	}
	m, err := nativegen.Load(dir)
	if err != nil {
		return err
	}
	if !nativegen.HasPlatform(dir, "apple") {
		return fmt.Errorf("%s has no platform directory to prebuild", dir)
	}
	if runtime.GOOS != "darwin" {
		return errors.New("prebuilding apple/ needs macOS, with Xcode or its Command Line Tools")
	}
	b, err := buildSwiftModule(dir, "universal", true)
	if err != nil {
		return err
	}
	hash, err := swiftSourceHash(dir)
	if err != nil {
		return err
	}
	work, err := os.MkdirTemp("", "mygo-prebuild-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	name := fmt.Sprintf("%s-%s-darwin-universal.tar.gz", m.Package, hash)
	archive := filepath.Join(work, name)
	if *noUpload {
		archive = filepath.Join(dir, "apple", ".build", "mygo", name)
	}
	if err := tarDir(archive, append([]string{b.dylib}, b.frameworks...)); err != nil {
		return err
	}
	if *noUpload {
		logf("Packed %s", cyan(archive))
		return nil
	}
	sum, err := download.FileSHA256(archive)
	if err != nil {
		return err
	}
	if *repo == "" {
		if *repo, err = gitHubRepo(dir); err != nil {
			return err
		}
	}
	gh, err := exec.LookPath("gh")
	if err != nil {
		return errors.New("mygo native prebuild uploads with the GitHub CLI (https://cli.github.com), signed in with gh auth login or GH_TOKEN; -no-upload only packs")
	}
	tag := "native-" + m.Package + "-" + hash
	if exec.Command(gh, "release", "view", tag, "--repo", *repo).Run() != nil {
		t := con.start("Creating the release " + tag + " of " + *repo)
		notes := fmt.Sprintf("The prebuilt implementation of the native module %s, for mygo build on machines without Xcode.", m.Package)
		out, err := exec.Command(gh, "release", "create", tag, "--repo", *repo, "--title", tag, "--notes", notes, "--latest=false").CombinedOutput()
		if err != nil {
			return t.end(fmt.Errorf("gh release create: %v\n%s", err, out), "")
		}
		t.done("Created the release " + tag + " of " + *repo)
	}
	t := con.start("Uploading " + name)
	if out, err := exec.Command(gh, "release", "upload", tag, "--repo", *repo, "--clobber", archive).CombinedOutput(); err != nil {
		return t.end(fmt.Errorf("gh release upload: %v\n%s", err, out), "")
	}
	t.done("Uploaded " + name)
	p := &prebuilt{Source: hash, File: nativeFile{Name: name, URL: fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", *repo, tag, name), SHA256: sum}}
	if err := writePrebuilt(dir, "apple", p); err != nil {
		return err
	}
	logf("Recorded it in %s: commit it", cyan(filepath.Join(dir, pluginFile)))
	return nil
}

// writePrebuilt records p in the module's mygo-plugin.json, keeping what
// else the file says.
func writePrebuilt(dir, platform string, p *prebuilt) error {
	path := filepath.Join(dir, pluginFile)
	doc := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	native, _ := doc["native"].(map[string]any)
	if native == nil {
		native = map[string]any{}
	}
	native[platform] = p
	doc["native"] = native
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
