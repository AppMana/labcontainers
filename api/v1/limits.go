package labcontainersv1

import "github.com/appmana/labcontainers/internal/transferlimits"

const (
	// MaxPutBytes is the guest upload contract. MaxMessageBytes includes room
	// for protobuf path/mode framing around that payload.
	MaxPutBytes            = transferlimits.Upload
	MaxGuestExecStdinBytes = transferlimits.GuestExecStdin
	MaxMessageBytes        = MaxPutBytes + (1 << 20)
)
