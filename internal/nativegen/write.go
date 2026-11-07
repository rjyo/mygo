package nativegen

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Platforms are the directories a module may implement it in.
var Platforms = []string{"apple"}

// SwiftDir is the directory of a module's Swift sources.
func SwiftDir(m *Module) string { return filepath.Join(m.Dir, "apple", "Sources", m.Name) }

// HasPlatform reports whether the module in dir implements it in platform.
func HasPlatform(dir, platform string) bool {
	switch platform {
	case "apple":
		_, err := os.Stat(filepath.Join(dir, "apple", "Package.swift"))
		return err == nil
	}
	return false
}

var macOSPlatform = regexp.MustCompile(`\.macOS\(\s*(?:\.v(\d+)(?:_(\d+))?|"([\d.]+)")\s*\)`)

// MinMacOS reads the oldest macOS the Swift package in dir supports from
// the platforms of its Package.swift, or "" when it names none.
func MinMacOS(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "apple", "Package.swift"))
	if err != nil {
		return ""
	}
	m := macOSPlatform.FindSubmatch(data)
	switch {
	case m == nil:
		return ""
	case len(m[3]) > 0:
		return string(m[3])
	case len(m[2]) > 0:
		return string(m[1]) + "." + string(m[2])
	}
	return string(m[1])
}

// Result is what Generate wrote.
type Result struct {
	Module  *Module
	Written []string // the files that changed
}

// Generate loads the module in dir and writes its generated files,
// leaving those that are up to date.
func Generate(dir string) (*Result, error) {
	m, err := Load(dir)
	if err != nil {
		return nil, err
	}
	r := &Result{Module: m}
	goSrc, err := GenerateGo(m, MinMacOS(m.Dir))
	if err != nil {
		return nil, err
	}
	if err := r.write(filepath.Join(m.Dir, GeneratedGo(m.Package)), goSrc); err != nil {
		return nil, err
	}
	if HasPlatform(m.Dir, "apple") {
		if err := r.write(filepath.Join(SwiftDir(m), SwiftFile(m)), GenerateSwift(m)); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *Result) write(path string, data []byte) error {
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	r.Written = append(r.Written, path)
	return nil
}

// Stale returns the generated files of the module in dir that are older
// than its interface, or missing.
func Stale(dir string) ([]string, error) {
	m, err := Load(dir)
	if err != nil {
		return nil, err
	}
	line := SourceLine(m.Hash)
	var stale []string
	check := func(path string) {
		data, err := os.ReadFile(path)
		if err != nil || !bytes.Contains(data, []byte(line)) {
			stale = append(stale, path)
		}
	}
	check(filepath.Join(m.Dir, GeneratedGo(m.Package)))
	if HasPlatform(m.Dir, "apple") {
		check(filepath.Join(SwiftDir(m), SwiftFile(m)))
	}
	return stale, nil
}

// IsModule reports whether dir holds a native module: a Go package whose
// generated file nativegen wrote.
func IsModule(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), "_native.go") && isGenerated(filepath.Join(dir, e.Name())) {
			return true
		}
	}
	return false
}

// Scaffold writes a new module named pkg in dir, implemented in the
// platforms given.
func Scaffold(dir, pkg string, platforms []string) error {
	if !validPackage(pkg) {
		return fmt.Errorf("%q is not a Go package name: use lower case letters and digits, as language or media", pkg)
	}
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return fmt.Errorf("%s is not empty", dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := UpperCamel(pkg)
	files := map[string]string{
		pkg + ".go": fmt.Sprintf(goTemplate, pkg, name),
		"AGENTS.md": strings.ReplaceAll(strings.ReplaceAll(agentsTemplate, "PKG", pkg), "MODULE", name),
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(dir, path), []byte(content), 0o644); err != nil {
			return err
		}
	}
	if _, err := Generate(dir); err != nil {
		return err
	}
	for _, p := range platforms {
		if err := AddPlatform(dir, p); err != nil {
			return err
		}
	}
	return nil
}

func validPackage(s string) bool {
	if s == "" || s == "main" || !(s[0] >= 'a' && s[0] <= 'z') {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
			return false
		}
	}
	return true
}

