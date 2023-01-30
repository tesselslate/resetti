# Affinity

resetti can (mostly) automatically manage the CPU affinity of your instances
to increase performance while resetting on the wall.

# Resetti Configuration

Each instance can be in one of several affinity "groups," each of which is
allowed to use a certain number of CPU cores/threads. These groups are listed
in the default configuration:

```toml
# - Idle:   Done generating, paused
# - Low:    On the preview, generated more than low_threshold
# - Mid:    A high priority instance in the background while another instance is being played
# - High:   On the dirt screen or below low_threshold
# - Active: Currently being played

# The world generation percentage to reach before moving an instance
# from high priority to low priority.
low_threshold = 20
```

Typically, you'll want to assign all of your CPU cores to the active group, and
then decrease it for each lesser priority group. If you have a 6c/12t CPU, you
would set `affinity_active` to 12. Here's an example configuration for a 6c/12t
CPU:

```toml
affinity_idle = 2
affinity_low = 3
affinity_mid = 8
affinity_high = 11
affinity_active = 12
low_threshold = 20
```

Feel free to experiment and figure out what works best on your system.
