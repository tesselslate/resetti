#define _GNU_SOURCE

#include <errno.h>
#include <sched.h>
#include <stdint.h>
#include <string.h>
#include <unistd.h>

uint64_t get_proc_affinity(uint64_t pid);
uint64_t get_proc_count();
int set_proc_affinity(uint64_t pid, uint64_t cpus);
char *get_error();

// Get the CPU cores a process is assigned to.
// Returns 0 if the call to `sched_getaffinity` fails.
uint64_t get_proc_affinity(uint64_t pid) {
    cpu_set_t cpu_set;

    int result = sched_getaffinity(pid, sizeof(cpu_set_t), &cpu_set);
    if (result == -1) {
        return 0;
    }

    uint64_t res;
    for (int i = 0; i < get_proc_count(); i++) {
        if (CPU_ISSET(i, &cpu_set)) {
            res |= 1 << i;
        }
    }

    return res;
}

// Get the amount of processors for setting affinity.
uint64_t get_proc_count() {
    return sysconf(_SC_NPROCESSORS_ONLN);
}

// Set the affinity for a given process.
// Returns 0 if successful, -1 otherwise.
int set_proc_affinity(uint64_t pid, uint64_t cpus) {
    cpu_set_t cpu_set;

    CPU_ZERO(&cpu_set);

    for (int i = 0; i < get_proc_count(); i++) {
        if ((cpus & (1 << i)) != 0) {
            CPU_SET(i, &cpu_set);
        }
    }

    return sched_setaffinity(pid, sizeof(cpu_set_t), &cpu_set);
}

// Return the result of strerror(errno).
char *get_error() {
    return strerror(errno);
}
