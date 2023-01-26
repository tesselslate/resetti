for p in $(pgrep java)
do
    echo $p > /sys/fs/cgroup/resetti/cgroup.procs;
done