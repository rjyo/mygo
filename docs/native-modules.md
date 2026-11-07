# Native modules

A native module is a Go interface implemented in a platform's own
language. You declare the interface in Go, implement what MyGo generates
from it for each platform you support, and call it from Go like any other
Go value: with `context.Context`, errors, iterators and callbacks, checked
by the Go compiler on one side and the platform's compiler on the other.

Use one for what a platform offers only in its own language: its
frameworks, its packages and libraries, and its views, shown in native UI.

A module keeps an implementation per platform, each in a directory of its
own, built with that platform's tools:

| Directory | Language | Runs on |
|---|---|---|
| `apple/` | Swift, as a Swift package | macOS |

`apple/` is a Swift package for Apple's platforms, which may target more of
them as MyGo runs there; other platforms will get directories and languages
of their own, behind the same Go interface. The Go side says what a module
does, and each directory how. On a platform the module has no
implementation for, its constructors return `errors.ErrUnsupported`, so the
app compiles everywhere and decides what to do without it.

The Go side:

```go
package language

// Analyzer reads natural language text.
//
//mygo:native
type Analyzer interface {
	// DetectLanguage returns the BCP 47 code of the text's language, such
	// as "en" or "zh-Hans".
	DetectLanguage(text string) (string, error)
	// Tokens yields the words, sentences or paragraphs of the text.
	Tokens(ctx context.Context, text string, unit Unit) iter.Seq2[Token, error]
}

type Unit string

const (
	Word      Unit = "word"
	Sentence  Unit = "sentence"
	Paragraph Unit = "paragraph"
)

// Token is a range of the text, in UTF-8 bytes.
type Token struct {
	Start, End int
	Text       string
}
```

Its implementation in `apple/`, with NaturalLanguage:

```swift
import NaturalLanguage

func makeAnalyzer() -> any Analyzer { NLAnalyzer() }

final class NLAnalyzer: Analyzer {
    func detectLanguage(text: String) throws -> String {
        guard let lang = NLLanguageRecognizer.dominantLanguage(for: text) else {
            throw LanguageError.undetermined
        }
        return lang.rawValue
    }

    func tokens(text: String, unit: Unit) -> AsyncThrowingStream<Token, Error> {
        AsyncThrowingStream { continuation in
            let tokenizer = NLTokenizer(unit: NLTokenUnit(unit))
            tokenizer.string = text
            tokenizer.enumerateTokens(in: text.startIndex..<text.endIndex) { range, _ in
                continuation.yield(Token(range, in: text))
                return !Task.isCancelled
            }
            continuation.finish()
        }
    }
}
```

And the app, calling it:

```go
a, err := language.NewAnalyzer()
if err != nil {
	return err // errors.ErrUnsupported where the module has no implementation
}
lang, err := a.DetectLanguage("Bonjour tout le monde")
for tok, err := range a.Tokens(ctx, text, language.Sentence) {
	if err != nil {
		return err
	}
	fmt.Println(tok.Start, tok.Text)
}
```

The interface is the only declaration. `mygo generate` writes the rest:
`NewAnalyzer` in Go, and for each platform the declarations its
implementation must satisfy, here the Swift protocol `Analyzer`, the struct
`Token`, the enums `Unit` and `LanguageError`, with the glue between them.
Change the interface and every implementation stops compiling until it
matches; forget a method and the platform's compiler names it.

## Create a module

```sh
mygo native new [-platform <platforms>] <module name>
```

makes a module named `<module name>` in `modules/<module name>` of the app,
or at the path you give instead of a name, such as `internal/<module name>`.
`-platform` names the directories to implement it in, separated by commas:
`apple` by default. `-platform none` makes the Go side alone, to agree on
the interface first and implement it later.

```
modules/<module name>/
├── <module name>.go              # the interface: yours
├── <module name>_native.go       # generated
├── AGENTS.md                     # how to write and check the module, for agents
└── apple/
    ├── Package.swift             # the Swift package: yours, for its dependencies
    ├── Info.plist                # optional: keys added to the app's Info.plist
    ├── Entitlements.plist        # optional: entitlements added to the app's
    └── Sources/<Module>/
        ├── <Module>.swift        # the implementation: yours
        └── <Module>.mygo.swift   # generated
```

