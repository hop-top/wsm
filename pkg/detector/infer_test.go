package detector

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"hop.top/wsm/pkg/model"
)

func TestInferSpaceFromBoundary_HTTP(t *testing.T) {
	inf := InferSpaceFromBoundary(context.Background(), "https://api.example.com")
	if inf.Type != model.SpaceAPI {
		t.Fatalf("expected api, got %s", inf.Type)
	}
	if inf.TrustLevel != model.TrustExternal {
		t.Fatalf("expected external, got %s", inf.TrustLevel)
	}
}

func TestInferSpaceFromBoundary_Docker(t *testing.T) {
	inf := InferSpaceFromBoundary(context.Background(), "docker://myapp")
	if inf.Type != model.SpaceContainer {
		t.Fatalf("expected container, got %s", inf.Type)
	}
}

func TestInferSpaceFromBoundary_SSH(t *testing.T) {
	inf := InferSpaceFromBoundary(context.Background(), "ssh://server.example.com")
	if inf.Type != model.SpaceRemote {
		t.Fatalf("expected remote, got %s", inf.Type)
	}
}

func TestInferSpaceFromBoundary_Database(t *testing.T) {
	inf := InferSpaceFromBoundary(context.Background(), "postgres://localhost/mydb")
	if inf.Type != model.SpaceDatabase {
		t.Fatalf("expected database, got %s", inf.Type)
	}
}

func TestInferSpaceFromBoundary_FilesystemDefault(t *testing.T) {
	dir := t.TempDir()
	inf := InferSpaceFromBoundary(context.Background(), dir)
	if inf.Type != model.SpaceFilesystem {
		t.Fatalf("expected filesystem, got %s", inf.Type)
	}
	if inf.TrustLevel != model.TrustLocal {
		t.Fatalf("expected local, got %s", inf.TrustLevel)
	}
}

func TestInferSpaceFromBoundary_DockerCompose(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("version: '3'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inf := InferSpaceFromBoundary(context.Background(), dir)
	if inf.Type != model.SpaceContainer {
		t.Fatalf("expected container from docker-compose, got %s", inf.Type)
	}
}

func TestInferSpaceFromBoundary_PackageJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	inf := InferSpaceFromBoundary(context.Background(), dir)
	if inf.Type != model.SpaceFilesystem {
		t.Fatalf("expected filesystem (package.json doesn't change type), got %s", inf.Type)
	}
	found := false
	for _, c := range inf.Capabilities {
		if c == "package-json" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected package-json in capabilities, got %v", inf.Capabilities)
	}
}
