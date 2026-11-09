# Segmented control

<!-- preview segmented 200 -->

`ui.Segmented` creates a segmented control showing labels, of which an
`*int` is the index of the one chosen, as a switch between views: a click
chooses a segment, as do the arrows while it has the keyboard focus, and
`Changed` reports a new choice.

```go
ui.Segmented(c, &app.view, "List", "Grid").Label("View")
switch app.view {
case 0:
	app.list(c)
case 1:
	app.grid(c)
}
```

It is one stop of Tab, which goes to the segment chosen.

## Full width

Each segment is as wide as its label. `FillEqually` makes the control as
wide as its parent and shares the width equally among the segments, as a
view switcher or a setting across a panel shows them:

```go
ui.Segmented(c, &app.appearance, "Light", "Dark", "System").FillEqually().Label("Appearance")
```

The segment chosen takes its share, its label in the middle, and a label
longer than its share ends with an ellipsis. The segments keep their look,
their keys, what assistive technology reads of them, and their disabled
state. A width set after `FillEqually`, as `Width(320)`, is the control's,
which its segments still share. A [toggle group](toggle.md) takes it too.

## Icons and other content

For segments of icons or of your own look, build them with
`ui.SegmentedBase`, a `Track` of `Segment`s without a look:

```go
seg := ui.SegmentedBase(c, &app.view, 2)
seg.Track.Padding(3).Radius(8).Background(t.Surface).Children(func() {
	for i, icon := range []*ui.SVG{listIcon, gridIcon} {
		s := seg.Segment(i).Padding(4, 10).Radius(6).Label([]string{"List", "Grid"}[i])
		if i == app.view {
			s.Background(t.Background)
		}
		s.Children(func() { ui.Icon(c, icon) })
	}
})
```

Call `FillEqually` on the `Track` after building its segments to share its
width among them. See [custom widgets](custom-widgets.md).

## Accessibility

Assistive technology sees a radio group, named by its `Label`, of radio
buttons named by their labels.
