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
- [Usage](#usage)
  - [Standard](#standard)
  - [Wall](#wall)
    - [On the Wall](#on-the-wall)
    - [Ingame](#ingame)

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
- Instance locking on wall
- Window stretching for better visibility on wall
- Redetect instances without having to stop and restart
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

Go to the `.minecraft` folder of each instance you will be using. Inside,
create a file named `instance_num` whose contents are nothing but the number
of the instance. Make sure these are in sequential order **starting from 0.**

resetti will only detect instances with the `instance_num` file. It will
refuse to start up when the instances it detects do not have IDs starting from
0 and in sequential order (e.g. 0, 1, 2, ... n)

# Usage

resetti provides a rudimentary text-based user interface. Read through the
section applicable to your reset style ([standard](#standard), [wall](#wall))
to figure out how to operate it.

Regardless of which mode you use, you can switch back to the terminal running
resetti for certain operations. Pressing `ctrl+c` in the terminal with resetti
will stop it.

## Standard

Run `resetti cycle` to get started. Press your Reset keybind to reset the
current instance and switch to the next instance (if any). Press your Focus
keybind whenever you would like to get back to the correct Minecraft instance
if you tabbed away to another window.

## Wall

Run `resetti wall` to get started. If you have not already opened one, resetti
will spawn an OBS projector for the Wall scene. Feel free to resize it or move
it around as needed.

### On the Wall

You can press your general Reset key to reset every instance or your Focus key
to switch to the OBS projector.

You can perform operations on individual instances while on the wall scene.
The number of each instance is calculated from left to right and top to bottom,
like so:

```
123
456
```

Press the number of the instance plus the keys for a given action to perform it.
For example, if you have set your Reset Others keybind to Control, press Control+1
to start playing Instance 1 and reset all other instances. You can do the same
for the other available actions.

### Ingame

You can press your general Reset key to reset the instance and return to the
Wall scene. You can press your Focus key to tab back into the instance if you
have switched away.
