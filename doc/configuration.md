# Configuration

Click the icon in the upper left to view the table of contents.

## Resolutions

If you are not using instance stretching or alternate resolutions, you can
delete or ignore the `alt_res` and `play_res` options. If you are using either,
`play_res` is mandatory.

## Hooks

Hooks are *not* run as shell commands. If you want to use any shell features
(such as variable expansion), call a shell from your hook (e.g. `sh -c "..."`).

## Keybinds

While you are able to run several actions with a single keybind, certain
combinations may produce odd effects. It's fine to have both wall and ingame
actions on the same keybind. If you're on the wall when activating the bind,
then only wall actions will be taken (and vice versa for ingame).
