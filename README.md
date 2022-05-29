# resetti

resetti is a utility for automatically managing and "resetting" one or more
Minecraft instances for speedrunning. It supports Linux (X11 only).

## Table of Contents

- [Features](#features)
  - [Planned Features](#planned-features)
- [Installation](#installation)
  - [From source](#from-source-requires-go)
  - [From binary](#from-binary)
- [Setup](#setup)
  - [resetti](#resetti-1)

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

- [Memory tuning](https://github.com/woofdoggo/resetti/blob/main/doc/tuning.md)
- [OpenAL configuration](https://github.com/woofdoggo/resetti/blob/main/doc/openal.md)

## resetti

- Run `resetti --save-default` to get the default configuration
- Edit as needed
- Run `resetti keys` to setup your keybinds
- Run `resetti obs` to setup your OBS scene collection(s)
- Run `resetti cycle` or `resetti wall` to begin using resetti
