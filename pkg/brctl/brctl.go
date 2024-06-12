// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package brctl

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"
)

func Addbr(name string) error {
	brctl_socket, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)
	if err != nil {
		return fmt.Errorf("unix.Socket: %w", err)
	}

	if _, err := executeIoctlStr(brctl_socket, unix.SIOCBRADDBR, name); err != nil {
		return fmt.Errorf("executeIoctlStr: %w", err)
	}

	name_bytes, err := unix.ByteSliceFromString(name)
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	args := []int64{int64(BRCTL_ADD_I), int64(uintptr(unsafe.Pointer(&name_bytes))), 0, 0}
	if _, err := ioctl(brctl_socket, unix.SIOCSIFBR, uintptr(unsafe.Pointer(&args))); err != nil {
		return fmt.Errorf("ioctl: %w", err)
	}

	return nil
}

func Delbr(name string) error {
	brctl_socket, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)
	if err != nil {
		return fmt.Errorf("unix.Socket: %w", err)
	}

	if _, err := executeIoctlStr(brctl_socket, unix.SIOCBRDELBR, name); err != nil {
		return fmt.Errorf("executeIoctlStr: %w", err)
	}

	name_bytes, err := unix.ByteSliceFromString(name)
	if err != nil {
		return fmt.Errorf("unix.ByteSliceFromString: %w", err)
	}

	args := []int64{int64(BRCTL_DEL_BRIDGE), int64(uintptr(unsafe.Pointer(&name_bytes))), 0, 0}
	if _, err := ioctl(brctl_socket, unix.SIOCSIFBR, uintptr(unsafe.Pointer(&args))); err != nil {
		return fmt.Errorf("ioctl: %w", err)
	}

	return nil
}

// Create dummy device for testing: `sudo ip link add eth10 type dummy`
func Addif(name string, iface string) error {
	brctl_socket, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)
	if err != nil {
		return fmt.Errorf("unix.Socket: %w", err)
	}

	ifr, err := unix.NewIfreq(name)
	if err != nil {
		return fmt.Errorf("unix.NewIfreq: %w", err)
	}

	if_index, err := getIndexFromInterfaceName(iface)
	if err != nil {
		return fmt.Errorf("getIndexFromInterfaceName: %w", err)
	}
	ifr.SetUint32(uint32(if_index))

	//SIOCBRADDIF
	//SIOCGIFINDEX
	if err := unix.IoctlIfreq(brctl_socket, unix.SIOCBRADDIF, ifr); err != nil {
		return fmt.Errorf("unix.IoctlIfreq: %w", err)
	}

	// prepare args for ifr
	// apparently the go unix api does not support setting the second union value to a raw pointer
	// so we have to manually craft sth that looks like a struct ifreq
	var args = []int64{int64(BRCTL_ADD_I), int64(if_index), 0, 0}
	ifrd := getIfreqOption(ifr, unsafe.Pointer(&args[0]))

	if _, err = ioctl(brctl_socket, unix.SIOCDEVPRIVATE, uintptr(unsafe.Pointer(&ifrd))); err != nil {
		return fmt.Errorf("ioctl: %w", err)
	}

	return nil
}

// Create dummy device for testing: `sudo ip link add eth10 type dummy`
func Delif(name string, iface string) error {
	brctl_socket, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)
	if err != nil {
		return fmt.Errorf("unix.Socket: %w", err)
	}

	ifr, err := unix.NewIfreq(name)
	if err != nil {
		return fmt.Errorf("unix.NewIfreq: %w", err)
	}

	if_index, err := getIndexFromInterfaceName(iface)
	if err != nil || if_index == 0 {
		return fmt.Errorf("getIndexFromInterfaceName: %w", err)
	}
	ifr.SetUint32(uint32(if_index))

	if err := unix.IoctlIfreq(brctl_socket, unix.SIOCBRDELIF, ifr); err != nil {
		return fmt.Errorf("unix.IoctlIfreq: %w", err)
	}

	// set args
	var args = []int64{int64(BRCTL_DEL_I), int64(if_index), 0, 0}
	ifrd := getIfreqOption(ifr, unsafe.Pointer(&args[0]))

	if _, err = ioctl(brctl_socket, unix.SIOCDEVPRIVATE, uintptr(unsafe.Pointer(&ifrd))); err != nil {
		return fmt.Errorf("ioctl: %w", err)
	}
	return nil
}

// All bridges are in the virtfs under /sys/class/net/<name>/bridge/<item>, read info from there
// Update this function if BridgeInfo struct changes
func getBridgeInfo(name string) (BridgeInfo, error) {
	base_path := BRCTL_SYS_NET + name + "/bridge/"
	bridge_id, err := os.ReadFile(base_path + "bridge_id")
	if err != nil {
		return BridgeInfo{}, fmt.Errorf("os.ReadFile: %w", err)
	}

	stp_enabled, err := os.ReadFile(base_path + "stp_state")
	if err != nil {
		return BridgeInfo{}, fmt.Errorf("os.ReadFile: %w", err)
	}

	stp_enabled_bool, err := strconv.ParseBool(strings.TrimSuffix(string(stp_enabled), "\n"))
	if err != nil {
		return BridgeInfo{}, fmt.Errorf("strconv.ParseBool: %w", err)
	}

	// get interfaceDir from sysfs
	interfaceDir, err := os.ReadDir(BRCTL_SYS_NET + name + "/brif/")
	if err != nil {
		return BridgeInfo{}, fmt.Errorf("os.ReadDir: %w", err)
	}

	interfaces := []string{}
	for i := range interfaceDir {
		interfaces = append(interfaces, interfaceDir[i].Name())
	}

	return BridgeInfo{
		Name:       name,
		BridgeID:   strings.TrimSuffix(string(bridge_id), "\n"),
		StpState:   stp_enabled_bool,
		Interfaces: interfaces,
	}, nil

}

