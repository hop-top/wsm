package upgrade

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// extractTarGz extracts the binary named binaryName from a .tar.gz stream
// and writes it to w.
func extractTarGz(r io.Reader, binaryName string, w io.Writer) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("upgrade: gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("upgrade: tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		base := filepath.Base(hdr.Name)
		if strings.EqualFold(base, binaryName) || strings.EqualFold(base, binaryName+".exe") {
			_, err = io.Copy(w, tr) //nolint:gosec // size bounded by asset
			return err
		}
	}
	return fmt.Errorf("upgrade: binary %q not found in archive", binaryName)
}
