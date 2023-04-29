# Optimization

System-wide improvements are (currently) out of scope for this document. Nothing
has been extensively tested yet, but you may see some gains with e.g. different
schedulers or CPU power governors.

Click the icon in the upper left to view the table of contents.

## Benchmarking

resetti has a simple benchmark utility to test the reset speed of your instances.
It can be downloaded from the [Releases](https://github.com/woofdoggo/resetti/releases)
page (or it may come with a package.)

> When starting up your instances, benchmark results can be wildly variable and
> are likely not representative of actual performance. Perform several preliminary
> runs to allow time for the JVM to warmup and code to get JIT compiled.

<details>

<summary>Benchmark arguments</summary>

```
  -affinity string
    	The affinity type to use (sequence, ccx, none). (default "none")
  -ccx int
    	The number of CCXs to split across for CCX affinity. (default 2)
  -fancy
    	Show a fancy progress display or plain text output.
  -instances int
    	The number of instances to use. Set to 0 to use all instances.
  -pause-after
    	Whether or not to pause all instances before exiting. (default true)
  -profile
    	Whether or not to collect profiling information.
  -reset-count int
    	The number of resets to perform. (default 2000)
  -reset-percent int
    	What percent to reset instances at. 0 for preview, 100 for full load.
```

> When not using the `-fancy` argument, benchmark results are provided in the
> format `RESET_NUM     INST_ID     MS_SINCE_START`. Each value is separated
> by one tab.

</details>

## CPU affinity

resetti has a number of configuration options for CPU affinity, which can make
your resets faster and your ingame experience smoother. See the
[configuration document](https://github.com/woofdoggo/resetti/blob/main/doc/configuration.md)
for more information.

## Garbage collection tuning

The default garbage collector (G1) has been benchmarked to be the fastest for
multi-instance resetting. You can try to adjust the max pause time with
`-XX:MaxGCPauseMillis` for marginally better performance, but it is not likely
to cause any large performance wins. Sticking around 25-50ms is perfectly fine.

If you experience bad framerates ingame and/or run lower instance counts, you can
try ZGC instead by adding `-XX:+UseZGC` to your Java arguments. It is not likely
to be better.

If you are bottlenecked by memory capacity, you can try Shenandoah. **Be warned
that Shenandoah is absolutely terrible for performance, but it does substantially
reduce memory usage.**

```
-XX:+UseShenandoahGC
-XX:ShenandoahGCHeuristics=compact
```

## Malloc improvements

On many (most) distributions, glibc is the default libc (and thus default malloc
implementation.) Unfortunately, it performs ***very*** poorly with Java. To fix
the issue, you can use jemalloc.

First, install `jemalloc` from your distribution's package manager. After doing
so, you'll have to find where the library is located. On many distributions, you
can find it in `/usr/lib` or similar. If your distribution includes
`jemalloc-config` (Debian does not, for instance), you can use that to find where
the jemalloc library is located with `jemalloc-config --libdir`.

After you've found `libjemalloc.so`, you'll need to get your instances to use it.
Open MultiMC and go to `Settings` -> `Custom Commands` and insert the following
into `Wrapper command`, replacing `JEMALLOC_DIR` with the correct directory:

```sh
sh -c "
export LD_PRELOAD='JEMALLOC_DIR/libjemalloc.so';
$INST_JAVA \"$@\"
"
```

> You cannot put newlines in the wrapper command. They are included here purely
> for readability.

You can further tune jemalloc with the `MALLOC_CONF` environment variable. For
example:

```sh
sh -c "
export LD_PRELOAD=`jemalloc-config --libdir`/libjemalloc.so;
export MALLOC_CONF=background_thread:true,narenas:2,dirty_decay_ms:10000,muzzy_decay_ms:10000;
$INST_JAVA \"$@\"
"
```

> See the [jemalloc documentation](https://github.com/jemalloc/jemalloc/blob/dev/TUNING.md)
> for more information on `MALLOC_CONF` if you want to tune it yourself.

## Storing files on a tmpfs

You may get slightly faster resets by storing certain files (e.g. worlds and logs)
on a tmpfs (in memory) instead of a physical disk, especially if you are storing
them to your boot drive. You can link the `saves` and/or `logs` folders of each
instance to separate folders on `/tmp` and clear them out from time to time with
a script.

The script below, for example, will clear out all but the most 20 recent worlds
on each instance every 5 minutes. Adjust as you need (e.g. for instance count,
or the names of your folders on `/tmp`.)


```sh
#!/bin/bash
while true
do
    for i in {1..15}
    do
        cd $i
        rm -r $(ls -t1 | tail -n 20)
        cd ..
    done
done
```

> Since files on `/tmp` are stored in the page cache (typically RAM), previous
> worlds and logs *will occupy memory.* Don't use this if you're close to running
> out of RAM. Additionally, if you have swap, files on `/tmp` can get swapped.

## Transparent huge pages

You may or may not experience slight performance gains by having Java use huge
pages. If your system has THP enabled, you can get your instances to use them
by adding `-XX:+UseTransparentHugePages` to your Java arguments. As with other
changes, do benchmarks when deciding whether or not to use this.

> Huge pages can also incur a slight memory overhead depending on the system,
> but it shouldn't be a huge issue if your system has 2 MB huge pages.
