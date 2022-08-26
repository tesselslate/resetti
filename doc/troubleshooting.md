# Troubleshooting

This document contains information about various issues you may encounter and
how to solve them.

## Table of Contents

- [resetti](#resetti)
  - [Keybinds not working](#keybinds-not-working)
  - [Mouse not working](#mouse-not-working)
  - [Resetting not working](#resetting-not-working)
  - [No instance with ID 0](#no-instance-with-id-0)
  - [Instances do not have sequential IDs](#instances-do-not-have-sequential-ids)
- [Minecraft](#minecraft)
  - [Moving Minecraft's audio output](#moving-minecrafts-audio-output)
  - [Excessive memory usage](#excessive-memory-usage)
  - [Crashing with BadWindow (X_QueryPointer)](#crashing-with-badwindow-x_querypointer)

## resetti

This section contains information about issues with resetti.

### Keybinds not working

Make sure you are only pressing the keys for your keybind. If you have Caps
Lock, Num Lock, or any other modifiers on, turn them off. If it still does not
work, please open an issue with your configuration profile and the output of
`xev` when pressing the keybind(s) in question.

### Mouse not working

If you are using the wall with mouse enabled, you will be unable to click on
anything while resetti considers you to be on the wall. Enter an instance to
regain control of your mouse.

Sometimes, resetti will fail to grab the mouse pointer even though the X server
does not report an error. In this case, clicking will not do anything. Use the
number keys to perform actions on the wall and try again the next time you go
back to the wall.

### Resetting not working

Check that you have pause on lost focus (F3+P) disabled. Try increasing the
`delay` value in your configuration profile. If you have SleepBackground, make
sure that your framerates are not set to excessively low values (e.g. 1.) If
none of these solve the problem, please open an issue with your configuration
profile and more details on what problem you are experiencing.

### No instance with ID 0

resetti was not able to find an instance whose `instance_num` file contained an
ID of 0. Ensure that you set up your `instance_num` files properly and that all
of your instances are running.

### Instances do not have sequential IDs

As stated in the setup document, resetti requires that all instances have unique
identifiers starting from 0 and increasing sequentially. If you are running `n`
instances, check the `instance_num` files of each instance to ensure that they
contain all numbers from 0 through `n-1`.

## Minecraft

This section contains information about Minecraft-specific issues.

### Moving Minecraft's audio output

Minecraft uses [OpenAL](https://www.openal.org/) for audio. By default, it may
prevent you from switching Minecraft's audio output with the PulseAudio backend.
To fix this, create a file named `~/.alsoftrc` (or `~/.config/alsoft.conf`) with
the following contents:

```
[general]
drivers = pulse

[pulse]
allow-moves = true
```

> *Note:* If you have Pipewire but are using `pipewire-pulse`, switching your
> OpenAL driver to `pipewire` also works just fine (and is necessary if you
> want to use instance freezing.)

### Excessive memory usage

There are two main things to improve memory usage:
1. GC tuning
2. Malloc tuning

#### Tuning the Java garbage collector

If you are limited primarily by your RAM, then use Shenandoah with the following
JVM arguments:

```
-XX:+UseShenandoahGC
-XX:ShenandoahGCHeuristics=compact
```

If memory usage is not an issue, try using ZGC instead to improve performance:

```
-XX:+UseZGC
```

#### Improving malloc performance

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
$INST_JAVA \"$@\"
"
```

> You cannot put newlines in the wrapper command text input. They are here
> purely for readability.

If this crashes with `/usr/bin/java: 1: jemalloc-config: not found`, then find
the location of `libjemalloc.so` on your system and edit the LD_PRELOAD line
to point directly to that library.

However, this alone will likely not help much. To further improve Minecraft's
memory usage, you can tune jemalloc with the `MALLOC_CONF` environment variable.
More detailed information on the subject can be found [here](https://github.com/jemalloc/jemalloc/blob/dev/TUNING.md),
but this configuration should help reduce memory usage substantially:

```sh
sh -c "
export LD_PRELOAD=`jemalloc-config --libdir`/libjemalloc.so;
export MALLOC_CONF=background_thread:true,narenas:2,dirty_decay_ms:10000,muzzy_decay_ms:10000;
$INST_JAVA \"$@\"
"
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

#### Using system GLFW

On MultiMC and forks, go to `Edit Instance` -> `Settings` -> `Workarounds` and
enable `Use system installation of GLFW`. Ensure that you have installed an
up-to-date (>= 3.3.3) version of GLFW from your package manager.

#### Compiling your own GLFW build

Create an empty directory (henceforth referred to as `GLFW_DIR`) anywhere of
your choosing. Place an up-to-date (>= 3.3.3) version of `libglfw.so` inside
`GLFW_DIR`. You can follow the instructions for [compiling it yourself](https://https://www.glfw.org/docs/latest/compile.html)
from source.

Lastly, add this additional JVM argument to all of your Minecraft instances:

```
-Dorg.lwjgl.librarypath=GLFW_DIR
```
