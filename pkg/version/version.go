package version

import "fmt"

const (
	// ProtocolVersion records protocol-incompatible changes to the matching
	// engine. Buyers and the matching server must be running the same protocol
	// version in order for the matching process to proceed.
	// History of Versions:
	// v1: Initial version
	// v2: Modified algo that calculates the lottery commiment hash to include
	// the amounts and voter addresses
	// v3: Modified pool fee to be proportional by contribution amount
	// v4: Added the session_token return to FindMatchesResponse, which must
	// be sent back on all further requests to validate the access.
	ProtocolVersion = 4
)

// These are the individual version numbers
const (
	Major = 0
	Minor = 7
	Patch = 2
)

// These are pre-release and build metadata vars that can be modified by linking
// with a `-ldflags -X
// github.com/matheusd/dcr-split-ticket-matcher/internal/version.XXXX=bla`
var (
	PreRelease    = "pre"
	BuildMetadata = "dev"
)

// String returns the full software version as a string.
func String() string {
	version := fmt.Sprintf("%d.%d.%d", Major, Minor, Patch)

	if PreRelease != "" {
		version = fmt.Sprintf("%s-%s", version, PreRelease)
	}

	if BuildMetadata != "" {
		version = fmt.Sprintf("%s+%s", version, BuildMetadata)
	}

	return version
}

// Root returns the root version of the app (ie, no pre-release or build
// metadata tags)
func Root() string {
	return fmt.Sprintf("%d.%d.%d", Major, Minor, Patch)
}

// NoMeta returns the vertion of the app without build metadata (but with
// prerelease tag if defined)
func NoMeta() string {
	version := fmt.Sprintf("%d.%d.%d", Major, Minor, Patch)

	if PreRelease != "" {
		version = fmt.Sprintf("%s-%s", version, PreRelease)
	}

	return version
}
