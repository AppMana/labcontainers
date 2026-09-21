package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/appmana/labcontainers/internal/engine"
	serverpkg "github.com/appmana/labcontainers/internal/server"
	"github.com/appmana/labcontainers/internal/session"
	"github.com/appmana/labcontainers/pkg/capi"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var err error
	switch os.Args[1] {
	case "doctor":
		err = engine.NewContainerlab().Doctor(ctx)
		if err == nil {
			fmt.Printf("Containerlab %s is supported\n", engine.SupportedContainerlabVersion)
		}
	case "sessions":
		err = listSessions()
	case "cleanup":
		err = cleanup(ctx)
	case "capi-crds":
		_, err = os.Stdout.Write(capi.CRDs)
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "labctl:", err)
		os.Exit(1)
	}
}

func usage() { fmt.Fprintln(os.Stderr, "usage: labctl <doctor|sessions|cleanup|capi-crds>") }

func openStore() (*session.Store, error) { return session.NewStore("") }

func listSessions() error {
	store, err := openStore()
	if err != nil {
		return err
	}
	records, err := store.List()
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(records)
}

func cleanup(ctx context.Context) error {
	store, err := openStore()
	if err != nil {
		return err
	}
	return serverpkg.New(store, engine.NewContainerlab()).Scavenge(ctx, time.Now().UTC())
}
