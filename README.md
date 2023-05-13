# resetti [![Discord](https://img.shields.io/discord/1095808506239651942?style=flat-square)](https://discord.gg/fwZA2VJh7k)

resetti is a Linux-compatible reset macro for Minecraft speedruns. It supports
a variety of different resetting styles, categories, and Minecraft versions.

## Installation

Distribution specific packages are currently only limited to **Arch** and
**Debian-based** distributions. More distributions are planned for later.

- [From the AUR](#from-the-aur)
- [From source](#from-source)
- [From binary](#from-binary)

### From the AUR

There are two AUR packages available: `resetti` and `resetti-git`.
Install with your AUR helper of choice, or manually:

```sh
git clone https://aur.archlinux.org/{resetti,resetti-git}
cd resetti
makepkg -si
```

### From the Debian package

Check the [Releases](https://github.com/woofdoggo/resetti/releases) tab for
Debian packages or download the latest development builds from the
[Discord](https://discord.gg/fwZA2VJh7k).

### From source

If you have [Go](https://go.dev) installed, you can install from source:

```sh
# Stable
env CGO_ENABLED=0 go install -ldflags="-s -w" github.com/woofdoggo/resetti@latest

# Dev
env CGO_ENABLED=0 go install -ldflags="-s -w" github.com/woofdoggo/resetti@dev
```

### From binary

You can download the latest version from the [Releases](https://github.com/woofdoggo/resetti/releases)
tab and place it somewhere on your `$PATH`. All binaries are statically linked
and built for AMD64 (x86-64) systems.

## Usage

You can refer to the [documentation](https://github.com/woofdoggo/resetti/blob/main/doc/README.md)
for detailed usage instructions.

Please report any bugs which you encounter. resetti is still beta software and
is not guaranteed to work.

## Features

- Single- and multi-instance support
- Wall-style resetting
  - Reset all instances
  - Reset an instance
  - Play an instance and reset all others
  - Play an instance
  - Lock an instance
  - Instance stretching, hiding, and preview freezing
  - Mouse support
  - Moving wall
    - Define one or more "groups" of instances to interact with
- Flexible keybind and "hook" system for running commands
- Lock instances to specific cores (affinity) for performance
- Supports 1.14+ (Atum required)
- Run with or without WorldPreview
- Multi-version support
  - Reset with multiple different versions at once
- OBS integration
  - Automatically switch OBS scenes
  - Simple setup wizard for generating OBS scene collections

## Contributing

Contributions are welcome. Join the [Discord](https://discord.gg/fwZA2VJh7k) or
open an issue to discuss the changes you'd like to make.
- Please use proper English for documentation and code comments.
- Ensure that code changes are properly formatted with `go fmt ./...`.
- Perform at least some testing before submitting code changes within a PR.
- Use `make check`.
- Try to keep lines wrapped at roughly 80 columns where possible.

## License

resetti is licensed under the GNU General Public License v3 ONLY, no later
version. You can view the full license [here](https://raw.githubusercontent.com/woofdoggo/resetti/main/LICENSE).

## Prior Art

- Wall
  - jojoe, for creating the wall
  - boyenn's moving wall ideas
  - [MultiResetWall](https://github.com/specnr/multiresetwall) by Specnr and contributors
