# Common Issues

Click the icon in the upper left to view the table of contents.

## GLFW issues

Minecraft bundles a fairly out-of-date version of [GLFW](https://www.glfw.org/),
a library used for input and window handling. It has numerous bugs, some of which
you may run into while playing.

### Crashing with BadWindow (X_QueryPointer)

This is a known issue which was fixed in version 3.3.3 of GLFW.

- [Fix commit](https://github.com/glfw/glfw/commit/539f4bdca28ba959dd631dd2e90fded528cfc942)

Older versions of GLFW have a bug where closing a window while Minecraft is not
focused can cause a crash. The bug was fixed in version 3.3.3, which is newer
than what Minecraft uses.

### Inputs getting delayed or dropped

This is a [known issue](https://github.com/glfw/glfw/pull/1472) which was fixed
in version 3.3.3 of GLFW.

- [Fix commit](https://github.com/glfw/glfw/commit/606c0fc03e05d8260aceec188fb1d9074527de0c)
- [Mojang bug](https://bugs.mojang.com/browse/MC-122421)

### Weird cursor warping and broken mouse movement

Setting your mouse sensitivity with libinput's `Coordinate Transformation Matrix`
property causes cursor warping to break entirely. There is no upstream fix
yet, but the issue [is known.](https://github.com/glfw/glfw/issues/1860) You can
fix it by applying [this patch](https://github.com/woofdoggo/resetti/blob/main/contrib/glfw-xinput.patch)
and [building GLFW from source](#building-glfw-from-source).

#### Building GLFW from source

You can follow the instructions [here](https://www.glfw.org/docs/latest/compile.html)
for compiling GLFW from source. 3.3.8 is the latest version compatible with Minecraft,
so do not build from master. Checkout the 3.3.8 tag.

## Minecraft issues

### Excessive memory usage

If you use glibc, then your instances will use an egregious amount of memory
(their RSS will likely climb to several times the max heap size.) See the
[optimization document](https://github.com/woofdoggo/resetti/blob/main/doc/optimization.md)
for information on how to reduce memory usage.

### Flickering chunk borders

If you have an AMD GPU, this is an issue with Mesa. I've [filed a bug report](https://gitlab.freedesktop.org/mesa/mesa/-/issues/8950).
In the mean time, exporting `AMD_DEBUG=nonggc` to your instances should solve the problem.

### Instances stuck on dirt screen

Sometimes, one or more of your instances may get stuck on the dirt screen while
generating a world for a long time (up to several seconds). This is a known issue
and is being investigated, but there is no known cause or fix yet.

### Unable to move audio output

By default, OpenAL will not let you move Minecraft's audio output between
PulseAudio sinks. To fix this, create `~/.alsoftrc` or `~/.config/alsoft.conf`
with the following contents:

```ini
[general]
drivers = pulse

[pulse]
allow-moves = true
```

### Using Pipewire

The version of OpenAL bundled with Minecraft is too old to support Pipewire.
You can install OpenAL from your package manager and turn on `Use system
installation of OpenAL` in MultiMC to use a newer version with Pipewire support.

## OBS issues

### Instances appear blank

If your window manager unmaps windows on other workspaces, you'll have to keep
everything on a single workspace.

### Instances freeze when tabbing in

Window captures might freeze if several window geometry changes happen in quick
succession. This should not be a problem with most window managers, but it is for
some (e.g. dwm, which moves windows offscreen and back.)
