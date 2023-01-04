# Affinity

resetti can (mostly) automatically manage the CPU affinity of your instances
to increase performance while resetting on the wall.

# System Configuration

> This is a mess. If you can find a better way to keep the affinity mask of all
> threads of a process, please open an issue to let me know. If this doesn't
> work for you, see the `Ramblings` section below (and good luck!)

resetti's affinity management makes use of cgroups to confine instances to
running on certain CPU threads. As utilizing cgroups requires root privileges
by default, you'll need to set them up yourself first.

> Your system will need to have cgroup2. Check that running `grep cgroup2
> /proc/filesystems` returns a line containing cgroup2.

Create a shell script with the following contents, replacing `[USER]` with
your username:

```sh
#!/usr/bin/sh
mkdir /sys/fs/cgroup/resetti
chown [USER] /sys/fs/cgroup/resetti
chown [USER] /sys/fs/cgroup/resetti/cgroup.procs
chown [USER] /sys/fs/cgroup/resetti/cpuset.cpus
mount -t cgroup2 none /sys/fs/cgroup/resetti
echo "+cpuset" > /sys/fs/cgroup/resetti/cgroup.subtree_control
for subgroup in idle low mid high active; do
    mkdir /sys/fs/cgroup/resetti/${subgroup}
    chown [USER] /sys/fs/cgroup/resetti/${subgroup}/cgroup.procs
    chown [USER] /sys/fs/cgroup/resetti/${subgroup}/cpuset.cpus
done
```

This script will need to be executed **with root privileges** once every time
you boot up your system and before you'd like to run. Alternatively, you may use
`libcgroup` to automatically create the necessary cgroup at startup, or you may
want to get your init system to run this script for you.

Then, create another script with the following contents:

```sh
#!/usr/bin/sh
for p in $(pgrep java)
do
    echo $p > /sys/fs/cgroup/resetti/cgroup.procs
done
```

You will need to run this **with root privileges** after you start up your
instances, every time you start them up.

<details>
    <summary>Ramblings (cgroup setup nonsense)</summary>

    If you actually need this information, good luck! I spent about 8 hours
    figuring out cgroups enough to get this to work on my system. It might
    be somewhat useful. Enjoy the passive aggressiveness!

    Also, you might be wondering why this is necessary. This is the first way
    I've found to manage the affinity for all threads of a given process that
    doesn't require giving resetti root privileges (or capabilities.) The fact
    that the setup still requires superuser privileges is not ideal, but oh well.
    I do not want to find another method, I've wasted probably 50 hours on getting
    affinity working correctly-ish over the past half a year.

    As (debatably useful) references, check:
    - [Some cgroup manpage](https://man7.org/linux/man-pages/man7/cgroups.7.html)
    - [The kernel.org docs](https://www.kernel.org/doc/html/v5.0/admin-guide/cgroup-v2.html#controllers)

    First, we have to give ownership of a bunch of files in the cgroup pseudo-FS
    to our user so that we can modify them without root privileges. Then, we have
    to mount a new pseudo-FS to create a new cgroup hierarchy for resetti.

    We need a new cgroup hierarchy to rid the resetti cgroup of its type (which it
    would have if it were just a "normal" cgroup underneath the root one.) This way,
    the subgroups beneath resetti end up having a type of `domain`. If they were
    threaded (or if the resetti group were threaded), then nothing would work because
    you can't write to `cgroup.procs` in threaded cgroups. They would be threaded if
    resetti were a normal subgroup.

    There are some other miscellaneous concerns regarding moving processes between
    cgroups, but those should be solved with the 2nd script above.

    TL:DR - The subgroups (idle, low, mid, high, active) must have a type of `domain`
    (check cgroup.type). We can only move processes between cgroups with a type of
    `domain`. We can also only move processes if we have write privileges to the
    `cgroup.procs` file in both the source and destination cgroups. If these conditions
    aren't met on your system, good luck with figuring it out! Feel free to open an
    issue but I can't guarantee that I can help.

</details>

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
