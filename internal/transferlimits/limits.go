// Package transferlimits defines payload bounds shared by the RPC and guest
// transports. Oversized payloads must fail, never succeed with partial bytes.
package transferlimits

const (
	GuestExecStdin = 64 << 20
	Upload         = 256 << 20
)
