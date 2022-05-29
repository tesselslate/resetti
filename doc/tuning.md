# Motivation
Minecraft's memory usage is an annoyance when speedrunning on Linux. glibc's
memory allocator is not great for Java - even when setting a maximum heap
size of 3GB, it can eventually consume more than double that.

jemalloc seems to provide far more reasonable memory consumption and lower
fragmentation, without any noticeable drop in performance. jemalloc also
releases memory back to the kernel - something glibc appears to do rarely (if
at all) with Minecraft. There are
[a](https://blog.arkey.fr/drafts/2021/01/22/native-memory-fragmentation-with-glibc/)
[large](https://engineering.linkedin.com/blog/2021/taming-memory-fragmentation-in-venice-with-jemalloc)
[number](https://medium.com/nerds-malt/java-in-k8s-how-weve-reduced-memory-usage-without-changing-any-code-cbef5d740ad)
[of](https://devcenter.heroku.com/articles/tuning-glibc-memory-behavior)
[posts](https://www.ibm.com/docs/en/wmla/1.2.0?topic=performance-virtual-memory-usage-is-high-some-services-why)
[detailing](https://github.com/prestodb/presto/issues/8993)
[glibc's](https://github.com/cloudfoundry/java-buildpack/issues/320)
[poor](https://thehftguy.com/2020/05/21/major-bug-in-glibc-is-killing-applications-with-a-memory-limit/)
[performance](https://bugs.openjdk.java.net/browse/JDK-8193521) with Java.

# Methodology
Tested each configuration once. Resets were performed by clicking
`menu.quitWorld`, unfocusing the game, waiting for the world to finish
generating, and immediately clicking the reset button again until the desired
number of resets was reached.

Both minimum and maximum allocation were set to 2GB for all tests in
[PolyMC](https://github.com/PolyMC/PolyMC).

# Conclusion
jemalloc works fine, far better than glibc's ptmalloc implementation. The
memory usage eventually stabilized and stopped climbing with the setup listed
below.

You can install jemalloc from your distribution's package manager and use a
wrapper command in your launcher to have Minecraft use it. This is what I use
with [PolyMC](https://github.com/PolyMC/PolyMC), a fork of MultiMC. You can
adjust it as needed:

```sh
sh -c "export LD_PRELOAD='/usr/lib/libjemalloc.so'; export MALLOC_CONF=background_thread:true,narenas:2,dirty_decay_ms:10000,muzzy_decay_ms:10000; $INST_JAVA $@"
```

The below settings are by no means perfect. If you experience performance issues,
you may have more luck trying different GC implementations. There are plenty of
other settings which might help with memory usage, performance, e.t.c.

One JVM argument I would advise you not to use is `-XX:+DisableExplicitGC`. You
will probably experience OOM crashes in certain scenarios even though they
probably should not happen. The most notable I found was adjusting render
distance with F3+F - you can crash your game in less than a second by
increasing RD with a small heap size.

Additionally, if you are using WorldPreview, you will likely want to omit
`-XX:MaxDirectMemorySize=xxM`. This unfortunately results in greater memory
consumption, but WorldPreview can cause unplayable stuttering (or crashes)
with this limit set too low.

```sh
# malloc config
export MALLOC_CONF=background_thread:true,narenas:2,dirty_decay_ms:10000,muzzy_decay_ms:10000

# jvm args
-XX:+UseG1GC
-Dsun.rmi.dgc.server.gcInterval=999999
-XX:+UnlockExperimentalVMOptions
-XX:MaxGCPauseMillis=50
-XX:G1NewSizePercent=20
-XX:G1ReservePercent=20
-XX:G1HeapRegionSize=32M
-XX:MaxDirectMemorySize=384M
```

# Software
```
minecraft       1.16.1
fabric          0.13.3
jemalloc        1:5.2.1-6 (arch pkg), provides: libjemalloc.so=2-64
jdk             openjdk 18-2
kernel          5.17.2-arch3-1
```

## Mods
```
atum            1.0.5+1.16.1
dynamic fps     0.1
fastreset       1.4.0
lazydfu         0.1.2
lazystronghold  1.1.1
lithium port    0.6.6
motiono         1.0.1+1.16
sodium port     0.2.0+build.17
speedrunigt     7.2+1.16.1
starlight       1.0.0-rc2
voyager         1.0.0
```

# Results
-------------------------------------------------------------------------------
```
jvm:            -XX:+UseG1GC
                -Dsun.rmi.dgc.server.gcInterval=999999
                -XX:+UnlockExperimentalVMOptions
                -XX:MaxGCPauseMillis=50
memory:         2048 MB
malloc:         jemalloc
malloc tuning:  none

                RSS
launch to menu: 2.1G
launch world:   2.3G
unpause:        2.7G
10 resets:      3.1G
20 resets:      3.0G
```

-------------------------------------------------------------------------------
```
jvm:            -XX:+UseG1GC
                -Dsun.rmi.dgc.server.gcInterval=999999
                -XX:+UnlockExperimentalVMOptions
                -XX:MaxGCPauseMillis=50
            +   -XX:G1NewSizePercent=20
            +   -XX:G1ReservePercent=20
            +   -XX:G1HeapRegionSize=32M
            +   -XX:MaxDirectMemorySize=384M
memory:         2048 MB
malloc:         jemalloc
malloc tuning:  none

                RSS
launch to menu: 1.2G
launch world:   1.5G
unpause:        1.9G
10 resets:      2.8G
20 resets:      3.1G
```

-------------------------------------------------------------------------------
```
jvm:            -XX:+UseG1GC
                -Dsun.rmi.dgc.server.gcInterval=999999
                -XX:+UnlockExperimentalVMOptions
                -XX:MaxGCPauseMillis=50
            +   -XX:G1NewSizePercent=20
            +   -XX:G1ReservePercent=20
            +   -XX:G1HeapRegionSize=32M
            +   -XX:MaxDirectMemorySize=384M
memory:         2048 MB
malloc:         jemalloc
malloc tuning:  background_thread:true,narenas:2

                RSS
launch to menu: 1.2G
launch world:   1.4G
unpause:        1.8G
10 resets:      2.7G
20 resets:      2.9G
```

-------------------------------------------------------------------------------
```
jvm:            -XX:+UseG1GC
                -Dsun.rmi.dgc.server.gcInterval=999999
                -XX:+UnlockExperimentalVMOptions
                -XX:MaxGCPauseMillis=50
            +   -XX:G1NewSizePercent=20
            +   -XX:G1ReservePercent=20
            +   -XX:G1HeapRegionSize=32M
            +   -XX:MaxDirectMemorySize=384M
memory:         2048 MB
malloc:         jemalloc
malloc tuning:  background_thread:true,narenas:2,dirty_decay_ms:10000,muzzy_decay_ms:10000

                RSS
launch to menu: 1.2G
launch world:   1.5G
unpause:        1.7G
10 resets:      2.7G
20 resets:      2.8G
```
