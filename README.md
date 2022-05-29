# resetti

resetti is a utility for automatically managing and "resetting" one or more
Minecraft instances for speedrunning. It supports Linux (X11 only).

## Table of Contents

**TODO**

# Features

resetti's feature set is larger than most Windows-only reset macros.

- Standard single/multi-instance support
- Wall-style resetting, with keybinds to:
  - Reset an instance
  - Reset all instances
  - Play an instance
  - Play an instance and reset all others
- Supports 1.14+ (Atum required)
- Run with or without WorldPreview
- **Out-of-the-box multi-version support**
  - Play and reset multiple different versions at once
- **OBS integration**
  - Automatically switch scenes with OBS websocket
- **Automatic OBS setup**
  - Create scene collections with a simple guided wizard
  - Automatically pick correct windows for each source

## Planned Features

- Mouse support on wall
- Window stretching for better visibility on wall
- Process affinity for better performance
- 1.7 and 1.8 support

# Installation

## From source (requires [Go](https://go.dev)):

```sh
env CGO_ENABLED=0 go install github.com/woofdoggo/resetti -ldflags="-s -w"
```

## From binary

Check the [Releases](https://github.com/woofdoggo/resetti/releases) tab.
Prebuilt, statically linked 64-bit binaries are available there.
Download the latest version and place it somewhere on your `$PATH`.

# Setup

This section is largely about setting up resetti. However, there is some other
information which you may find useful for setting up Minecraft:

- [Memory Tuning]() **TODO**
- [OpenAL configuration]() **TODO**

## resetti
Once installed, run `resetti --save-default` to get the default configuration.
