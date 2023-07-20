# Setup

This document contains instructions on how to set up resetti. Refer to the main
README for details on how to install resetti.

Click the icon in the upper left to view the table of contents.

## Setting up Minecraft

resetti has only been tested with [MultiMC](https://multimc.org) and forks such
as [PrismLauncher](https://prismlauncher.org). No functionality is guaranteed
when using the vanilla launcher or any other launcher.

### Java

You'll need a relatively recent version of Java for certain mods to work
correctly. OpenJDK 19 currently appears to have the best performance with
Minecraft (benchmarks show OpenJDK 20 to be slower), so get it if available.

### Minecraft

We will create a single instance, and then duplicate it as many times as
needed. Start by creating an instance in MultiMC (or your fork) with the desired
version (e.g. 1.16.1). Launch and close the game once; then, install Fabric by
going to `Edit Instance` -> `Version`. Unfortunatly, resetti does not currently 
work with Flatpak launchers.

#### Mods

You can use [ModCheck](https://github.com/RedLime/ModCheck) to download any
mods you would like. Atum and Fast Reset are mandatory. The following are
*heavily recommended* if they are available for the version you are playing:

- Sodium
- Starlight
- Lithium
- SpeedRunIGT
- WorldPreview
- LazyDFU
- Voyager
- LazyStronghold
- SleepBackground
- antiresourcereload
- StandardSettings

> ServerSideRNG can be installed to make verifying your runs easier. However,
> it still has some issues (as of April 2023) that may impact your experience.

> Force Port can be installed if you plan on doing co-op runs.

#### Configuration

- Disable "pause on lost focus." To do so, enter a world and press F3+P. Verify
  that the chat message says it is disabled.
- You'll want to disable `syncChunkWrites` in `options.txt`, which can be found in
  the instance's `.minecraft` folder.
- You may want to enable `Use Global Options` in the SpeedRunIGT options from
  ingame.
- If using StandardSettings, you may want to read the documentation (available [here](https://github.com/KingContaria/StandardSettings#standardsettings)).
  - In particular, you may want to use a global configuration file. This is
    mentioned in the documentation.
- If you want to make updating your settings easier, you can softlink your `config` 
  and `mods` folders to one place like so: `cd YOUR_NEW_INSTANCE; ln -s YOUR_CONFIG config`.
- If using SleepBackground, the default configuration is suboptimal. You can add
  the below configuration to `.minecraft/config/sleepbg.json`.
  - You may have a better experience by tweaking some of these values, but they
    are a better starting point than the defaults.

<details>

<summary>sleepbg.json</summary>

```json
{
  "world_preview": {
    "_description": "config for world preview, every time (loading_screen) is rendered (render_times) times, will be render a preview. ex) if (loading_screen.fps_limit) is 30 and this value is 2, preview fps will be 15 (as 30 / 2).",
    "enable": true,
    "render_times": 1
  },
  "background": {
    "_description": "It works when instance is in the background after joined the world.",
    "enable": true,
    "fps_limit": 1
  },
  "world_setup": {
    "_description": "same with (background) config but for (max_ticks) ticks after the joined the world.",
    "enable": true,
    "fps_limit": 30,
    "max_ticks": 20
  },
  "log_interval": {
    "_description": "Changes how often the game prints the worldgen progress to the log file, may be useful for macros (minimum: 50ms, max/default: 500ms)",
    "enable": true,
    "log_interval": 500
  },
  "loading_screen": {
    "_description": "It works when instance is in the world loading screen. minimum (fps_limit) is 15.",
    "enable": true,
    "fps_limit": 30
  },
  "lock_instance": {
    "_description": "It works when instance is in the background with sleepbg.lock file is exist in user directory at every interval ticks. (for macros option)",
    "enable": true,
    "fps_limit": 1,
    "tick_interval": 10,
    "wp_render_times_enable": true,
    "wp_render_times": 10
  }
}
```
</details>

#### Instance Numbers

Once you've created one instance, duplicate it as needed to reach the desired
number of instances. Once all of your instances have been created, they will
each need their own `instance_num` file to let resetti know their ID. Here is
an example of how that looks:

```
instances
├─ 16_MULTI1
│  └─ .minecraft
│     └─ instance_num
│        └─ contents: "0"
├─ 16_MULTI2
│  └─ .minecraft
│     └─ instance_num
│        └─ contents: "1"
└─ 16_MULTI3
   └─ .minecraft
      └─ instance_num
         └─ contents: "2"
```

> The instance names are not important, although a consistent format like this
> makes it easier to run shell commands to operate on all of your instances at
> once.

You can use a shell one-liner to create the `instance_num` files, if you'd like.
Here's an example in [fish](https://fishshell.com); adjust it for your shell.

```sh
for i in (seq 1 3); echo (math $i - 1) | tee 16_MULTI$i/.minecraft/instance_num; end
```

When running resetti, it will only detect instances with an `instance_num`
file. resetti will refuse to start if it cannot detect a set of instances whose
IDs start at 0 and increase sequentially (0, 1, 2, .. n).

## Setting up OBS

If using OBS, you will need to perform some additional setup. Refer to the
[OBS document](https://github.com/woofdoggo/resetti/blob/main/doc/obs.md) for
more information.

## Optimization and Fixes

On most distributions, the out-of-the-box experience playing Minecraft is quite
subpar. Refer to the [optimization document](https://github.com/woofdoggo/resetti/blob/main/doc/optimization.md)
and [common issues](https://github.com/woofdoggo/resetti/blob/main/doc/common-issues.md)
for more information.

## Configuring resetti

To start, create a new configuration profile. You can create as many as you
would like and are able to choose which to use whenever you launch resetti.

```sh
resetti new PROFILE_NAME
```

The above command will create a new profile at `$XDG_CONFIG_HOME/resetti/PROFILE_NAME.toml`,
or `$HOME/.config/resetti/PROFILE_NAME.toml` if `$XDG_CONFIG_HOME` is unset.

The generated configuration profile will contain all of the available options
with some documentation comments to explain their purpose. You may find the
[configuration document](https://github.com/woofdoggo/resetti/blob/main/doc/configuration.md)
helpful for more detailed information on certain settings.

## Running

Congratulations! Once you've set everything up, you can get started by simply
running `resetti PROFILE_NAME`. Refer to the [usage document](https://github.com/woofdoggo/resetti/blob/main/doc/usage.md)
for more information on how to use resetti once you've started it.

- If you've configured affinity, it may prompt you for root privileges to
  perform the necessary setup.
- If you're using OBS, you'll need to switch to the correct scene collection
  and may need to open up a projector for your wall scene.

If you encounter any issues or think this documentation could be improved, feel
free to join the [Discord](https://discord.gg/fwZA2VJh7k) or open an issue.
Happy resetting!
