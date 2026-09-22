package client

import (
	"testing"

	labv1 "github.com/appmana/labcontainers/api/v1"
)

func TestRawTransportAndNodeReference(t *testing.T) {
	stub := labv1.NewLabcontainersClient(nil)
	c := &Client{rpc: stub}
	if c.RPC() != stub {
		t.Fatal("RPC must return the underlying generated client")
	}
	s := &Session{client: c, value: &labv1.Session{Id: "session"}}
	n := s.Node("guest")
	ref := n.Ref()
	if ref.GetSessionId() != "session" || ref.GetNode() != "guest" {
		t.Fatalf("wrong node reference: %v", ref)
	}
	ref.Node = "different"
	if n.Ref().GetNode() != "guest" {
		t.Fatal("caller modified the node handle through its reference")
	}
}
