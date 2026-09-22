package labcontainersv1

import "testing"

func TestMessageLimitCoversPutContractAndFraming(t *testing.T) {
	if MaxPutBytes != 256<<20 {
		t.Fatalf("MaxPutBytes = %d", MaxPutBytes)
	}
	if MaxMessageBytes <= MaxPutBytes {
		t.Fatalf("MaxMessageBytes = %d, must include protobuf framing", MaxMessageBytes)
	}
}
