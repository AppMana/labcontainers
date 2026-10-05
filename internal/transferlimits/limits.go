// Package transferlimits defines payload bounds shared by the RPC and guest
// transports. Oversized payloads must fail, never succeed with partial bytes.
package transferlimits

const (
	// QGAMessage is QEMU's bound on one guest agent message: its JSON
	// streamer refuses a message whose tokens total more than 64 MiB
	// (MAX_TOKEN_SIZE), reporting "JSON token size limit exceeded".
	QGAMessage = 64 << 20
	// GuestExecFraming is the part of a guest-exec message reserved for
	// the command, its arguments and the JSON around them.
	GuestExecFraming = 1 << 20
	// GuestExecStdin is the largest stdin a guest-exec message carries:
	// it travels base64-encoded, four bytes for every three, inside one
	// message with the framing.
	GuestExecStdin = (QGAMessage - GuestExecFraming) / 4 * 3
	Upload         = 256 << 20
)
