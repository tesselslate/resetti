# variables
CGROUP_DIR=/sys/fs/cgroup/resetti
USERNAME=$(logname)

# create cgroup directory
if [ ! -d "$CGROUP_DIR" ]; then
    mkdir $CGROUP_DIR
    mount -t cgroup2 none $CGROUP_DIR
fi

# gain ownership of cgroup directory and setup cpuset handler
chown "$USERNAME" $CGROUP_DIR
chown "$USERNAME" $CGROUP_DIR/cgroup.procs
echo "+cpuset" > $CGROUP_DIR/cgroup.subtree_control

# create subgroups
for subgroup in idle low mid high active; do
    mkdir $CGROUP_DIR/$subgroup
    chown "$USERNAME" $CGROUP_DIR/$subgroup/cgroup.procs
    chown "$USERNAME" $CGROUP_DIR/$subgroup/cpuset.cpus
done
