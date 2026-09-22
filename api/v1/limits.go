package labcontainersv1

const (
	// MaxPutBytes is the guest upload contract. MaxMessageBytes includes room
	// for protobuf path/mode framing around that payload.
	MaxPutBytes     = 256 << 20
	MaxMessageBytes = MaxPutBytes + (1 << 20)
)
