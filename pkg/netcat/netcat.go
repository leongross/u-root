// Copyright 2012-2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package netcat

import "strings"

// Default constants that are not covered by flags
const (
	DEFAULT_PORT = 31337
)

type IPType int

const (
	IP_TYPE_V4 = iota
	IP_TYPE_V6
)

type SocketType int

const (
	SOCKET_TYPE_TCP = iota
	SOCKET_TYPE_UDP
	SOCKET_TYPE_UNIX
	SOCKET_TYPE_VSOCK
)

// TODO: should we do this via enabled or can we just pass nil?
// NOTE: The key files have to be of PEM format

type ProxyType int

const (
	PROXY_TYPE_NONE = iota
	PROXY_TYPE_HTTP
	PROXY_TYPE_SOCKS4
	PROXY_TYPE_SOCKS5
)

func (p ProxyType) String() string {
	return [...]string{"None", "HTTP", "SOCKS4", "SOCKS5"}[p]
}

func ProxyTypeFromString(s string) ProxyType {
	switch strings.ToUpper(s) {
	case "HTTP":
		return PROXY_TYPE_HTTP
	case "SOCKS4":
		return PROXY_TYPE_SOCKS4
	case "SOCKS5":
		return PROXY_TYPE_SOCKS5
	default:
		return PROXY_TYPE_NONE
	}
}

type NetcatConnectTypeOptions struct {
	LooseSourceRouterPoints []string
}

type ProxyAuthType int

const (
	PROXY_AUTH_NONE = iota
	PROXY_AUTH_HTTP
	PROXY_AUTH_SOCKS5
)

func ProxyAuthTypeFromString(s string) ProxyAuthType {
	switch strings.ToUpper(s) {
	case "HTTP":
		return PROXY_AUTH_HTTP
	case "SOCKS5":
		return PROXY_AUTH_SOCKS5
	default:
		return PROXY_AUTH_NONE
	}
}

type NetcatProxyConfig struct {
	Type       ProxyType // If this is none, discard the entire Proxy handling
	Address    string
	DNSAddress string
	Port       uint
	AuthType   ProxyAuthType // If this is none, discard the entire ProxyAuth handling
}

type NetcatSSLConfig struct {
	Enabled       bool
	CertFilePath  string // Path to the certificate file in PEM format
	KeyFilePath   string // Path to the private key file in PEM format
	VerifyTrust   bool
	TrustFilePath string   //  Verify trust and domain name of certificates
	Ciphers       []string // List of ciphersuites that Ncat will use when connecting to servers or when accepting SSL connections from clients
	SNI           string   // (Server Name Indication) Tell the server the name of the logical server Ncat is contacting
	ALPN          []string // List of protocols to send via the Application-Layer Protocol Negotiation
}

type NetcatConnectionMode int

// Ncat operates in one of two primary modes: connect mode and listen mode.
const (
	CONNECTION_MODE_CONNECT = iota
	CONNECTION_MODE_LISTEN
)

type NetcatConfig struct {
	ConnectionMode NetcatConnectionMode
	Hostname       string
	Port           string
	IPType         IPType
	SocketType     SocketType
	EOFCharacter   uint8
	SSLConfig      NetcatSSLConfig
	ProxyConfig    NetcatProxyConfig
	Verbose        bool
}
