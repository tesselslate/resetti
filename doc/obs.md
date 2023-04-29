# OBS

Click the icon in the upper left to view the table of contents.

## Requirements

If using OBS, you will need to have `obs-websocket` version 5.0 or greater.
If you are using OBS 28 or newer, `obs-websocket` *may* come pre-installed -
Some distributions, such as [Arch Linux](https://bugs.archlinux.org/task/76710),
do not bundle it. In this case, you'll want to install OBS via the
[Flatpak](https://flathub.org/apps/com.obsproject.Studio) or
[build it from source](https://obsproject.com/wiki/Build-Instructions-For-Linux).

## Setup

resetti comes with a simple OBS script that can generate scene collections for
you. To start, create a new scene collection. Then, import the script by going to
`Tools` -> `Scripts` and clicking the `+` icon.

- If you installed from source or from binary, the script can be found at
  `$XDG_DATA_HOME/resetti/scene-setup.lua` (or `~/.local/share/resetti/scene-setup.lua`).
- If you installed from a package, it may be located elsewhere (such as within
  `/usr`.) Check any documentation for it.

The script options should be fairly self explanatory. If you are just using multi
(no wall), you can disable the wall options. If you are using the wall and don't
want lock icons, you can disable the lock icon options.

## Additional Setup

If you want to use preview freezing, you will need to install the
[Freeze Filter](https://obsproject.com/forum/resources/freeze-filter.950/) plugin.

If you are using wall, you will need to record the verification (or wall) scene.
If you are using stretched instances, you may need to update the verification
scene to show the chunkmap on each instance more clearly.

If you need to recreate your scene collection in the future for any reason,
you will have to delete the existing scenes (`Wall`, `Verification`, and
`Instance`.)
