# Setup

This document contains instructions on how to set up resetti. Refer to the main
README for details on how to install resetti.

## Table of Contents

- [Setting up Minecraft](#setting-up-minecraft)
- [Setting up OBS](#setting-up-obs)
- [Configuration](#configuration)
- [Usage](#usage)

## Setting up Minecraft

You will first need to setup your Minecraft instances. resetti has only been
tested with [MultiMC](https://multimc.org/) and forks such as [Prism Launcher](https://prismlauncher.org).
It is highly recommend that you use MultiMC or a derivative, as it is much
better for managing multiple instances than the vanilla Minecraft launcher.

> *Tip:* MultiMC will allow you to duplicate instances. You can create one
> with your desired mods and settings and then copy it multiple times.

Once you have created your Minecraft instances, you will have to place a text
file in each one to let resetti know about their existence. Go to the `.minecraft`
folder of each instance and create a text file named `instance_num` within.
They should have contents like so:

```
instances
├─ 16_MULTI1
│  └─ .minecraft
│     └─ instance_num
│        └─ contents: 0
├─ 16_MULTI2
│  └─ .minecraft
│     └─ instance_num
│        └─ contents: 1
└─ 16_MULTI3
   └─ .minecraft
      └─ instance_num
         └─ contents: 2
```

> *Note:* The names of your instances do not matter here, just that they have
> the `instance_num` file.

resetti will only detect instances with the `instance_num` file. It will only
start up if the instances it detects have numbers starting from 0 and
increasing sequentially (e.g. 0, 1, 2, ...) as in the above example.

You may be able to eliminate the tedium of creating these files with a simple
shell one-liner. For example, with the fish shell:

```sh
for i in seq (1 3); echo (math $i - 1) | tee 16_MULTI$i/.minecraft/instance_num > /dev/null; end
```

> You will have to update this based on the shell you use and how you have your
> instances named.

Lastly, you will have to ensure that all of your instances have *pause on lost
focus* disabled. To do so, you can press F3+P while ingame on each instance.

## Setting up OBS

If you want to record your speedruns, then you should setup OBS. If you want to
use either the wall or set-seed resetters, then OBS is *required.*

Ensure that you have both OBS and [obs-websocket](https://github.com/obsproject/obs-websocket)
installed. On newer versions of OBS (28 and up), obs-websocket *may* come bundled
with OBS (depends on your distribution.) If your distribution's OBS build does not
have obs-websocket bundled, you can compile OBS from source yourself or get it
from somewhere that does have obs-websocket (e.g. flatpak.)

With OBS and `obs-websocket`, you can run `resetti obs` to automatically
generate a scene collection for the amount of instances you are running.

### Notes

After generating a scene collection, you can make edits to it if you would like
(e.g. adding a stream overlay or creating a magnifier scene for using
Ninjabrain Bot.) Leave the existing scene items and scenes untouched, or
resetti may not work.

If you change your base/canvas resolution, you will have to delete
and recreate your scene collection(s).

## Configuration

resetti allows you to have multiple different profiles with different settings.
To begin, you can create a new profile with `resetti new PROFILE_NAME`. This
will place the default configuration at `~/.config/resetti/PROFILE_NAME.toml`.

You can edit the values as needed. You can delete sections irrelevant to your
configuration.

## Usage

Once your configuration profile has been setup, you can launch your instances
and run resetti. Run `resetti PROFILE_NAME` to get started.

Refer to the documentation for your reset style for more detailed information
on how to use resetti from this point onward.

> *Note:* Caps Lock and Num Lock are considered as modifiers. If enabled, they
> will prevent your keybinds from being registered. If you would like to keep
> them on, use `xmodmap` to figure out what modifiers they correspond to and
> add them to your keybinds (e.g. if Num_Lock is present under `mod2`, add
> `mod2` as a modifier to your keybind.)
