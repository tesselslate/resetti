# Wall Resetting

The `wall` manager provides an efficient way of resetting multiple instances
for random seeds. It is a literal wall of one or more Minecraft instances
where you can choose what seeds to play or reset.

## Starting Up

Open up an OBS projector for your wall scene before starting `resetti`.

> *Tip:* If you use multiple virtual desktops and some of your instances
> appear blank, switch to the desktop containing your instances and back.

## On the Wall

- You can press your `reset` key to reset all instances.
- You can press your `focus` key to focus the projector if you tab away.

resetti provides two ways of managing individual instances
(the number keys and the mouse.) The latter requires you to enable mouse
support within your configuration profile.

### Number Keys

resetti allows you to perform actions on instances by pressing their number
key with the appropriate modifier held down. resetti orders instances from left
to right and top to bottom, like so:

```
123
456
```

Here, pressing the `1` key while holding down whatever modifier is set for the
`wall_reset` bind within your configuration would reset instance 1. The same
is true for all of the other instances and `wall_*` keys in your configuration.

### Mouse

Instead of using the number keys, you can enable mouse support within your
configuration. Doing so allows you to click on instances to perform various
actions.

**Please note that enabling mouse support will prevent you from using the mouse
entirely while on the wall. You will have to play an instance to regain control
of your mouse.**

> *Note:* Make sure that your projector is entirely fullscreen - no window
> decorations, titlebar, e.t.c. If it is not, where you see instances and where
> resetti thinks instances are will be different.

For example, if your `wall_reset` bind is set to Shift, then shift clicking an
instance will reset it. You can continue to hold Shift and the mouse button
down while dragging your cursor over other instances to apply the same action
to multiple instances quickly.

> *Note:* Unfortunately, the mouse support can be inconsistent at times, even
> though the X server reports no errors with grabbing the mouse pointer. You
> may have to use the number keys if your mouse does not do anything.

### Locking

When an instance is "locked," it *cannot* be reset. As such, you can lock
instances which look good to play (e.g. coastal spawns) and then press your
`reset` key to reset all other unlocked instances.

Playing an instance or attempting to lock it again will result in it unlocking,
at which point it can be reset via normal means.

If you setup a lock indicator image with `resetti obs`, then you will see it in
the corner of locked instances.

## Ingame

While ingame, you can press the `reset` and `focus` keys. The `focus` key will
simply focus the instance. The `reset` key will send you back to the wall scene
and reset the instance, *unless* you have enabled `goto_locked` and there are
other locked instances.

### Goto Locked

When `goto_locked` is enabled, resetting an instance while ingame will switch
you to the next locked instance until there are no more locked instances, at
which point resetti will send you back to the wall. This eliminates the need to
repeatedly swap back to the wall when there are multiple good seeds you would
like to play.
