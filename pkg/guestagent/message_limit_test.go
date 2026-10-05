package guestagent

import (
	"bufio"
	"context"
	"encoding/base64"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/appmana/labcontainers/internal/transferlimits"
)

// QEMU parses each guest agent message under a 64 MiB limit on the
// message's total token bytes, and guest-exec stdin travels inside it
// base64-encoded. A 50 MB image archive piped to "k0s ctr images
// import -" was measured failing with "guest-exec: JSON token size limit
// exceeded" while the advertised stdin limit was 64 MiB. A message the
// guest cannot parse must be refused before it is written.
func TestAMessageTheGuestCannotParseIsNeverWritten(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	written := make(chan int, 1)
	go func() {
		line, _ := bufio.NewReader(server).ReadBytes('\n')
		written <- len(line)
	}()
	a := &Agent{conn: client, reader: bufio.NewReader(client)}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	tooLarge := make([]byte, transferlimits.QGAMessage/4*3+1)
	_, err := a.Execute(ctx, []string{"true"}, tooLarge)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("an unparseable guest-exec message was accepted: %v", err)
	}
	select {
	case n := <-written:
		if n > 0 {
			t.Fatalf("%d bytes reached the guest agent", n)
		}
	case <-time.After(100 * time.Millisecond):
	}
}

// The advertised stdin limit is one the guest can always parse: its
// base64 encoding plus the reserved framing fits one QGA message.
func TestTheStdinLimitFitsOneGuestMessage(t *testing.T) {
	encoded := base64.StdEncoding.EncodedLen(transferlimits.GuestExecStdin)
	if encoded+transferlimits.GuestExecFraming > transferlimits.QGAMessage {
		t.Fatalf("stdin of %d bytes encodes to %d, beyond a %d-byte message with %d of framing",
			transferlimits.GuestExecStdin, encoded, transferlimits.QGAMessage, transferlimits.GuestExecFraming)
	}
	if base64.StdEncoding.EncodedLen(transferlimits.GuestExecStdin+3)+transferlimits.GuestExecFraming <= transferlimits.QGAMessage {
		t.Fatalf("the stdin limit %d leaves usable room unadvertised", transferlimits.GuestExecStdin)
	}
}
