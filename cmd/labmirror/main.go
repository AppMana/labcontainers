// labmirror serves a staged directory tree as the HTTPS hosts it is named
// after. See pkg/mirror.
package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/appmana/labcontainers/pkg/mirror"
)

func main() {
	root := flag.String("root", "", "directory whose subdirectories are the host names to serve")
	state := flag.String("state", "", "directory holding the mirror's certificate authority")
	listen := flag.String("listen", ":443", "HTTPS listen address")
	crlListen := flag.String("crl-listen", ":80", "HTTP listen address for the revocation list")
	crlURL := flag.String("crl-url", "", "URL guests fetch the revocation list from")
	flag.Parse()
	if *root == "" || *state == "" || *crlURL == "" {
		fmt.Fprintln(os.Stderr, "labmirror: --root, --state and --crl-url are required")
		os.Exit(2)
	}
	if err := run(*root, *state, *listen, *crlListen, *crlURL); err != nil {
		fmt.Fprintln(os.Stderr, "labmirror:", err)
		os.Exit(1)
	}
}

func run(root, state, listen, crlListen, crlURL string) error {
	authority, err := mirror.LoadAuthority(state, time.Now())
	if err != nil {
		return err
	}
	tlsListener, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}
	crlListener, err := net.Listen("tcp", crlListen)
	if err != nil {
		return err
	}
	return mirror.Serve(root, authority, crlURL, tlsListener, crlListener, os.Stdout)
}