The module's name is its Go package's, so it is a valid package name, such
as `language` or `media`. `<Module>` is the name in upper camel case, the
name of the Swift module: `Language` for `language`.

To implement an existing module in another directory:

```sh
mygo native add <platform> [module dir]
```

makes the directory in the module, the current directory by default, with
a factory for every entry point and a class whose every method says it is
unsupported, so the module builds at once and the methods are implemented
one at a time.

A module is an ordinary Go package. Import it from the app, or from any Go
module: `mygo dev` and `mygo build` find every package of the app with an
implementation among its dependencies (`go list -deps`), build the one for
the platform they build for and put it into the app. There is nothing to
register.

Commit the generated files. `<module name>_native.go` lets the app compile,
vet and test on any machine, with or without the platform's tools, and
`<Module>.mygo.swift`, which holds the protocols, structs and enums of the
Go side and the `@_cdecl` functions Go calls, lets SourceKit-LSP and Xcode
complete and check the implementation; never edit them. They say which
interface they came from, and `mygo build` refuses to build when one is
older than the interface.

## Declare the interface

The Go side of a module is its contract: interfaces, the types they use and
the errors they return, with nothing of any platform in it.

**Entry points.** A comment `//mygo:native` before an interface makes it
an entry point of the module, which the package gets a constructor for:
`NewT() (T, error)` for an interface `T`. The directive takes no options.
Make the value once, as you would open a file, and keep it.

**Factories.** Each implementation chooses what implements an entry point
with a factory named after it, which the generated code calls and the
compiler requires: in Swift, `makeAnalyzer` for `Analyzer`. Without it the
package does not compile, and Swift says
`cannot find 'makeAnalyzer' in scope`.

```swift
func makeAnalyzer() throws -> any Analyzer {
    guard #available(macOS 14, *) else { throw NativeError.unsupported }
    return NLAnalyzer()
}
```

The factory may throw, and the constructor returns its error.
`NativeError.unsupported` reaches Go as `errors.ErrUnsupported`, for what
the system lacks, such as a framework of a newer version. The systems an
implementation runs on are those its `Package.swift` lists in `platforms`:
on others, and on platforms with no implementation, the constructor returns
`errors.ErrUnsupported` without asking.

