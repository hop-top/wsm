package device

import (
	"os"
	"testing"
)

func TestCurrentReadOnly_PersistsNothing(t *testing.T) {
	data := t.TempDir() + "/fresh"
	t.Setenv("XDG_DATA_HOME", data)

	d := CurrentReadOnly()
	if d.ID == "" {
		t.Fatal("empty device id")
	}
	if _, err := os.Lstat(data); !os.IsNotExist(err) {
		t.Fatalf("CurrentReadOnly created %s (err: %v)", data, err)
	}
}
