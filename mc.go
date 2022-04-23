package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jezek/xgb/xproto"
)

type InstanceState int

const (
	StateUnknown    InstanceState = 0
	StateIdle       InstanceState = 1
	StateIngame     InstanceState = 2
	StateGenerating InstanceState = 3
)

type McVersion int

const (
	VersionUnknown McVersion = 0
	Version1_7     McVersion = 7
	Version1_8     McVersion = 8
	Version1_14    McVersion = 14
	Version1_15    McVersion = 15
	Version1_16    McVersion = 16
)

type Instance struct {
	Id      int
	Window  xproto.Window
	Dir     string
	Pid     uint32
	State   InstanceState
	Version McVersion
}

func GetInstances(x *XClient) ([]Instance, error) {
	windows, err := x.GetWindowList(x.root)
	if err != nil {
		return nil, err
	}

	instances := []Instance{}

	for _, win := range windows {
		// check if window is Minecraft
		attrs, err := x.GetWindowAttributes(win)
		if err != nil {
			continue
		}

		if !strings.Contains(attrs.class[0], "Minecraft") {
			continue
		}

		// TODO: This could be made better. MultiMC and its forks omit
		// the --gameDir argument (I believe the vanilla launcher uses
		// it, perhaps more do?)
		//
		// It is also possible to parse the file `/proc/$pid/environ`
		// for INST_DIR, INST_MC_DIR, e.t.c. I would have to
		// investigate vanilla launcher behavior to determine the best
		// method for getting the game directory (although nobody should
		// be using the vanilla launcher, it's pretty bad...)

		// get instance path
		argbytes, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", attrs.pid))
		if err != nil {
			continue
		}

		dir := ""
		args := strings.Split(string(argbytes), "\x00")
		for _, arg := range args {
			if !strings.Contains(arg, "-Djava.library.path") {
				continue
			}

			dirsplit := strings.Split(arg, "=")
			dir = strings.ReplaceAll(dirsplit[1], "natives", ".minecraft")
			break
		}

		if dir == "" {
			continue
		}

		// get instance number
		var id int

		numbytes, err := os.ReadFile(fmt.Sprintf("%s/instance_num", dir))
		if err == nil {
			id, err = strconv.Atoi(strings.Trim(string(numbytes), "\n"))
			if err != nil {
				continue
			}
		} else {
			id = -1
		}

		// get instance version
		verstr := strings.Split(attrs.class[0], " ")[1]
		verstr = strings.Split(verstr, ".")[1]
		var version McVersion

		switch verstr {
		case "7":
			version = Version1_7
		case "8":
			version = Version1_8
		case "14":
			version = Version1_14
		case "15":
			version = Version1_15
		case "16":
			version = Version1_16
		default:
			fmt.Println("warn: invalid version", verstr)
			version = VersionUnknown
		}

		instance := Instance{
			Id:      id,
			Window:  win,
			Dir:     dir,
			Pid:     attrs.pid,
			State:   StateUnknown,
			Version: version,
		}

		instances = append(instances, instance)
	}

	return instances, nil
}
