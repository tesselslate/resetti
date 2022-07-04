# Set-Seed Resetting

The `setseed` manager provides a [spawn-juicer](https://github.com/pjagada/spawn-juicer)
style resetting experience for playing a set seed.

> *Note:* OBS integration must be enabled to reset set seeds.

## Starting Up

Starting resetti will switch you to the wall OBS projector to see all of your
instances. At this point, pressing the `reset` key will begin background
resetting your instances for "good" spawns as defined by your configuration
profile. You can press the `reset` key again once an instance has reached a good
spawn and stopped resetting to tab into that instance and begin playing.

> *Tip:* If you use multiple virtual desktops and some of your instances
> appear blank, switch to the desktop containing your instances and back.

## Playing

Once you begin playing an instance, pressing the `reset` key will reset the
current instance and search for another "good" instance. If one is found, then
resetti will automatically switch you to that instance. If no instances have
found a "good" spawn, then resetti will bring you back to the wall scene where
you can wait for an instance to become ready. Once an instance has reached a good
spawn, pressing the `reset` key will take you to that instance.

## General

At any point, pressing the `focus` key will focus the window of current interest.

If you are waiting for an instance to become ready, the `focus` key will bring
you to the wall projector.

If you are playing an instance, pressing the `focus` key will focus that
instance.
