# Packaging

resetti uses a data directory (in `$XDG_DATA_HOME` or `$HOME/.local/share/`) to
drop various write-once scripts and other files. If you are creating a package
for a distribution, you are able to overwrite this directory at build time.

```sh
go build -ldflags="-X 'github.com/tesselslate/resetti/internal/res.overrideDataDir=/YOUR_DIR'"
```

Add the above `-ldflags="..."` argument to your build invocation and change
`YOUR_DIR` as desired.

> If you overwrite the data directory with this method, resetti will expect that
> you have already placed the necessary files there and will not create them
> itself. It expects to see all of the resources contained in `internal/res`:
>
> - cgroup_setup.sh
> - default.toml
> - scene-setup.lua
