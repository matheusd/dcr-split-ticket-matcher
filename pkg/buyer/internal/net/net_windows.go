// +build windows

package net

var (
	listeningTCPPortsRegext = `^\s+TCP\s+[\d.]+:(\d+)\s.+\sLISTENING.*$`
	netstatCmd              = "netstat"
	netstatCmdArgs          = []string{"-an"}
)
