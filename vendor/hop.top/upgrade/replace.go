package upgrade

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// replaceBinary downloads the asset at assetURL and replaces the running binary.
// Uses an atomic rename: download → tmp file → rename over existing.
func replaceBinary(ctx context.Context, cfg Config, assetURL string) error {
	if assetURL == "" {
		return fmt.Errorf("upgrade: no download URL for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("upgrade: resolve self: %w", err)
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return fmt.Errorf("upgrade: eval symlinks: %w", err)
	}

	// Download to a temp file beside the binary.
	tmpFile, err := os.CreateTemp(filepath.Dir(self), ".upgrade-*")
	if err != nil {
		return fmt.Errorf("upgrade: create temp: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		tmpFile.Close()
		// cleanup on failure; on success file is renamed away
		os.Remove(tmpPath) //nolint:errcheck
	}()

	if err := download(ctx, cfg, assetURL, tmpFile); err != nil {
		return err
	}
	tmpFile.Close()

	// Make executable
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return fmt.Errorf("upgrade: chmod: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, self); err != nil {
		return fmt.Errorf("upgrade: rename: %w", err)
	}
	return nil
}

// download writes the content at url to w.
// Handles plain binary downloads and (future) tar.gz / zip archives.
func download(ctx context.Context, cfg Config, url string, w io.Writer) error {
	client := &http.Client{Timeout: cfg.Timeout * 30} // larger budget for downloads
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("upgrade: download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upgrade: download returned %d", resp.StatusCode)
	}

	name := filepath.Base(url)
	switch {
	case strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tgz"):
		return extractTarGz(resp.Body, cfg.BinaryName, w)
	case strings.HasSuffix(name, ".zip"):
		return fmt.Errorf("upgrade: zip extraction not yet supported; use tar.gz releases")
	default:
		// bare binary
		_, err = io.Copy(w, resp.Body)
		return err
	}
}
