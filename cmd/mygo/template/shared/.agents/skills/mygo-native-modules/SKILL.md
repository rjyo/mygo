---
name: mygo-native-modules
description: Write and change native modules of MyGo apps, Go interfaces implemented in a platform's own language (Swift on macOS) for its frameworks, packages and views. Use when a MyGo app needs an API only a platform's language offers, or when a package has a file ending in _native.go or a directory apple/ with a Package.swift.
---

# MyGo native modules

A native module is a Go package whose interfaces Swift implements. Read
`docs/native-modules.md` of the app's MyGo (`go list -m -f '{{.Dir}}'
github.com/egoist/mygo`) for the whole contract; this is the loop.

## Make one

```sh
mygo native new <name>            # modules/<name>: the Go interface, apple/ in Swift
mygo native add apple [dir]       # implement an existing module on macOS
```

## The loop

1. Change the Go interface in `<name>.go`: the contract. `//mygo:native`
   marks an entry point (the package gets `New<T>()`), `//mygo:mainthread`
   runs methods on the main thread, embedding `native.View` makes the
   implementation an `NSView` that `ui.NativeView(c, v)` shows, and
   `native.NewError("code")` declares errors Swift throws as
   `<Module>Error.code`.
2. Run `mygo native check [dir]`. It regenerates `<name>_native.go` and
   `apple/Sources/<Module>/<Module>.mygo.swift` (never edit those),
   builds the Swift package and vets Go, and prints every problem as
   `file:line: message` with what to write instead. Fix them all; repeat
   until it says the module is fine.
3. Implement in `apple/Sources/<Module>/<Module>.swift`: a factory
   `make<T>()` per entry point and classes conforming to the generated
   protocols. Swift's compiler names any requirement missing.
4. Try a method without the app:
   `mygo native call [dir] <Interface>.<Method> '{"param": value}'`
   prints the result as JSON, a line per value of a stream, and the calls
   of callbacks.
5. Test from Go: `go test` in the module runs the implementation once
   `mygo native check` or `mygo generate` built it. Test code that takes
   the interface with a fake of it.

## What crosses

- Parameters, results and fields: numbers, `bool`, `string`, `[]byte`,
  `time.Time`, `time.Duration`, slices, `map[string]T`, pointers, structs
  and enums (`type T string` with constants) of the package.
- Methods return nothing, a value, an error, both, `iter.Seq[T]` or
  `iter.Seq2[T, error]` (taking a `context.Context` first), or an
  `io.Closer` that stops what they started.
- `context.Context` first makes the Swift method `async`; canceling it
  cancels the task. Function parameters are `@Sendable` closures:
  without results they run later, in order, on Go's main thread.
- An interface the module returns is Swift's (an object, closed with
  `io.Closer`); one it only takes is Go's, which Swift calls.
- `*mygo.Window` and `ui.Rect` anchor system UI; `*native.Buffer` shares
  memory without copying.

Keep Swift's own types (enums with payloads, generics, overloads) inside
the implementation. Put usage descriptions in `apple/Info.plist` and
entitlements in `apple/Entitlements.plist`; packages and frameworks in
`apple/Package.swift`, keeping its product dynamic and named after the
module.