// for now, only show essentials: bridge name, bridge id interfaces
func showBridge(name string, out io.Writer) {
	info, err := getBridgeInfo(name)
	if err != nil {
		log.Fatalf("show_bridge: %v", err)
	}

	ifaceString := ""
	for _, iface := range info.Interfaces {
		ifaceString += iface + " "
	}

	fmt.Fprintf(out, "%s\t\t%s\t\t%v\t\t%v\n", info.Name, info.BridgeID, info.StpState, ifaceString)
}

// The mac addresses are stored in the first 6 bytes of /sys/class/net/<name>/brforward,
// The following format applies:
// 00-05: MAC address
// 06-08: port number
// 09-10: is_local
// 11-15: timeval (ignored for now)
func Showmacs(name string, out io.Writer) error {

	// parse sysf into 0x10 byte chunks
	brforward, err := os.ReadFile(BRCTL_SYS_NET + name + "/brforward")
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	fmt.Fprintf(out, "port no\tmac addr\t\tis_local?\n")

	for i := 0; i < len(brforward); i += 0x10 {
		chunk := brforward[i : i+0x10]
		mac := chunk[0:6]
		port_no := uint16(binary.BigEndian.Uint16(chunk[6:8]))
		is_local := uint8(chunk[9]) != 0

		fmt.Fprintf(out, "%3d\t%2x:%2x:%2x:%2x:%2x:%2x\t%v\n", port_no, mac[0], mac[1], mac[2], mac[3], mac[4], mac[5], is_local)
	}

	return nil
}

func Show(out io.Writer, names ...string) error {
	fmt.Println("bridge name\tbridge id\tSTP enabled\t\tinterfaces")
	if len(names) == 0 {
		devices, err := os.ReadDir(BRCTL_SYS_NET)
		if err != nil {
			return fmt.Errorf("%w", err)
		}

		for _, bridge := range devices {
			// check if device is bridge, aka if it has a bridge directory
			_, err := os.Stat(BRCTL_SYS_NET + bridge.Name() + "/bridge/")
			if err == nil {
				showBridge(bridge.Name(), out)
			}
		}
	} else {
		for _, name := range names {
			showBridge(name, out)
		}
	}
	return nil
}

// Spanning Tree Options
func Setageingtime(name string, time string) error {
	ageing_time, err := stringToJiffies(time)
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	if err = setBridgeValue(name, BRCTL_AGEING_TIME, uint64(ageing_time), uint64(BRCTL_SET_AEGING_TIME)); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

// Set the STP state of the bridge to on or off
// Enable using "on" or "yes", disable by providing anything else
// The manpage states:
// > If <state> is "on" or "yes"  the STP  will  be turned on, otherwise it will be turned off
// So this is actually the described behavior, not checking for "off" and "no"
func Stp(bridge string, state string) error {
	var stp_state uint64
	if state == "on" || state == "yes" {
		stp_state = 1
	} else {
		stp_state = 0
	}

	if err := setBridgeValue(bridge, BRCTL_STP_STATE, stp_state, uint64(BRCTL_SET_BRIDGE_PRIORITY)); err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}

// The manpage states only uint16 should be supplied, but brctl_cmd.c uses regular 'int'
// providing 2^16+1 results in 0 -> integer overflow
func Setbridgeprio(bridge string, bridgePriority string) error {
	prio, err := strconv.ParseUint(bridgePriority, 10, 16)
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	if err := setBridgeValue(bridge, BRCTL_BRIDGE_PRIO, prio, uint64(BRCTL_SET_AEGING_TIME)); err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}

func Setfd(bridge string, time string) error {
	forward_delay, err := stringToJiffies(time)
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	if err := setBridgeValue(bridge, BRCTL_FORWARD_DELAY, uint64(forward_delay), uint64(BRCTL_SET_AEGING_TIME)); err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}

func Sethello(bridge string, time string) error {
	hello_time, err := stringToJiffies(time)
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	if err := setBridgeValue(bridge, BRCTL_HELLO_TIME, uint64(hello_time), uint64(BRCTL_SET_AEGING_TIME)); err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}

func Setmaxage(bridge string, time string) error {
	max_age, err := stringToJiffies(time)
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	if err := setBridgeValue(bridge, BRCTL_MAX_AGE, uint64(max_age), uint64(BRCTL_SET_AEGING_TIME)); err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}

// port ~= interface
func Setpathcost(bridge string, port string, cost string) error {
	path_cost, err := strconv.ParseUint(cost, 10, 64)
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	if err := setBridgePort(bridge, port, BRCTL_PATH_COST, path_cost, uint64(BRCTL_SET_PATH_COST)); err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}

func Setportprio(bridge string, port string, prio string) error {
	port_priority, err := strconv.ParseUint(prio, 10, 64)
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	if err := setBridgePort(bridge, port, BRCTL_PRIORITY, port_priority, uint64(BRCTL_SET_PATH_COST)); err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}

func Hairpin(bridge string, port string, hairpinmode string) error {
	var hairpin_mode uint64
	if hairpinmode == "on" {
		hairpin_mode = 1
	} else {
		hairpin_mode = 0
	}

	if err := setBridgePort(bridge, port, BRCTL_HAIRPIN, hairpin_mode, 0); err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}
