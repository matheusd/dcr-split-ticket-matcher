// +build darwin

package net

var (
	listeningTCPPortsRegext = `^tcp4\s.+\d+\.(\d+)\s.+LISTEN.*$`
	netstatCmd              = "netstat"
	netstatCmdArgs          = []string{"-an"}
)
