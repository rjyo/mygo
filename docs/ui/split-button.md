# Split button

<!-- preview split-button 120 -->

`ui.SplitButton` creates a button joined to an arrow that opens a menu of
related actions, as Save beside Save As…, Run beside Run with…, or New
beside New from template…. It is one control, drawn as a
[button](button.md) is: one face and one border around its two segments,
with a line between them, each lighting up apart as the pointer hovers and
presses it. `ui.PrimarySplitButton` is one in the accent color, as
`ui.PrimaryButton` is.

It returns its parts: `Button`, the leading segment, shows the label and
does the action, and its `Clicked` reports a click, or Enter or Space
while it has the keyboard focus, as a button's does; `Arrow`, the
trailing segment, opens the menu that its `Menu` gives it, the
[system's](menu-button.md#any-button), as a menu button's opens below it:

```go
save := ui.SplitButton(c, "Save")
save.Arrow.Menu(func(m *ui.Menu) {
	if m.Item("Save As…").Chosen() {
		app.saveAs()
	}
	if m.Item("Save a Copy…").Chosen() {
		app.saveCopy()
	}
})
if save.Button.Clicked() {
	app.save()
}
```

The menu takes the items of [context menus](context-menu.md#items), and
its function runs again in the frame after one was chosen, where that
item's `Chosen` reports it.

## Drawn in the window

`Arrow.PopoverMenuButton` gives it a menu MyGo draws in the window with
the theme's look, as [`ui.PopoverMenuButton`](menu-button.md#drawn-in-the-window)
does, with the same function, keys and options: `ui.PopoverMenuButtonOptions`
place it elsewhere along the arrow, as lined up with its end, and let the
app open and close it:

```go
run := ui.PrimarySplitButton(c, "Run")
run.Arrow.PopoverMenuButton(func(m *ui.Menu) {
	for _, target := range app.targets {
		if m.Item("Run " + target).Chosen() {
			app.run(target)
		}
	}
}, ui.PopoverMenuButtonOptions{At: ui.AnchorBottomEnd, Self: ui.AnchorTopEnd})
if run.Button.Clicked() {
	app.run(app.target)
}
```

A window that draws all its menus ([every menu of a
window](context-menu.md#every-menu-of-a-window)) draws `Arrow.Menu`'s too.

## Content and states

Give the button other content, as an icon before the label, with
`Button.Children` and an empty label, and name it with `Button.Label`, as
a [button](button.md#content). `Group`, which holds the two segments,
lays out among its siblings as a button does: a margin, an alignment or a
size goes on it.

`Group.Disabled(true)` disables the whole control: neither segment takes
clicks or the focus, and both show grayed out. `Button.Disabled(true)`
disables the action alone, and the arrow still opens the menu, as Save
does while there is nothing new to save but Save As… is still there:

```go
save := ui.SplitButton(c, "Save")
save.Group.Disabled(app.saving)
save.Button.Disabled(!app.dirty)
```

In a [toolbar](toolbar.md), it takes no face of its own until the pointer
is over it, as the toolbar's buttons: then its face, the line between its
segments, and the segment under the pointer a shade deeper show. When the
toolbar has no room for it, its overflow menu lists its two segments:
choosing the button's does the action, and the arrow's opens the menu.

Right to left, the arrow is at the left, and its menu opens at the start
of the arrow, as a [menu button's](menu-button.md) does.

## Keyboard

A split button is one stop of Tab, which goes to the segment that had the
focus last, the button at first. Left and Right move the focus between
its segments, the other way round right to left; in a toolbar, they go on
to its other controls. Enter and Space click the button while it has the
focus, and open the menu while the arrow has it, as do Up and Down;
Alt+Down opens the menu from the button too.

## Accessibility

Assistive technology sees a group holding a button and a menu button, the
usual model of a split button: the button named by its text or its
`Label`, and the menu button named "More options", in the app's language,
until `Arrow.Label` names it, expanded while its menu shows. `Group.Label`
names the group.

## In tests

The segments are found by their names: `tt.Click("Save")` clicks the
button, and `tt.Click("More options")`, or the arrow's own `Label`, opens
the menu, which `tt.Menu` lists and `tt.ChooseMenuItem` chooses from, or,
drawn in the window, whose items `tt.Click` chooses. See
[testing](testing.md).
