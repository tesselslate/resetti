CGROUP_DIR=/sys/fs/cgroup/resetti;
USER=$(logname);
if [ ! -d "$CGROUP_DIR" ]; then
    mkdir $CGROUP_DIR;
    mount -t cgroup2 none $CGROUP_DIR;
    chown $USER $CGROUP_DIR;
    chown $USER ${CGROUP_DIR}/cgroup.procs;
    echo "+cpuset" > ${CGROUP_DIR}/cgroup.subtree_control;
    for subgroup in idle low mid high active; do
        mkdir ${CGROUP_DIR}/${subgroup}/;
        chown -v $USER ${CGROUP_DIR}/${subgroup}/cgroup.procs;
        chown -v $USER ${CGROUP_DIR}/${subgroup}/cpuset.cpus;
    done
fi
