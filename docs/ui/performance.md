# Native UI performance

These measurements cover steady frames rendered on the CPU with `ui.Tester`.
They were taken on October 8, 2026 on Apple M5, macOS arm64, Go 1.27.1, with
`CGO_ENABLED=0 GOMAXPROCS=1`. Each timing is the median of six sequential
300 ms samples after warming the view.

| Fixture | Time per frame | Allocations per frame | Allocated bytes per frame |
|---|---:|---:|---:|
| 1,000 static labels | 397 µs | 0 | 0 |
| Visible rows of a million-row list | 14.5 µs | 20 | 176 |
| Diff view with no changes | 389 µs | 2 | 72 |
| A 50 KB Markdown reply, shown whole | 1.3 ms | 353 | 15,894 |
| The same reply streaming, 16 bytes a frame | 0.8 ms | 350 | 35,946 |

The Markdown rows, `BenchmarkViewSteady` and `BenchmarkViewStream` of
`ui/markdown`, were taken on October 14, 2026, on a busier machine; see
[Markdown](markdown.md#performance) for the parser's own costs.

Element handles validate their owner, arena slot and build generation.
Static label handles do not allocate. Input bindings, action queues and
persistent identity bindings reuse storage once warmed. Composite controls
can allocate callback closures; these fixtures do not cover every widget
or interaction.

The label and list fixtures use the public API. The diff fixture uses the
private renderer API and measures its steady-frame allocation floor.

## What a frame costs

A window draws a frame only when something asks for one: input that may
change what the view shows, `Window.Update` or `Invalidate`, a timer from
`After`, or something that moves. Most frames build the view anew: your
view function runs whole, and MyGo compares, lays out and places every
element it built, changed or not, before it paints what shows. Layout
reuses what did not change, but the work grows with how many elements the
view builds, not with how much changed. These frames were measured on
November 9, 2026, on Apple M5 with `GOMAXPROCS=1` on a busy machine, with
rows of a label and a button inside a `Scroll`:

| What happens | Time per frame |
|---|---:|
| One label of 1,000 rows changes | 2.8 ms |
| One label of 3,000 rows changes | 11 ms |
| The 3,000 rows are asked to draw with nothing changed | 11 ms |
| One label of 3,000 rows in a `List` changes | 0.2 ms |
| The pointer comes over a button of the 3,000 rows | 0.3 ms |
| The pointer goes from row to row of the 3,000 | 0.1 ms |
| A letter is typed into a field above the 3,000 rows | 17 ms |

`List` and `Table` build only the rows in view, so their frames cost what
shows, whatever the length of the collection. Put long collections in
them rather than in a `Scroll` that builds every row.

**Hover.** The pointer coming over an element, or leaving it, asks for a
frame only when something looks at that element's hover. The toolkit's
controls pick their hovered look as they paint, and a scroll container
shows its scroll bars as it paints, so the frame only paints the last
frame's elements again, without running your view. Calling `Hovered` as
your view builds makes the pointer coming over that element, or leaving
it, build the whole view anew. When only a drawing changes with the
hover, read `Hovered` in its `Draw` callback instead:

```go
card := ui.Box(c).Size(200, 120)
card.Draw(func(p *ui.Painter, r ui.Rect) {
	if card.Hovered() {
		p.Fill(r, c.Theme().SurfaceHover, 8)
	}
})
```

**Passes.** A control applies its input as its constructor returns, as Dear
ImGui's do, so the code after it reads the new value. The view then builds
once more in the same frame, so that what was built before the control
shows the new value too: typing a letter builds the view twice.
`MYGO_FRAME_STATS` logs how many passes each frame took (see
[Rendering](rendering.md)).

## Retained memory

A warmed view containing 1,000 labels retained approximately **2.50 MB of
Go heap**, measured over three samples. The measurement warms fonts and
rendering, releases a warm view, forces collection, then measures a second
retained view after ten frames and another collection.

This includes the headless rendering host. It does not measure native window
process memory or GPU resources.

## Measurement scope

Timing depends on the machine, rendering backend, content and activity in
other processes. Small timing differences should be rechecked on several
machines; allocation counts are more stable. These results describe steady
frames and do not compare MyGo with another GUI framework.

<!-- repository-only:start -->

## Run the benchmark fixtures

From the MyGo repository root:

```sh
CGO_ENABLED=0 GOMAXPROCS=1 go test ./ui -run '^$' \
  -bench '^(BenchmarkValueFrame|BenchmarkValueList|BenchmarkDiffSteady)$' \
  -benchmem -count=6 -benchtime=300ms
CGO_ENABLED=0 GOMAXPROCS=1 go test ./ui -run '^$' \
  -bench '^Benchmark(LiveRows|ListRows)' -benchmem -count=6 -benchtime=300ms
CGO_ENABLED=0 GOMAXPROCS=1 go test ./ui/markdown -run '^$' \
  -bench '^BenchmarkView' -benchmem -count=6 -benchtime=300ms
CGO_ENABLED=0 GOMAXPROCS=1 go test ./ui \
  -run '^TestValueFrameRetainedHeap$' -count=3 -v
```

Keep compilation and other tests out of the timed runs.

<!-- repository-only:end -->
