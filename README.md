# resetti

resetti is a Linux-compatible reset macro for Minecraft speedruns. It supports
a variety of different resetting styles, categories, and Minecraft versions.

## Installation

There are currently no distribution-specific packages available.

### From source

If you have [Go](https://go.dev) installed, you can install from source:

```
env CGO_ENABLED=0 go install github.com/woofdoggo/resetti@latest -ldflags="-s -w"
```

### From binary

You can download the latest version from the [Releases](https://github.com/woofdoggo/resetti/releases)
tab and place it somewhere on your `$PATH`. All binaries are statically linked
and built for AMD64 (x86-64) systems.

## Usage

You can refer to the documentation for detailed usage instructions:

- [Setup](https://github.com/woofdoggo/resetti/blob/main/doc/setup.md)
- [Troubleshooting](https://github.com/woofdoggo/resetti/blob/main/doc/troubleshooting.md)
- [Traditional](https://github.com/woofdoggo/resetti/blob/main/doc/traditional.md)
- [Wall](https://github.com/woofdoggo/resetti/blob/main/doc/wall.md)
- [Set Seed](https://github.com/woofdoggo/resetti/blob/main/doc/setseed.md)

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
  - Instance stretching for better visibility
  - Mouse support for quicker resetting
- Lock instances to specific cores (affinity) for performance
- Supports 1.14+ (Atum required)
- Run with or without WorldPreview
- Multi-version support
  - Reset with multiple different versions at once
- OBS integration
  - Automatically switch OBS scenes
  - Simple setup wizard for generating OBS scene collections

## Contributing

Contributions are welcome.
- Please use proper English for documentation and code comments.
- Ensure that code changes are properly formatted with `go fmt ./...`.
- Perform at least some testing before submitting code changes within a PR.
- Try to keep lines wrapped at roughly 80 columns where possible.

## License

resetti is licensed under the GNU General Public License v3 ONLY, no later
version. You can view the full license [here](https://raw.githubusercontent.com/woofdoggo/resetti/main/LICENSE).

## Prior Art

- Wall
  - jojoe, for creating the wall
  - [MultiResetWall](https://github.com/specnr/multiresetwall) by Specnr and contributors
- Set Seed
  - [spawn-juicer](https://github.com/pjagada/spawn-juicer) by pjagada and contributors
