// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// https://github.com/mirror/busybox/blob/master/networking/traceroute.c
package traceroute

// Const Maximums. taken from original implementation
const (
	TR_MAX_HOPS       = 255
	TR_MAX_PROBES     = 10
	TR_MAX_GATEWAYS_4 = 8
	TR_MAX_GATEWAYS_6 = 127
	TR_MAX_PACKET_LEN = 65000
)

// Const Defaults
const (
	TR_DEFAULT_START_PORT          = 33434
	TR_DEFAULT_UDP_PORT            = 53
	TR_DEFAULT_TCP_PORT            = 80
	TR_DEFAULT_DCCP_PORT           = TR_DEFAULT_START_PORT
	TR_DEFAULT_SIM_QUERIES         = 30
	TR_DEFAULT_NQUERIES            = 3
	TR_DEFAULT_PROBE_INTERVAL_WAIT = 0
	TR_DEFAULT_RAW_PORT            = 253 // see RFC3692
)

// Enum Type of Service (TOS)
const (
	TR_TOS_LOWDELAY   = 0x10
	TR_TOS_THROUGHPUT = 0x08
)

// Enum Method
const (
	TR_METHOD_DEFAULT = iota
	TR_METHOD_ICMP
	TR_METHOD_TCP
	TR_METHOD_UDP
	TR_METHOD_UDPL
)

// Enum IP
const (
	TR_IP_4 = iota
	TR_IP_6
)

// Canonical struct for the traceroute config
// There are mutually exclusive options that have to resolved here
type TracerouteConfig struct {
	ip_type     int
	method_type int
}
