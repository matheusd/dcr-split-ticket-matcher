// +build !windows

package net

var (
	listeningTCPPortsRegext = `^tcp\s.+\s[\d.]+:(\d+)\s.+$`
	netstatCmd              = "netstat"
	netstatCmdArgs          = []string{"-nlt"}
)
