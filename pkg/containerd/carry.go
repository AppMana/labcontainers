package containerd

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/appmana/labcontainers/pkg/rig"
)

// CarriedArchive is where CarryArchive stages an image archive on a node.
const CarriedArchive = "/var/tmp/labcontainers-image-import.tar"

// CarryArchive copies an image archive to each node as a file, imports it
// with importArgv (whose final "-" names the archive), and removes the file.
//
// The archive travels as a file, not stdin, because node transports bound a
// single stdin message well below a typical image archive while file
// transfer is chunked. Nodes are served one at a time so a lab host holds a
// single transfer in memory. Which runtime CLI imports is the caller's
// choice: a distribution that embeds its own containerd hides images
// imported into the one on the node's PATH.
func CarryArchive(ctx context.Context, archive io.ReadSeeker, importArgv []string, nodes ...rig.Commands) error {
	if len(importArgv) == 0 || importArgv[len(importArgv)-1] != "-" {
		return fmt.Errorf("image import command %v does not read its archive from \"-\"", importArgv)
	}
	argv := append([]string(nil), importArgv...)
	argv[len(argv)-1] = CarriedArchive
	var errs []error
	for _, node := range nodes {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		if _, err := archive.Seek(0, io.SeekStart); err != nil {
			return errors.Join(append(errs, err)...)
		}
		if err := node.Put(ctx, archive, CarriedArchive, 0o600); err != nil {
			errs = append(errs, fmt.Errorf("carrying an image archive to %s: %w", node.Name(), err))
			continue
		}
		_, err := node.Exec(ctx, argv...)
		if _, rmErr := node.Exec(ctx, "rm", "-f", CarriedArchive); rmErr != nil && err == nil {
			err = rmErr
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("importing an image archive on %s: %w", node.Name(), err))
		}
	}
	return errors.Join(errs...)
}
