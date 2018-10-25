// +build openbsd

package net

var (
	listeningTCPPortsRegext = `^tcp\s.+\s[\d.]+\.(\d+)\s.+$`
	netstatCmd              = "/usr/bin/netstat"
	netstatCmdArgs          = []string{"-nlt", "-p", "tcp"}
)
