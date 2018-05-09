package pkg

const (
	// ProtocolVersion records protocol-incompatible changes to the matching
	// engine. Buyers and the matching server must be running the same protocol
	// version in order for the matching process to proceed.
	ProtocolVersion = 1

	MajorVersion = 0
	MinorVersion = 4
	PatchVersion = 6
	Version      = "0.4.6"
)