// AddPlatform adds a platform's directory to the module in dir: a package
// whose every method says it is unsupported, so the module builds at once.
func AddPlatform(dir, platform string) error {
	if platform != "apple" {
		return fmt.Errorf("unknown platform %q: modules are implemented in %s", platform, strings.Join(Platforms, ", "))
	}
	if HasPlatform(dir, platform) {
		return fmt.Errorf("%s already has %s/", dir, platform)
	}
	m, err := Load(dir)
	if err != nil {
		return err
	}
	pkg := filepath.Join(m.Dir, "apple")
	if err := os.MkdirAll(SwiftDir(m), 0o755); err != nil {
		return err
	}
	files := map[string][]byte{
		filepath.Join(pkg, "Package.swift"):         []byte(strings.ReplaceAll(packageTemplate, "MODULE", m.Name)),
		filepath.Join(pkg, ".gitignore"):            []byte("/.build\n/.swiftpm\n"),
		filepath.Join(SwiftDir(m), m.Name+".swift"): stubSwift(m),
	}
	for path, data := range files {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
	}
	_, err = Generate(dir)
	return err
}

// stubSwift is an implementation whose every method says it is
// unsupported, to fill in one at a time.
func stubSwift(m *Module) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "import Foundation\n#if canImport(AppKit)\nimport AppKit\n#endif\n\n// The implementation of package %s on Apple's platforms. Implement the\n// methods one at a time: each says it is unsupported until it does.\n", m.Package)
	g := &swiftGen{m: m, b: &b}
	for _, it := range m.Interfaces {
		if it.ByGo {
			continue
		}
		attr := ""
		if it.MainThread {
			attr = "@MainActor "
		}
		class := "Native" + it.Name
		if it.Entry {
			fmt.Fprintf(&b, "\n%sfunc make%s() throws -> any %s { %s() }\n", attr, it.Name, it.Name, class)
		}
		base := it.Name
		if it.View {
			base = "NSView, " + it.Name
		}
		fmt.Fprintf(&b, "\n%sfinal class %s: %s {\n", attr, class, base)
		for _, me := range it.Methods {
			req := g.requirement(it, me)
			var body string
			switch {
			case me.Close:
				req, body = "func close() throws", ""
			case me.Stream == 1:
				body = "AsyncStream { $0.finish() }"
			case me.Stream == 2:
				body = "AsyncThrowingStream { $0.finish(throwing: NativeError.unsupported) }"
			case me.Closer:
				body = "NativeCloser {}"
			case me.Err:
				body = "throw NativeError.unsupported"
			case me.Result == nil:
				body = ""
			default:
				body = "fatalError(\"" + it.Name + "." + me.Name + " is not implemented\")"
			}
			fmt.Fprintf(&b, "    %s {\n", req)
			if body != "" {
				fmt.Fprintf(&b, "        %s\n", body)
			}
			b.WriteString("    }\n")
		}
		b.WriteString("}\n")
	}
	return b.Bytes()
}

const goTemplate = `// Package %[1]s is a native module: its interfaces are implemented in each
// platform's own language, Swift in apple/ (see AGENTS.md).
package %[1]s

// Greeter greets.
//
//mygo:native
type Greeter interface {
	// Greet returns a greeting for name.
	Greet(name string) (string, error)
}
`

const packageTemplate = `// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "MODULE",
    platforms: [.macOS(.v13)],
    products: [.library(name: "MODULE", type: .dynamic, targets: ["MODULE"])],
    dependencies: [],
    targets: [
        .target(name: "MODULE"),
    ],
    swiftLanguageModes: [.v5]
)
`

const agentsTemplate = "# Native module PKG\n\n" +
	"PKG is a native module of MyGo: Go interfaces implemented in a platform's own\n" +
	"language. Read https://mygo.dev/docs/native-modules (docs/native-modules.md)\n" +
	"before changing it.\n\n" +
	"- `PKG.go` declares the interfaces, the types they use and the errors\n" +
	"  (`native.NewError`): it is the contract. `//mygo:native` marks an entry point,\n" +
	"  which gets a constructor, `NewT`.\n" +
	"- `PKG_native.go` and `apple/Sources/MODULE/MODULE.mygo.swift` are generated:\n" +
	"  never edit them; run `mygo generate` after changing the interfaces.\n" +
	"- `apple/` is the Swift package implementing it on macOS: a factory\n" +
	"  `makeT()` per entry point and classes conforming to the generated protocols.\n" +
	"  Add Swift packages and frameworks in `apple/Package.swift`.\n\n" +
	"Check the module without opening the app:\n\n" +
	"```sh\n" +
	"mygo native check            # generate, build and vet: every problem as file:line\n" +
	"mygo native call Greeter.Greet '{\"name\": \"Ada\"}'   # call a method, print its result\n" +
	"```\n"
