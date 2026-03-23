package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hop.top/upgrade"
)

func TestUpgrade_NoUpdate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
			"version": "0.0.0",
			"url":     "http://example.com/wsm",
		})
	}))
	defer srv.Close()

	c := upgrade.New(
		upgrade.WithBinary("wsm", "99.0.0"),
		upgrade.WithReleaseURL(srv.URL),
		upgrade.WithStateDir(t.TempDir()),
	)

	var out strings.Builder
	err := upgrade.RunCLI(context.Background(), c, upgrade.CLIOptions{
		Quiet: true,
		Out:   &out,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestUpgrade_UpdateAvail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
			"version": "99.0.0",
			"url":     "http://example.com/wsm",
			"notes":   "Many improvements",
		})
	}))
	defer srv.Close()

	c := upgrade.New(
		upgrade.WithBinary("wsm", "1.0.0"),
		upgrade.WithReleaseURL(srv.URL),
		upgrade.WithStateDir(t.TempDir()),
	)

	r := c.Check(context.Background())
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	if !r.UpdateAvail {
		t.Error("expected update available")
	}
}
