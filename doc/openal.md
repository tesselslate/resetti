# OpenAL

> This document assumes you are using PulseAudio (or an alternative
> implementation of it, such as `pipewire-pulse`.) You may or may not encounter
> the issue described below.

Minecraft uses a library known as [OpenAL](https://www.openal.org/) for audio.
By default, you may have difficulty moving Minecraft's audio output from one
PulseAudio sink to another.

To fix this, create a file named `~/.alsoftrc` and insert the following
contents:

```
[general]
drivers = pulse

[pulse]
allow-moves = true
```

Reboot any open Minecraft instances and you should now be able to switch
Minecraft's audio output with your tool of choice.