**Other interfaces.** The interfaces that the methods of an entry point
take or return are part of the module too, and generated with it, without
a constructor. They need no directive. Who implements one follows from
where it appears: an interface that a method of the module returns, such
as a loaded model, is implemented by the platform (see
[objects](#objects)); one that methods only take, such as a delegate, is
implemented by Go (see [interfaces Go implements](#interfaces-go-implements)).
`mygo generate` stops with an error at an interface that is both.

**Capabilities.** What an implementation is, beyond its methods, is said in
Go's own way, by embedding:

- `native.View`: the implementation is a view of the platform, shown in
  native UI with `ui.NativeView` (see [views](#views)).
- `io.Closer`, or a `Close() error` method: the object is released when Go
  closes it (see [objects](#objects)).

**Threads.** A comment `//mygo:mainthread` before an interface runs all its
methods on the main thread; before a method, that method. In Swift they are
`@MainActor`. Views run on the main thread without it.

The Go doc comments of the interfaces, their methods and their types become
the documentation comments of the generated code, so every implementation
reads the same contract.

### Methods

A method may:

- take any number of the [types](#types) below, after an optional
  `context.Context`, and function parameters, which are
  [callbacks](#callbacks);
- return nothing, a value, an error, a value and an error, an
  `iter.Seq[T]` or an `iter.Seq2[T, error]`, or an `io.Closer` that stops
  what the method started (see [subscriptions](#subscriptions)).

| Go | Swift |
|---|---|
| `M(a int, b string)` | `func m(a: Int, b: String)` |
| `M(...) error` | `func m(...) throws` |
| `M(...) (T, error)` | `func m(...) throws -> T` |
| `M(ctx context.Context, ...) (T, error)` | `func m(...) async throws -> T` |
| `M(ctx context.Context, ...) iter.Seq2[T, error]` | `func m(...) -> AsyncThrowingStream<T, Error>` |
| `M(ctx context.Context, ...) iter.Seq[T]` | `func m(...) -> AsyncStream<T>` |
| `M(..., onChange func(float64))` | `func m(..., onChange: @escaping @Sendable (Double) -> Void)` |
| `M(..., allow func(url string) bool)` | `func m(..., allow: @escaping @Sendable (String) -> Bool)` |
| `M(..., check func(string) error)` | `func m(..., check: @escaping @Sendable (String) throws -> Void)` |
| `M(...) io.Closer` | `func m(...) -> any Closer` |
| `//mygo:mainthread` | `@MainActor` |

Method names become lower camel case (`DetectLanguage` → `detectLanguage`,
`URLFor` → `urlFor`) and parameters keep their Go names as Swift labels.

**Contexts.** A method that takes a `context.Context` first is `async` in
Swift and runs in a `Task`. Canceling the context cancels the task, so
`Task.isCancelled` and `try Task.checkCancellation()` see it, and a
cancellable await, such as `Task.sleep`, throws `CancellationError`, which
Go gets as `ctx.Err()`. A deadline of the context cancels the task when it
passes. The context is not a parameter of the implementation: cancellation
is all it carries there. A method without a context is synchronous and
runs on the calling goroutine's thread, or on the main thread with
`//mygo:mainthread`: keep those quick, as the view and handlers call them
on the main thread.

**Streams.** A method that returns an iterator yields what the Swift stream
yields. The stream starts when the loop starts and stops when the loop
breaks, the context is canceled or the stream finishes or throws. When Go
stops first, the stream's task is canceled and its `onTermination` runs:
stop there what the stream started.

### Callbacks

A function parameter becomes a `@Sendable` closure the implementation may
keep and call from any thread, any number of times. It stays valid as long
as what it was given to: the subscription the method returns, until it is
closed, or else the object the method belongs to.

- A callback with no results is a notification: the closure returns at
  once, and Go runs the function later on the main thread, as it runs
  every handler, in the order the implementation called it.
- A callback with results is a question: the closure waits for the answer,
  and Go runs the function on the thread that calls it, at once. Answer
  quickly when the system asks on its main thread, as it does for most
  delegates, and never wait there for work that needs the main thread.
  A callback's `error` result is thrown in Swift.

### Types

Parameters, results and the fields of structs use these types:

| Go | Swift |
|---|---|
| `bool`, `int`, `int64`, `uint8` … | `Bool`, `Int`, `Int64`, `UInt8` … |
| `float32`, `float64` | `Float`, `Double` |
| `string` | `String` |
| `[]byte` | `Data` |
| `time.Time`, `time.Duration` | `Date`, `Duration` |
| `[]T`, `map[string]T` | `[T]`, `[String: T]` |
| `*T` | `T?` |
| a struct of the package | a `struct` with the same fields, `Sendable`, and `Hashable` when its fields are |
| `type Unit string` (or an integer type) with constants | `enum Unit: String`, a case per constant |
| an interface of the module | the generated protocol, as `any P` |
| an interface or type of another module | that module's, from its Swift module (see [modules that use other modules](#modules-that-use-other-modules)) |
| `*mygo.Window` | `NSWindow` (see [windows and anchors](#windows-and-anchors)) |
| `ui.Rect` | `CGRect`, in the coordinates of the window's content view |
| `native.View` | `NSView` |
| `*native.Buffer` | `NativeBuffer` (see [buffers](#buffers)) |
| `*native.Texture` | `NativeTexture` (see [textures](#textures)) |

Exported fields become lower camel case properties; unexported ones are
left out. Structs and slices are copied across, so neither side sees the
other's later changes. A `[]byte` argument is lent to Swift for the call
without a copy: copy it, with `Data(data)`, to keep it.

`mygo generate` stops with an error, at the line of the declaration, for
anything else: channels, `any`, maps with other keys, generic types and
types of other packages, except those above. Give such a value a struct of
the package.

Swift's own types that Go has no equivalent of, such as enums with
associated values, generic types and overloaded methods, stay inside the
implementation: convert them there to a struct, or to a struct with a field
per case and a `type Kind string` saying which one is set.

### Errors

Declare the errors an implementation may return as variables of the
package, made with `native.NewError` and a code:

```go
var (
	ErrUndetermined = native.NewError("undetermined")
	ErrNoEmbedding  = native.NewError("no_embedding")
)
```

Swift gets them as an enum named after the module, `<Module>Error`, with a
case per error in lower camel case (`ErrNoEmbedding` → `.noEmbedding`):

```swift
guard let lang = NLLanguageRecognizer.dominantLanguage(for: text) else {
    throw LanguageError.undetermined
}
```

and Go compares them with `errors.Is(err, language.ErrUndetermined)`. Any
other error Swift throws reaches Go as a `*native.Error` with no code, and
the error's domain or type (`"NSCocoaErrorDomain"`, `"ParseError.badHeader"`),
code and `localizedDescription`. `NativeError.unsupported` reaches Go as
`errors.ErrUnsupported`.

A crash in the implementation, such as `fatalError`, a force unwrap of
`nil` or an index out of range, ends the app, as a panic Go does not
recover would.

## Objects

A method may return another interface of the package, which needs no
directive: the implementation returns any object conforming to it, and Go
gets a value of the interface that calls it. Use this for what has a
lifetime: a loaded model, a session, a connection.

```go
//mygo:native
type Analyzer interface {
	// ...
	// Embedding loads the word embedding of a language.
	Embedding(ctx context.Context, language string) (Embedding, error)
}

// Embedding places words of a language by meaning.
type Embedding interface {
	io.Closer
	Distance(a, b string) float64
	Neighbors(word string, n int) []string
}
```

```swift
extension NLAnalyzer {
    func embedding(language: String) async throws -> any Embedding {
        guard let e = NLEmbedding.wordEmbedding(for: NLLanguage(language)) else {
            throw LanguageError.noEmbedding
        }
        return WordEmbedding(e)
    }
}

final class WordEmbedding: Embedding {
    let e: NLEmbedding
    init(_ e: NLEmbedding) { self.e = e }
    func distance(a: String, b: String) -> Double { e.distance(between: a, and: b) }
    func neighbors(word: String, n: Int) -> [String] { e.neighbors(for: word, maximumCount: n).map(\.0) }
    func close() throws {}
}
```

```go
e, err := a.Embedding(ctx, "en")
if errors.Is(err, language.ErrNoEmbedding) {
	// ...
}
defer e.Close()
fmt.Println(e.Neighbors("coffee", 5))
```

An implementation is a class: the generated protocols are `AnyObject`.
The Go value holds a strong reference to the Swift object. An interface
that embeds `io.Closer`, or has a `Close() error` method, is closed
explicitly: Go calls the object's `close()`, then releases it, and every
later call returns `native.ErrClosed`. Otherwise, and as a safety net for
objects never closed, the object is released after the Go value becomes
unreachable.

An object the module returns can be passed back to it, or to another
module: the implementation receives the same instance, not a copy.

### Subscriptions

A method that starts something lasting, such as observing a value or
listening for a system event, returns an `io.Closer` that stops it:

```go
type Analyzer interface {
	// ...
	// OnLanguagesChange calls f when the user changes the preferred
	// languages, until the subscription is closed.
	OnLanguagesChange(f func(languages []string)) io.Closer
}
```

In Swift it returns a `Closer`, which `NativeCloser` makes from a closure
that runs once:

```swift
func onLanguagesChange(f: @escaping @Sendable ([String]) -> Void) -> any Closer {
    let token = NotificationCenter.default.addObserver(
        forName: NSLocale.currentLocaleDidChangeNotification, object: nil, queue: nil
    ) { _ in f(Locale.preferredLanguages) }
    return NativeCloser { NotificationCenter.default.removeObserver(token) }
}
```

```go
sub := a.OnLanguagesChange(func(langs []string) { ... })
defer sub.Close()
```

Closing it runs the closer once and releases the callback; the
implementation calling it afterwards does nothing.

## Interfaces Go implements

An interface that methods of the module only take, never return, is
implemented by Go: the platform asks it, as AppKit asks a delegate or a
data source. Pass any Go value that implements it.

```go
// Ranker decides the order of suggestions.
type Ranker interface {
	// Score is higher for better suggestions.
	Score(word string) float64
	// Explain describes a suggestion, which may take a request.
	Explain(ctx context.Context, word string) (string, error)
}

type Analyzer interface {
	// ...
	// Suggest yields the neighbors of word in the embedding, best first by
	// the ranker's scores.
	Suggest(ctx context.Context, word string, r Ranker) iter.Seq2[string, error]
}
```

It becomes a protocol too, and Swift receives an object conforming to it
that calls Go:

```swift
func suggest(word: String, r: any Ranker) -> AsyncThrowingStream<String, Error> {
    AsyncThrowingStream { continuation in
        let task = Task {
            let best = candidates(of: word).sorted { r.score(word: $0) > r.score(word: $1) }
            for w in best { continuation.yield(w) }
            let why = try await r.explain(word: best[0]) // Go's method with a context
            // ...
            continuation.finish()
        }
        continuation.onTermination = { _ in task.cancel() }
    }
}
```

Its methods follow the rules of [callbacks with results](#callbacks): one
without a context runs on the calling thread, at once, and returns its
results; one with a context is `async throws` in Swift, runs on a goroutine
of its own, and canceling the Swift task cancels its context. A Go error is
thrown as a `NativeError`, or as the module's error enum when it is one of
the package's.

Swift may keep the object, and Go keeps the value alive as long as Swift
does, until it releases the object.

## Views

An interface that embeds `native.View` is implemented by a view of the
platform and shown with `ui.NativeView`, which takes a `native.View`, as
`ui.WebView` shows a web view: make it once, outside the view, and show it
from the view.

```go
// Player plays a video.
//
//mygo:native
type Player interface {
	native.View
	Load(url string)
	Play()
	Pause()
	Seek(ctx context.Context, to time.Duration) error
	OnTime(f func(time.Duration)) io.Closer
}
```

In Swift the view is an `NSView` subclass conforming to the protocol, which
the generated code declares `@MainActor` and constrained to `NSView`:

```swift
import AVKit

@MainActor func makePlayer() -> any Player { PlayerView() }

final class PlayerView: AVPlayerView, Player {
    func load(url: String) { player = URL(string: url).map(AVPlayer.init(url:)) }
    func play() { player?.play() }
    func pause() { player?.pause() }
    func seek(to: Duration) async throws { await player?.seek(to: CMTime(to)) }
    func onTime(f: @escaping @Sendable (Duration) -> Void) -> any Closer {
        let token = player?.addPeriodicTimeObserver(forInterval: CMTime(seconds: 0.25, preferredTimescale: 600), queue: .main) {
            f(Duration($0))
        }
        return NativeCloser { [weak self] in token.map { self?.player?.removeTimeObserver($0) } }
    }
}
```

```go
type app struct {
	win     *mygo.Window
	player  media.Player
	timeSub io.Closer
	now     time.Duration
}

func (a *app) view(c *ui.Context) {
	// What the view paints after the native view, its children first,
	// shows over it.
	ui.NativeView(c, a.player).Fill().Children(func() {
		ui.Text(c, captionAt(a.now)).Absolute().Left(24).Bottom(24)
	})
}

// After making the player:
a.timeSub = a.player.OnTime(func(t time.Duration) { a.now = t; a.win.Invalidate() })
```

A native view is a box of native UI like a web view: it has no size of its
own, takes its size from layout, and is cut to the element's `Radius`.
What the view paints after it shows over it, so captions, controls,
popovers and dialogs draw on top of the video. MyGo places the `NSView`
under the frame of native UI, sizes and clips it: don't move, resize or
remove it yourself. It receives the mouse, and the keyboard when it has
the focus, as the first responder, and joins the window's accessibility
tree, as any `NSView`. A view the window's view does not build hides and
keeps its state.

A view runs on the main thread, as AppKit wants, with no
`//mygo:mainthread`. Make it once the app runs, in `App.WhenReady` or
later; it is closed and released there too. Its callbacks run on the main thread too; call the
window's `Invalidate` from them to show what changed.

### Windows and anchors

What the system shows of its own, such as a share menu, a Quick Look
panel, a popover or a sheet, needs the window it belongs to and where in it
to point. Take a `*mygo.Window`, and a `ui.Rect` in the window, such as an
element's bounds:

```go
//mygo:native
//mygo:mainthread
type Sharing interface {
	// Share shows the system's share menu for the files, from rect of win.
	Share(win *mygo.Window, rect ui.Rect, paths []string)
}
```

Swift receives the `NSWindow`, and the rectangle as a `CGRect` in the
coordinates of its content view, flipped from native UI's top-left origin,
ready for AppKit's `relativeTo:of:` arguments:

```swift
@MainActor final class SystemSharing: Sharing {
    func share(win: NSWindow, rect: CGRect, paths: [String]) {
        let picker = NSSharingServicePicker(items: paths.map(URL.init(fileURLWithPath:)))
        picker.show(relativeTo: rect, of: win.contentView!, preferredEdge: .minY)
    }
}
```

```go
btn := ui.Button(c, "Share")
if btn.Clicked() {
	a.sharing.Share(a.win, btn.Bounds(), []string{path})
}
```

A `native.View` parameter, such as a view of another module, arrives as
its `NSView`, to anchor to it instead.

## Modules that use other modules

A module may use the types and interfaces of another native module: import
its Go package and use them in the interface, as any Go type.

```go
package player

import "example.com/app/modules/media"

//mygo:native
type Player interface {
	native.View
	// Load plays an asset that the media module opened.
	Load(asset media.Asset)
}
```

The generated Swift imports the other module's Swift module, and the
implementation receives the object that module returned, an `AVAsset` in
its own class, not a copy.

Go's imports are the dependencies between modules: there is nothing to add
to `Package.swift`. The generated declarations of a module are `public`
for others to see; its Go package loads the modules it uses first.
`mygo dev`, `mygo build` and `mygo native check` build the modules a
module uses before it, each from its own Swift package, and build it
with their Swift modules and libraries, which it links: the app gets each
module once, however many modules use it, next to the others in
`Contents/Frameworks`.

`mygo generate` and `mygo dev` also write the app's modules as the targets
of one Swift package, `.mygo/native`, each depending on the modules its
Go package imports and on the packages its own `Package.swift` declares,
its sources a link to the module's. Open that package in Xcode, or point
SourceKit-LSP at it, to edit a module that uses others with completion;
it builds as the app does, so two modules that need incompatible versions
of one Swift package stop it as SwiftPM resolves them.

## Large data

Values cross as copies, which suits arguments and results but not a stream
of video frames or audio samples. Two types cross without copying.

### Buffers

A `*native.Buffer` is memory that Go and the implementation both use:
`native.NewBuffer(n)` makes one, and `Bytes()` is its contents in Go.
Pass it to have the implementation fill it, as `io.Reader` fills a slice,
or yield buffers from a stream:

```go
type Recorder interface {
	// Samples yields the microphone's PCM samples, 16-bit, a buffer at a
	// time. A buffer is valid until the loop asks for the next one.
	Samples(ctx context.Context) iter.Seq2[*native.Buffer, error]
	// Read fills buf with what was recorded since the last read.
	Read(buf *native.Buffer) (int, error)
}
```

In Swift it is a `NativeBuffer`, whose `bytes` is an
`UnsafeMutableRawBufferPointer` over it, valid as long as the buffer:

```swift
func read(buf: NativeBuffer) throws -> Int {
    ring.read(into: buf.bytes)
}
```

A buffer a stream yields is lent until the loop continues: copy what you
keep, as with `bufio.Scanner.Bytes`. Swift yields buffers from a
`NativeBufferPool`, which reuses each one once Go's loop moves on, so a
stream of buffers allocates none once it runs.

### Textures

A `*native.Texture` is an image on the GPU that the implementation draws
into and native UI shows, with no copy through the CPU: it is a
`ui.ImageSource`, as a `*ui.Bitmap` is.

```go
type Frames interface {
	// Attach draws the video's frames into tex as they play.
	Attach(tex *native.Texture) io.Closer
}
```

In Swift it is a `NativeTexture`, which takes frames with `present`: an
`IOSurface`, or a `CVPixelBuffer` backed by one. `pixelBufferAttributes`
makes such buffers in the pixel format native UI draws without converting,
and `displayLink` calls back on each refresh of the window's display:

```swift
func attach(tex: NativeTexture) -> any Closer {
    let output = AVPlayerItemVideoOutput(pixelBufferAttributes: tex.pixelBufferAttributes)
    item.add(output)
    let link = tex.displayLink { time in
        if let pb = output.copyPixelBuffer(forItemTime: output.itemTime(forHostTime: time), itemTimeForDisplay: nil) {
            tex.present(pb)
        }
    }
    return NativeCloser { link.invalidate(); self.item.remove(output) }
}
```

```go
tex := native.NewTexture()
sub := a.frames.Attach(tex)

func (a *app) view(c *ui.Context) {
	ui.Image(c, tex).Fill().Radius(12)
}
```

The window shows what Swift presents in its next frame, without the app
calling `Invalidate`. Native UI holds the surface it shows, and those
frames on the GPU may still read, in use (`IOSurfaceIncrementUseCount`):
a pool of pixel buffers gives a surface to the implementation again only
once no frame shows it, so draw each frame into a buffer of a pool rather
than into one again and again. `displayLink`'s callback gets the host
time the frame shows at, in seconds, as `CACurrentMediaTime` counts, and
`invalidate()` stops it. Unlike a [view](#views), a texture is drawn by native
UI itself: it can be covered, clipped, scaled, rotated and faded like any
image, and drawn by a [shader](ui/shaders.md).

## Dependencies

`apple/Package.swift` is the module's Swift package, and yours to edit:
add Swift packages and binary targets to its `dependencies`, link system
frameworks with `linkerSettings`, and list the systems it runs on in
`platforms`.

```swift
// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "Language",
    platforms: [.macOS(.v12)],
    products: [.library(name: "Language", type: .dynamic, targets: ["Language"])],
    dependencies: [
        .package(url: "https://github.com/apple/swift-collections", from: "1.1.0"),
    ],
    targets: [
        .target(name: "Language", dependencies: [
            .product(name: "Collections", package: "swift-collections"),
        ]),
    ]
)
```

Keep the `<Module>` product dynamic, named after the module, with the
generated file among its sources. The packages `mygo native new` writes
compile in the Swift 5 language mode (`swiftLanguageModes: [.v5]`), which
keeps Swift 6's strict checks of concurrency out of the way of
implementations, whose classes Go calls from its own threads. Dynamic frameworks the package brings,
such as an XCFramework's, go into the app's `Contents/Frameworks` with it,
and `mygo build` signs them all with the app's identity.

## Info.plist and entitlements

Some frameworks need a usage description in the app's `Info.plist`, such as
Speech, the microphone and the camera, or an entitlement, such as a shared
keychain or App Groups. Put them in the module's `apple/Info.plist` and
`apple/Entitlements.plist`, which `mygo build` and `mygo dev` merge into
the app's:

```xml
<!-- apple/Info.plist -->
<plist version="1.0">
<dict>
    <key>NSSpeechRecognitionUsageDescription</key>
    <string>Recognize speech in your recordings.</string>
</dict>
</plist>
```

Dictionaries merge by key, arrays by their values, and the app's own
`macos.infoPlist` and `macos.entitlements` ([configuration](configuration.md))
come last and win, so an app words a module's usage descriptions itself.
Two modules that set one key to different values stop the build, with both
named, unless the app sets it. An entitlement that a provisioning profile
must allow, such as App Groups, needs one that does
([provisioning profiles](distribution.md#provisioning-profiles)).

## Build, run and test

Building a module needs the platform's tools: for `apple/`, Xcode or its
Command Line Tools, with Swift 6.

`mygo dev` regenerates the module when its interface changes, rebuilds the
implementation in debug mode when one of its files changes, and restarts
the app, which reopens its windows where they were. `mygo build` builds the
implementation of every module in release mode, universal for arm64 and
x86_64, and signs it with the frameworks it brings, before it signs and
notarizes the app ([distribution](distribution.md)).

To build an app on a machine without the platform's tools, such as a Linux
runner of GitHub Actions, prebuild the modules where the tools are:

```sh
mygo native prebuild [-repo owner/name] [-no-upload] [module dir]
```

builds the module's implementations, uploads them as release assets of the
app's repository and records them in `mygo-plugin.json` next to the
module, as the [terminal](plugins/terminal.md) plugin ships libghostty-vt:
every machine then downloads those builds instead of compiling them, and
they must change with the sources, which `mygo build` checks. It uploads
with the [GitHub CLI](https://cli.github.com), to the release
`native-<module>-<hash>` of the repository of the module's git checkout,
or of `-repo`, where `<hash>` is that of the Swift package's sources;
`-no-upload` only builds the archive. Commit `mygo-plugin.json`.
`mygo build` takes the prebuilt implementation while its hash is the
sources', and `mygo dev` where Swift is missing; once the sources change,
a machine with Swift builds them, with a warning, and one without stops
and says to prebuild again.

`go test` runs the implementation of the platform it runs on once
`mygo generate` has built it; it says to run it when the build is missing
or older than the sources. Code that takes the interface, rather than the
constructor's result, is tested anywhere with a fake of it:

```go
type fakeAnalyzer struct{ lang string }

func (f fakeAnalyzer) DetectLanguage(text string) (string, error) { return f.lang, nil }

func (f fakeAnalyzer) Tokens(ctx context.Context, text string, unit language.Unit) iter.Seq2[language.Token, error] {
	return func(yield func(language.Token, error) bool) {
		yield(language.Token{Start: 0, End: len(text), Text: text}, nil)
	}
}
```

To stop at a breakpoint in Swift, attach Xcode's debugger to the app that
`mygo dev` runs (Debug ▸ Attach to Process), or `lldb -p <pid>`. What Swift
logs with `Logger` or `print` shows in `mygo dev`'s output, beside Go's.

## Check and call a module

Two commands check a module without opening the app, for you and for
agents:

```sh
mygo native check [module dir]
```

generates the module, builds the implementation of the platform it runs
on and vets the Go package, then lists every problem as
`file:line: message`, with what to change, such as `language.go:14: Tokens
returns chan Token, which an implementation cannot return: return
iter.Seq2[Token, error]`. It exits with status 1 when there are problems.

```sh
mygo native call [-timeout <duration>] [module dir] <Interface>.<Method> [<arguments>]
```

makes the implementation of the interface, an entry point, calls one of its
methods with `<arguments>`, a JSON object of the method's parameters by
their Go names, and prints the result as JSON, a line per value of a
stream:

```sh
mygo native call modules/language Analyzer.DetectLanguage '{"text": "Bonjour"}'
```

It takes no `ctx` argument: it makes a context, which `-timeout` limits.
Callbacks print their calls.

Every command of `mygo native` is in [the CLI's reference](cli.md#mygo-native).
The `AGENTS.md` that `mygo native new` writes into the module says this, and
`mygo install-skills` installs a skill that teaches agents to write native
modules.

## Use it from web pages

A native module is a Go value, so bind it to give its methods to web pages
as well, through the [generated client](bindings.md):

```go
a, _ := language.NewAnalyzer()
mygo.Bind(&Text{a}) // a service whose methods call a
```

## How it works

The generated Go calls the implementation's library through purego, with
no cgo, as MyGo calls the system: one C entry point per method, which the
generated Swift exports with `@_cdecl`, and one Go callback shared by every
module, for results of asynchronous methods, the values of streams,
callbacks, and calls into interfaces Go implements. Values cross as JSON,
the encoding of [bound services](bindings.md), except numbers and `[]byte`,
which cross as they are, buffers, which are memory both sides map, and
textures, which are `IOSurface`s shared with the GPU. An app that has no
native module links none of this.
