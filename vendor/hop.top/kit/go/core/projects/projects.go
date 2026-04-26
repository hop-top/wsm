package projects

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
	"gopkg.in/yaml.v3"

	"hop.top/kit/go/core/xdg"
)

// ErrMalformed is returned by Read when projects.yaml exists but its
// content is not valid YAML or does not match the expected shape.
var ErrMalformed = errors.New("projects: malformed file")

// ErrSchemaUnsupported is returned by Read when projects.yaml has a
// schema version greater than the SchemaVersion this build understands.
var ErrSchemaUnsupported = errors.New("projects: schema version unsupported")

// fileMode and dirMode are the permissions used when creating projects.yaml
// and its parent directory. They match kit/go/core/identity defaults for
// non-secret config files.
const (
	fileMode os.FileMode = 0o644
	dirMode  os.FileMode = 0o750
)

// DefaultPath returns the canonical projects.yaml path for the current
// user. It is filepath.Join(xdg.ConfigDir("rux"), "projects.yaml") and
// has no filesystem side effects. On macOS without XDG_CONFIG_HOME this
// resolves under ~/Library/Application Support/rux/. See doc.go for the
// full resolution table.
func DefaultPath() (string, error) {
	dir, err := xdg.ConfigDir("rux")
	if err != nil {
		return "", fmt.Errorf("projects: resolve config dir: %w", err)
	}
	return filepath.Join(dir, "projects.yaml"), nil
}

// Read loads the projects file from DefaultPath. Returns:
//
//   - empty File and nil if the file does not exist;
//   - empty File and ErrMalformed if YAML decoding fails;
//   - empty File and ErrSchemaUnsupported if Schema > SchemaVersion;
//   - the parsed File otherwise.
//
// Per kit core convention, this function never logs. Callers branch on
// the sentinels via errors.Is and decide their own log behavior.
func Read() (File, error) {
	path, err := DefaultPath()
	if err != nil {
		return File{}, err
	}
	return readFile(path)
}

// readFile is the path-injectable core of Read, also used internally by
// Write/Delete which already hold the path.
func readFile(path string) (File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return File{}, nil
		}
		return File{}, fmt.Errorf("projects: read file: %w", err)
	}

	var file File
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return File{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	if file.Schema > SchemaVersion {
		return File{}, fmt.Errorf("%w: file schema=%d, build supports %d",
			ErrSchemaUnsupported, file.Schema, SchemaVersion)
	}
	return file, nil
}

// Write upserts entry under name, preserving every other entry verbatim
// regardless of its Source. If entry.Source is empty it defaults to
// SourceWSM.
//
// Concurrency: Write acquires an exclusive flock on a sidecar
// "<path>.lock" for the full read-modify-write cycle and publishes the
// new file via temp+rename (atomic on POSIX).
func Write(name string, entry Entry) error {
	if entry.Source == "" {
		entry.Source = SourceWSM
	}

	path, err := DefaultPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return fmt.Errorf("projects: mkdir parent: %w", err)
	}

	unlock, err := acquireLock(path)
	if err != nil {
		return err
	}
	defer unlock()

	file, err := readFile(path)
	if err != nil && !errors.Is(err, ErrMalformed) && !errors.Is(err, ErrSchemaUnsupported) {
		return err
	}
	if file.Projects == nil {
		file.Projects = make(map[string]Entry)
	}
	file.Schema = SchemaVersion
	file.Projects[name] = entry

	return writeFile(path, file)
}

// Delete removes name from the registry, preserving every other entry.
// A missing key is not an error. Uses the same flock as Write.
func Delete(name string) error {
	path, err := DefaultPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return fmt.Errorf("projects: mkdir parent: %w", err)
	}

	unlock, err := acquireLock(path)
	if err != nil {
		return err
	}
	defer unlock()

	file, err := readFile(path)
	if err != nil && !errors.Is(err, ErrMalformed) && !errors.Is(err, ErrSchemaUnsupported) {
		return err
	}
	if file.Projects == nil {
		// Nothing to delete and nothing to persist; treat as no-op.
		return nil
	}
	if _, ok := file.Projects[name]; !ok {
		return nil
	}
	delete(file.Projects, name)
	file.Schema = SchemaVersion

	return writeFile(path, file)
}

// acquireLock takes an exclusive flock on "<path>.lock" and returns an
// unlock closure suitable for defer. Failure to obtain the lock returns
// an error with the projects: prefix.
func acquireLock(path string) (func(), error) {
	lock := flock.New(path + ".lock")
	if err := lock.Lock(); err != nil {
		return nil, fmt.Errorf("projects: acquire lock: %w", err)
	}
	return func() { _ = lock.Unlock() }, nil
}

// writeFile marshals file to YAML and writes via temp+rename, matching
// the atomicWrite pattern in kit/go/core/identity.
func writeFile(path string, file File) error {
	data, err := yaml.Marshal(file)
	if err != nil {
		return fmt.Errorf("projects: marshal: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, fileMode); err != nil {
		return fmt.Errorf("projects: write temp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("projects: rename: %w", err)
	}
	return nil
}
