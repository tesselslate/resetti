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

## Moving

The groups system allows for very flexible layouts. Here are a few examples
for a 1080p base resolution:

<details>

<summary>Classic Boyenn Moving</summary>

This layout does not stretch instances to fill each section as is typical with
the original moving style.

![Classic](https://user-images.githubusercontent.com/46545045/235276175-a3b2a0dd-cd71-4989-861c-5681f93b73ab.png)

```toml
[[wall.moving.groups]]
position = "1800x600+0,0"
width = 3
height = 2

[[wall.moving.groups]]
position = "120x1080+1800,0"
width = 1
height = 9

[wall.moving.locks]
position = "1800x480+0,600"
width = 3
height = 2
```

</details>

<details>

<summary>Julti Style</summary>

![Julti Style](https://user-images.githubusercontent.com/46545045/235276171-1afd796b-b45b-45ac-b872-e68d5d8efba5.png)

```toml
[[wall.moving.groups]]
position = "1920x900+0,0"
width = 3
height = 2

[wall.moving.locks]
position = "1920x180+0,900"
width = 6
height = 1
```

</details>

<details>

<summary>Several Groups</summary>

![Several Groups](https://user-images.githubusercontent.com/46545045/235276166-25d1027a-7a1d-4d4c-9126-d0a846e5459c.png)

```toml
[[wall.moving.groups]]
position = "1800x360+0,0"
width = 2
height = 2

[[wall.moving.groups]]
position = "1800x360+0,360"
width = 2
height = 2

[[wall.moving.groups]]
position = "1800x360+0,720"
width = 2
height = 2

[wall.moving.locks]
position = "120x1080+1800,0"
width = 1
height = 8
```

</details>

<details>

<summary>Boyenn Wall-In-Wall</summary>

![Wall-In-Wall](https://user-images.githubusercontent.com/46545045/235276160-7c372e5c-d2f2-4b5d-9724-c09f2d1a5320.png)

```toml
[[wall.moving.groups]]
position = "1920x800+0,140"
width = 2
height = 2

[[wall.moving.groups]]
position = "640x240+640,420"
width = 2
height = 2
cosmetic = true

[[wall.moving.groups]]
position = "1920x140+0,0"
width = 8
height = 1

[wall.moving.locks]
position = "1920x140+0,940"
width = 8
height = 1
```

</details>

## Performance

### SleepBackground

The `sleepbg_path` accepts a directory. If left blank, it will default to `$HOME`.
`sleepbg.lock` is automatically appended to whatever path is used.

### Sequence Affinity

Sequence affinity assigns one CPU core to each instance. The additional values
can be used to specify how many CPU cores (or threads, if using SMT) are
dedicated to instances in certain states. For example, setting `active_cpus` to 12
will give the active instance (if any) access to the first 12 CPU cores/threads
while it is being played.

### Advanced Affinity

`ccx_split` determines how many groups instances should be split into. If your
CPU has a single CCX (e.g. all cores access the same L3 cache), leave it as 1.
If you are running a multi-CCX CPU (or one with multiple separate L3 caches),
adjust the value accordingly.

> resetti will print out the number of auto-detected cores and CCXs when starting
> with advanced affinity, which you can use as a guide (but always verify.)
>
> If the printed information is incorrect, please file an issue or let us know.
> The automatic cache hierarchy detection has not been tested on a wide range
> of CPUs.

Each `affinity_X` value specifies how many CPU cores (or threads, if using SMT)
are to be dedicated to each instance state. The provided values are a fine
example for a 6c/12t CPU (or a 12c/24t, like the 5900X, if using `ccx_split`.)

> When creating the affinity groups, resetti will automatically sort by CPU core
> (if using SMT) to try and maximize locality when instances are moved between
> logical cores.
