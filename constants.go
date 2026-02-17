package main

import "time"

// Protocol constants for nameserver configuration.
const (
	protocolUDP = "udp"
	protocolTCP = "tcp"
	protocolDOT = "dot"
	protocolDOH = "doh"
)

// DNS check timeout constant
const dnsCheckTimeout = 5 * time.Second
