// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package traceroute

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

var (
	SOCKET_GLOBAL = unix.Socket(unix.AF_INET, unix.SOCK_RAW, unix.IPPROTO_ICMP)
)

type Flags struct {
	Ipv4               bool   // Force IPv4
	Ipv6               bool   // Force IPv6
	Icmp               bool   // Use ICMP
	Tcp                bool   // Use TCP
	Debug              bool   // Debug
	DontFragment       bool   // Don't fragment
	FirstTtl           uint   // First TTL
	Gateway            string // Gateway
	Iface              string // Interface
	MaxHops            int    // Max number of hops
	SimQueries         int    // Number of probes to send to each hop simultaneously
	NoMap              bool   // Do not map IP addresses to host names when displaying
	Port               int    // Port number
	Tos                int    // Type of Service
	Flowlabel          string // IPV6 flow label
	Wait               string // How long to wait for a response (passed as [,here,near])
	Nqueries           uint   // Number of probes to send to each hop
	BypassRoutingTable bool   // Bypass the routing table
	SourceAddr         string // Source address
	SendWait           int    // Minimal  time  interval  between probes
	Extensions         bool   // Show ICMP Extensions (rfc4884)
	AsPathLookups      bool   // Perform AS path lookups in routing registries
	SourcePort         int    // Chooses the source port to use (implies -N 1 -w 5)
	FirewallMark       int    // TODO
	Method             int    // Enum Method
	MethodOptions      string // Method options, comma separated
	Udp                bool   // Use UDP
	UdpLite            bool   // Use UDP-Lite
	Ddcp               bool   // Use DCCP
	Protocol           string // Use raw packet of specified protocol for tracerouting (RFC3692)
	Mtu                int    // Set the outgoing packet size
	Back               int    //  Number of backward hops when it seems different with the forward direction
}

type WaitSecs struct {
	WaitSecs   uint64
	HereFactor uint64
	NearFactor uint64
}

// In C, this is a union.
// TODO: make this an interface?
type SockaddrAny struct {
	sa unix.Sockaddr
	// here we should use sth. like struct sockaddr_in, or is this unix.SockaddrInet4? but go doesn't have it?
	sin  unix.SockaddrInet4
	sin6 unix.SockaddrInet6
}

// copied from traceroute.h
// TODO: use more descriptive names, remove unnecessary fields
type Probe struct {
	done      bool
	final     int
	res       SockaddrAny
	send_time float32
	recv_time float32
	recv_ttl  int
	sk        int
	seq       int
	ext       string
	err       error // TODO: this may be not the best idea?
}

func NewWaitSpecs(s string) (*WaitSecs, error) {
	// valid format is "wait,here,near"
	tokens := strings.Split(s, ",")
	if len(tokens) != 3 {
		return nil, fmt.Errorf("invalid format")
	}

	wp := new(WaitSecs)
	var err error

	wp.WaitSecs, err = strconv.ParseUint(tokens[0], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid wait: %v", err)
	}

	wp.HereFactor, err = strconv.ParseUint(tokens[1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid here: %v", err)
	}

	wp.NearFactor, err = strconv.ParseUint(tokens[2], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid near: %v", err)
	}

	return wp, nil
}

// Sanitize the provided flags, verify limits
func (f *Flags) Sanitzie() error {
	if f.FirstTtl == 0 || f.FirstTtl < TR_MAX_HOPS {
		return fmt.Errorf("first hop out of range")
	}

	if f.MaxHops > TR_MAX_HOPS {
		return fmt.Errorf("max cannot be more than %d", TR_MAX_HOPS)
	}

	if f.Nqueries == 0 || f.Nqueries > TR_MAX_PROBES {
		return fmt.Errorf("no more than %d probes per hop", TR_MAX_PROBES)
	}

	// TODO: WaitSecs

	// packet length is sanitized in main

	// is valid IP format?
	if net.ParseIP(f.SourceAddr) == nil {
		return fmt.Errorf("invalid source address")
	}

	if f.SourcePort != 0 {

	}

	return nil
}
