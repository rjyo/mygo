# Rendering

MyGo draws on the GPU with Metal on macOS, with Direct3D 11 on Windows, or
with WARP, Windows' own software renderer, where no GPU driver works, and
with OpenGL on Linux, in the GtkGLArea GTK shows, and with WebGL 2 in
[browsers](browsers.md), with the shader of Linux. A shader computes rounded
rectangles, borders, gradients and shadows from the distance to their
edges, so they stay sharp at any size and scale, and text comes from a
glyph atlas that only uploads what changes.

With a GPU, every frame draws on it. On Linux, the first window of
native UI loads OpenGL: Mesa, the driver, takes some 50 MB, which stay,
about as much as the rest of a small app. Where OpenGL would not run on a
GPU, as in virtual machines or in WSL (where `GALLIUM_DRIVER=d3d12` gives
Mesa the GPU), Linux draws on the CPU: about a millisecond for a whole
large window on a high-density display, on several cores, and less than a
tenth of one for what typically changes, such as a button under the
pointer, since it redraws, and has the compositor take, only that. Set
`MYGO_GPU=0` to use the CPU renderer everywhere, for instance to compare,
and on Linux `MYGO_GPU=1` to draw with OpenGL even where it runs on the
CPU.

Set `MYGO_FRAME_STATS=1` to log each frame that takes longer than 8 ms, or
`MYGO_FRAME_STATS=4` for another threshold in milliseconds (`all` logs
every frame). Each line tells how long building the frame (and in how many
passes), laying it out, painting and presenting it took, whether it was
drawn on the GPU or in memory (where a window has no GPU renderer), what
the process allocated and how many texts it laid out
meanwhile, and whether the garbage collector ran, with the time goroutines
spent helping it mark:

```
mygo: frame 412 took 17.3 ms: build 3.1 (2 passes), layout 1.2, paint 0.8, present 11.5 (drawn on the GPU), other 0.7; meanwhile the process made about 2312 allocations, 171.0 KB, 12 text layouts; GC: 1 cycles ended, 4.1 ms of assists
```

Allocations and the collector are the whole process's, other goroutines'
included, as `runtime/metrics` counts them: a span at a time, so that
small counts read low. While the variable is unset, frames measure
nothing. Frames that follow each other wait for the display's next
refresh, and present may wait for it too; to see how fast frames could
come, set `MYGO_UNCAPPED=1` as well ([Frame pacing](#frame-pacing)).

MyGo draws a frame only when something changes: input, `Invalidate`,
`After`, or an animation that moves. An idle window draws nothing, and two
seconds after its last frame it frees the frame it drew in memory.

A window animating on macOS keeps three frames for the display, one
more than the two its frames take turns in: with two, a 120 Hz display
holds both often enough that frames wait for one, and some go undrawn.
The third costs about 4 bytes a pixel of the window (8 with colors
outside sRGB) until the window idles.

## Frame pacing

Frames that follow each other, as an animation's, draw at the display's
refreshes, so every scene that takes less than a refresh shows the
display's rate: 120 frames a second on a 120 Hz display, light or heavy.
Set `MYGO_UNCAPPED=1` to present frames without waiting for the display,
to measure how much of a refresh a scene leaves, to compare two builds at
any load, or for the least latency, as ink, remote desktops or games
want. The next frame then starts as soon as the last was presented,
while something animates or asks for frames; a window where nothing
changes still draws nothing. With `MYGO_FRAME_STATS` set too, MyGo logs
how many frames came each second, and each frame logged says uncapped:

```
mygo: 2481 frames a second, uncapped: 0.40 ms a frame
```

The default stays paced: uncapped frames cost the CPU and the GPU all
they can take, and may show torn, which apps don't want. What each
platform does:

| Platform | Uncapped |
|---|---|
| Windows | The swap chain allows tearing where the system can (`DXGI_FEATURE_PRESENT_ALLOW_TEARING`) and every frame presents with sync interval 0, waiting neither for the swap chain's frame latency object nor for DWM's next composition. Frames show torn only where the window takes the display, as a borderless window covering the screen; elsewhere DWM still shows the newest frame at each refresh. Without tearing, frames still present at once, a new one taking the place of one DWM has not shown yet. A window over a material (DirectComposition) has no tearing. While the window changes size, until a frame half a second after the last change, frames present paced, as without `MYGO_UNCAPPED`, and log `paced while resizing`: uncapped frames queued for DWM as it resized the window, stalling a present for seconds. A frame of a new size waits for DWM to show it at most two refreshes, and every frame then presents with `DXGI_PRESENT_DO_NOT_WAIT`: one that Present cannot take within a refresh presents after the next composition, or gives way to the next frame, while the window keeps taking its messages, so that no frame waits on DWM for long. One still not taken after 100 ms presents without the flag, waiting for DXGI on another thread while the window keeps taking its messages, and the next frame draws once it did. After that, frames stay paced, with `DXGI_PRESENT_DO_NOT_WAIT`, until DXGI takes one at once with the resize settled, and the buffers resize, to settle or to grow, on another thread too: DXGI took no frame with the flag again until a call waited for it, and such a call on the main thread held one frame for about a second. A frame that comes while such a call waits is put off, the window showing the last frame. A frame Present refused logs `Present still drawing N times`, and `presenting once DXGI can` when it waits; a frame put off logs `put off while Present waits on another thread` (or `ResizeBuffers`), the frame after it `after Present waited on another thread`, and one paced after the resize settled `paced until DXGI takes a frame at once`. |
| macOS | The display link no longer paces frames: each starts in the run loop's next pass, after the input that came, into a `CAMetalLayer` whose `displaySyncEnabled` is off. The window server still composites a window once a refresh, showing its newest frame, without tearing: frames above the display's rate are drawn and measured, not all shown. |
| Linux | GTK's frame clock starts the next frame as soon as the last was painted, and on X11 the GLX swap interval of the window is 0 (GTK's EGL on Wayland swaps with 0 already). A compositor that paces windows still does: on X11 one sending `_NET_WM_FRAME_TIMINGS`, and on Wayland the frame callbacks of frames drawn in memory. |
| Browsers | Ignored: `requestAnimationFrame` paces frames, and nothing presents sooner. |

A game's steps keep their rate uncapped: [`game.View`](../plugins/game.md)
runs `Update` 60 times a second of time passed, however many frames
come, and `Draw` with how far each frame is into the next step.
