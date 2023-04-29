# Usage

Click the icon in the upper left to view the table of contents.

## Hotkeys

There are some hotkeys which can be used regardless of reset style (multi, wall,
etc.).

| Action              | Purpose                                         |
|---------------------|-------------------------------------------------|
| `ingame_focus`      | Focus active instance (if any).                 |
| `ingame_reset`      | Reset active instance (if any).                 |
| `ingame_toggle_res` | Toggle between resolutions for active instance. |

## Multi

Resetting the current instance will immediately move you to the next instance.
If you have reset all of your instances, you are sent back to the first. There
is currently no intelligent instance selector.

## Wall

You can reset all of your instances or interact with individual instances.
Resetting an instance from ingame will send you back to the wall (or to an idle
locked instance, if using `goto_locked`.)

## Moving Wall

You can reset all of the instances within the first group or interact with
individual instances from any group. Instances are organized in a queue, where
instances in the front of the queue appear in the first group, and instances in
the back of the queue appear in later groups. Instances are added to the queue
whenever they reach the preview screen and/or finish generating.

## Debug Information

resetti allows you to dump some basic information while it is running. You can
type any of the following mnemonics into the console while resetti is running
to do so:

| Mnemonic        | Info                                                   |
|-----------------|--------------------------------------------------------|
| `a`, `all`      | Print everything.                                      |
| `f`, `frontend` | Print information about the frontend (user-facing UI.) |
| `g`, `gc`       | Print garbage collection and memory usage statistics.  |
| `i`, `input`    | Show the current state of inputs.                      |
| `m`, `mgr`      | Show the state of each instance.                       |
