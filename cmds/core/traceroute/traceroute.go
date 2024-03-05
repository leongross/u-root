// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"

	flag "github.com/spf13/pflag"
	traceroute "github.com/u-root/u-root/pkg/traceroute"
)

var (
	f traceroute.Flags
)

// TODO: Maybe use go flags instead here, since we need case sensitivity (see -n -N)
func init() {
	flag.BoolVarP(&f.Ipv4, "ipv4", "4", false, "Explicitly force IPv4")
	flag.BoolVarP(&f.Ipv6, "ipv6", "6", false, "Explicitly force IPv6")
	flag.BoolVarP(&f.Icmp, "icmp", "I", false, "Use ICMP ECHO for tracerouting")
	flag.BoolVarP(&f.Tcp, "tcp", "T", false, "Use TCP SYN for tracerouting")
	flag.BoolVarP(&f.Debug, "debug", "d", false, "Enable socket level debugging")
	flag.BoolVarP(&f.DontFragment, "dont-fragment", "F", false, "Set the Don't Fragment bit")
	flag.UintVarP(&f.FirstTtl, "first", "f", 1, "Start from the <val> hop (instead from 1)")
	flag.StringVarP(&f.Gateway, "gateway", "g", "", "Use the specified gateway as the next hop")
	flag.StringVarP(&f.Iface, "interface", "i", "", "Use the specified interface")
	flag.IntVarP(&f.MaxHops, "max-hops", "m", traceroute.TR_MAX_HOPS, "Set the max number of hops (max TTL to be used)")
	flag.IntVarP(&f.SimQueries, "sim-queries", "N", traceroute.TR_DEFAULT_SIM_QUERIES, "Number of probes to send to each hop simultaneously")
	// flag.BoolVarP(&f.NoMap, "", "n", false, "Do not map IP addresses to host names when displaying them")
	flag.IntVarP(&f.Port, "port", "p", traceroute.TR_DEFAULT_START_PORT, "Set the base port number used in probes")
	flag.IntVarP(&f.Tos, "tos", "t", 0, "Set the type-of-service in probe packets")
	flag.StringVarP(&f.Flowlabel, "flowlabel", "l", "", "Set the flow label (IPv6 only)")
	flag.StringVarP(&f.Wait, "wait", "w", "", "Set the time (in seconds) to wait for a response to a probe")
	flag.UintVarP(&f.Nqueries, "queries", "q", traceroute.TR_DEFAULT_NQUERIES, "Set the number of probes to send to each hop")
	flag.BoolVarP(&f.BypassRoutingTable, "", "r", false, "Bypass the normal routing table and send directly to a host on an attached network")
	flag.StringVarP(&f.SourceAddr, "source", "s", "", "Use the specified source address")
	flag.IntVarP(&f.SendWait, "sendwait", "z", traceroute.TR_DEFAULT_PROBE_INTERVAL_WAIT, "Minimal time interval between probes")
	flag.BoolVarP(&f.Extensions, "extensions", "e", false, "Show ICMP Extensions (rfc4884)")
	flag.BoolVarP(&f.AsPathLookups, "as-path-lookups", "A", false, "Perform AS path lookups in routing registries")
	flag.IntVarP(&f.SourcePort, "sport", "", nil, "Chooses the source port to use (implies -N 1 -w 5)")
	flag.IntVarP(&f.FirewallMark, "fwmark", "", 0, "  Set the firewall mark for outgoing packets")
	flag.IntVarP(&f.Method, "method", "M", traceroute.TR_METHOD_DEFAULT, "Enum Method")
	flag.StringVarP(&f.MethodOptions, "options", "O", "", "Method options, comma separated")
	flag.BoolVarP(&f.Udp, "udp", "U", false, "Use UDP")
	flag.BoolVarP(&f.UdpLite, "UL", "", false, "Use UDP-Lite")
	flag.BoolVarP(&f.Ddcp, "dccp", "D", false, "Use DCCP")
	flag.StringVarP(&f.Protocol, "protocol", "P", "", "Use raw packet of specified protocol for tracerouting (RFC3692)")
	flag.IntVarP(&f.Mtu, "mtu", "", 0, "Set the outgoing packet size")
	flag.IntVarP(&f.Back, "back", "B", 0, "Number of backward hops when it seems different with the forward direction")
}

func run(args []string, w io.Writer, i *bufio.Reader) error {
	var packetlen uint64
	var err error

	err = f.Sanitzie()
	if err != nil {
		return fmt.Errorf("failed to sanitize flags: %v", err)
	}

	address_domain := args[0]
	// TODO: make sure this does not use cgo
	address_ip, err := net.LookupIP(address_domain)
	if err != nil {
		return fmt.Errorf("failed to resolve %s: %v", address_domain, err)
	}

	ipsfirst := address_ip[0].String()

	if len(args) == 2 {
		packetlen, err = strconv.ParseUint(args[1], 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse packet length: %v", err)
		}

		if packetlen > traceroute.TR_MAX_PACKET_LEN {
			return fmt.Errorf("failed to parse packet length: %v", err)
		}

		// TODO: this is not default behavior
		fmt.Printf("traceroute to %s (%s), %d hops max, %d byte packets\n", address_domain, ipsfirst, f.MaxHops, packetlen)
	} else {
		fmt.Printf("traceroute to %s (%s), %d hops max\n", address_domain, ipsfirst, f.MaxHops)
	}
	return nil
}

func main() {
	// TODO: add feature to check for argv[0] == "traceroute6"
	flag.Parse()
	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	if err := run(flag.Args(), os.Stderr, bufio.NewReader(os.Stdin)); err != nil {
		log.Fatalf("%q", err)
	}
}
