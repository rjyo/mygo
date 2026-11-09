# Meter

<!-- preview meter 200 -->

`ui.Meter` creates a bar showing a value between two bounds, as AppKit's
level indicator and the web's `meter`: in the theme's `Success` color, or
`Warning` and `Danger` from the levels given, nil for none.

```go
size := app.disk.Size
ui.Meter(c, app.disk.Used, 0, size, &ui.MeterLevels{Warning: 0.8 * size, Critical: 0.95 * size}).Label("Disk")
```

`MeterLevels` are the levels from which the value shows it is worse: the
`Warning` color from `Warning`, the `Danger` color from `Critical`. A
`Critical` below `Warning` makes low values the bad ones, as a battery's:

```go
ui.Meter(c, app.battery, 0, 100, &ui.MeterLevels{Warning: 20, Critical: 10}).Label("Battery")
```

## Colors

`NormalColor`, `WarningColor` and `CriticalColor` fill the bar in colors of
your own in place of the theme's `Success`, `Warning` and `Danger`; those left
zero keep the theme's. A storage bar in a neutral gray until it is almost
full, then amber, and the theme's `Danger` at the end:

```go
ui.Meter(c, used, 0, size, &ui.MeterLevels{
	Warning: 0.8 * size, Critical: 0.95 * size,
	NormalColor: ui.RGB(142, 142, 147), WarningColor: ui.RGB(255, 159, 10),
}).Label("Storage")
```

Levels without `Warning` and `Critical` fill it in `NormalColor` throughout,
as a quota bar in a product's own color. `Background` colors the track,
the theme's `Border` by default:

```go
ui.Meter(c, app.used, 0, app.quota, &ui.MeterLevels{NormalColor: brand}).Background(brandTrack).Label("Backup quota")
```

The bar keeps its shape, its range, its direction right to left and what
assistive technology reads of it.

For work going on, use a [progress bar](progress.md).

## Accessibility

Assistive technology sees a level indicator of the value, named by its
`Label`.
