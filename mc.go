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
	StateUnknown InstanceState = 0
	StateIdle
	StateIngame
	StateGenerating
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
	id      int
	window  xproto.Window
	dir     string
	pid     uint32
	state   InstanceState
	version McVersion
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
		var version McVersion

		// TODO: make this more robust and allow other subversions
		switch verstr {
		case "1.7.10":
			version = Version1_7
		case "1.8.9":
			version = Version1_8
		case "1.14.4":
			version = Version1_14
		case "1.15.2":
			version = Version1_15
		case "1.16.1":
			version = Version1_16
		default:
			fmt.Println("warn: invalid version", verstr)
			version = VersionUnknown
		}

		instance := Instance{
			id:      id,
			window:  win,
			dir:     dir,
			pid:     attrs.pid,
			state:   StateUnknown,
			version: version,
		}

		instances = append(instances, instance)
	}

	return instances, nil
}
