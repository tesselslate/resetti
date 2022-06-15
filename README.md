# resetti

resetti is a utility for automatically managing and "resetting" one or more
Minecraft instances for speedrunning. It supports Linux (X11 only).

> While resetti is still beta software, it should work reasonably well.
> That being said, please report any bugs which you encounter.

## Table of Contents

- [Features](#features)
  - [Planned Features](#planned-features)
- [Installation](#installation)
  - [From source](#from-source-requires-go)
  - [From binary](#from-binary)
- [Setup](#setup)
  - [resetti](#resetti-1)
- [Configuration](#configuration)
- [Usage](#usage)
  - [Standard](#standard)
  - [Wall](#wall)
    - [On the Wall](#on-the-wall)
    - [Ingame](#ingame)
- [Miscellaneous](#miscellaneous)

# Features

resetti's feature set ~~is~~ will soon be larger than most Windows-only reset macros.

- Standard single/multi-instance support
- Wall-style resetting
  - Functions:
    - Reset an instance
    - Reset all instances
    - Play an instance
    - Play an instance and reset all others
    - Lock an instance
  - Stretch instances for visibility
  - Mouse support for easier resetting
- Set process affinity for better performance
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

- Redetect instances without having to stop and restart
- 1.7 and 1.8 support
- Packages for various distributions

# Installation

## From source (requires [Go](https://go.dev)):

```sh
env CGO_ENABLED=0 go install github.com/woofdoggo/resetti@latest -ldflags="-s -w"
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

> If you would like to use the OBS integration (or if you are using wall, it is
> **required**) then you will have to install [obs-websocket](https://github.com/obsproject/obs-websocket).

- Run `resetti --save-default` to get the default configuration
- Edit as needed (refer to [Configuration](#configuration))
- Run `resetti keys` to setup your keybinds
- Run `resetti obs` to setup your OBS scene collection(s)

Go to the `.minecraft` folder of each instance you will be using. Inside,
create a file named `instance_num` whose contents are nothing but the number
of the instance. Make sure these are in sequential order **starting from 0.**

Depending on how you have your instances named, you can do this with a shell
one-liner. For example, in the fish shell:

```fish
for i in (seq 1 6); echo (math $i - 1) | tee 16_MULTI$i/.minecraft/instance_num > /dev/null; end
```

resetti will only detect instances with the `instance_num` file. It will
refuse to start up when the instances it detects do not have IDs starting from
0 and in sequential order (e.g. 0, 1, 2, ... n)

> **NOTE:** Make sure all of your instances have pause on lost focus
> (F3+P) disabled.

# Configuration

Run `resetti --save-default` to generate the default configuration file. It will
generate this (without the comments):

```yaml
# OBS integration settings. These should be self-explanatory.
obs:
  enabled: false
  port: 4440
  password: password

# Keybinds for resetting regardless of your reset style.
# Use `resetti keys` to set these up.
keys:
  reset: 0;0
  focus: 0;0

# Keybinds for resetting on the wall. Use `resetti keys`
# to set these up.
wall:
  mod-reset: 0
  mod-reset-others: 0
  mod-play: 0
  mod-lock: 0
  # Whether or not to enable mouse support on the wall.
  # Please note that this will prevent you from using the
  # mouse *entirely* while on the wall scene. You won't be
  # able to click on other windows until you enter an instance.
  use-mouse: false

# Reset settings.
reset:
  # Minecraft settings to be set automatically upon leaving
  # a world. The `set-settings` option below toggles this.
  mc:
    fov: 70
    rd: 16
    sensitivity: 100
  # This setting only applies on the wall. If enabled, instances will be
  # stretched while resetting for greater visibility. Please be cautious
  # when enabling this if you are photosensitive.
  stretch-windows: true
  # Whether or not to adjust your Minecraft settings automatically when
  # resetting an instance. Please be cautious when enabling this if you
  # are photosensitive, the menu will flash on screen.
  set-settings: false
  
  # The delay (in milliseconds) to use when switching between
  # menus.
  delay: 50

# The file to use for counting resets. If empty, persistent reset
# counting is disabled. If provided, resetti will automatically
# update the given file with the amount of resets you have done.
reset-file: ""
# When used, this setting will set each Minecraft instance to run
# on one specific CPU core/thread. This can offer a decent
# performance boost. Possible values are:
# "alternate" - allocate instances to every other thread
# "sequence" - allocate instances to threads starting from 0
# nothing - Do not use affinity.
affinity: ""
```

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

The names of each action should be self explanatory. However, "Lock" might confuse
you if you are not already familiar with it. Locking an instance will prevent it from
being reset by any means. You can unlock an instance by either pressing Lock
again or by playing the instance and resetting.

### Ingame

You can press your general Reset key to reset the instance and return to the
Wall scene. You can press your Focus key to tab back into the instance if you
have switched away.

# Miscellaneous

## License

resetti is licensed under the GNU General Public License v3 ONLY, no later
version. You can view the full license [here](https://raw.githubusercontent.com/woofdoggo/resetti/main/LICENSE).

## Prior Art

- Specnr's [MultiResetWall](https://github.com/specnr/multiresetwall)
- jojoe's wall macro
- Others I'm probably not aware of
