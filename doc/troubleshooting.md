# Troubleshooting

This document contains information about various issues you may encounter and
how to solve them.

## Table of Contents

- [Minecraft](#minecraft)

## Minecraft

This section contains information about Minecraft-specific issues.

### Moving Minecraft's audio output

Minecraft uses [OpenAL](https://www.openal.org/) for audio. By default, it may
prevent you from switching Minecraft's audio output. If you have PulseAudio
(or `pipewire-pulse`), create a file named `~/.alsoftrc` with the following
contents:

```
[general]
drivers = pulse

[pulse]
allow-moves = true
```

### Excessive memory usage

You can provide some additional arguments to the JVM to tune the garbage
collector (see [here](https://www.reddit.com/r/feedthebeast/comments/921woe/comment/e32ndog))
and cap heap allocation at 1.5-2GB, although this is usually not the main issue.

On most distributions, glibc is the default (or only) libc available. glibc's
malloc implementation has been known to perform poorly with Java. A band-aid
fix for the issue is to set the `MALLOC_ARENA_MAX` to a low value, such as 2.
Another option which may work better for you is to use `jemalloc` with a custom
malloc config.

First, install `jemalloc` from your distribution's package manager. Then, for
each of your instances in MultiMC, go to `Edit Instance` -> `Settings` ->
`Custom Commands`. Enable custom commands and insert the following into
`wrapper command`:

```sh
sh -c "
export LD_PRELOAD=`jemalloc-config --libdir`/libjemalloc.so;
$INST_JAVA $@
"
```

> You cannot put newlines in the wrapper command text input. They are here
> purely for readability.

However, this alone will likely not help much. To further improve Minecraft's
memory usage, you can tune jemalloc with the `MALLOC_CONF` environment variable.
More detailed information on the subject can be found [here](https://github.com/jemalloc/jemalloc/blob/dev/TUNING.md),
but this configuration should help reduce memory usage substantially:

```sh
sh -c "
export LD_PRELOAD=`jemalloc-config --libdir`/libjemalloc.so;
export MALLOC_CONF=background_thread:true,narenas:2,dirty_decay_ms:10000,muzzy_decay_ms:10000;
$INST_JAVA $@"
```

> The newlines are again only present for readability.

If the above configuration does not produce great results, you may want to
experiment further. There are other options for `MALLOC_CONF` not shown here
and the values provided above may not suit your system well.

### Crashing with BadWindow (X_QueryPointer)

Older versions of GLFW have a bug where closing a window while Minecraft is
not focused can cause a crash. The bug was fixed in version 3.3.3, which is
newer than what Minecraft uses. You can fix the issue by forcing Minecraft to
use a newer version of GLFW.

Create an empty directory (henceforth referred to as `GLFW_DIR`) anywhere of
your choosing. Place an up-to-date (>= 3.3.3) version of `libglfw.so` inside
`GLFW_DIR`. You can get it either from your distribution's package manager or
by [compiling it yourself](https://https://www.glfw.org/docs/latest/compile.html)
from source.

Lastly, add this additional JVM argument to all of your Minecraft instances:

```
-Dorg.lwjgl.librarypath=GLFW_DIR
```
