// +build !windows

package util

var (
	listeningTCPPortsRegext = `^tcp\s.+\s[\d.]+:(\d+)\s.+$`
	netstatCmd              = "netstat"
	netstatCmdArgs          = []string{"-nlt"}
)
