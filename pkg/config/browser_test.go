package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBrowserProfile_Validate(t *testing.T) {
	valid := BrowserProfile{
		ProfileDir: "/tmp/test-browser",
		CDPPort:    9333,
		Channel:    BrowserBrave,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid profile should pass: %v", err)
	}
}

func TestBrowserProfile_Validate_MissingDir(t *testing.T) {
	bp := BrowserProfile{CDPPort: 9333, Channel: BrowserChrome}
	if err := bp.Validate(); err == nil {
		t.Fatal("expected error for missing profile_dir")
	}
}

func TestBrowserProfile_Validate_BadPort(t *testing.T) {
	for _, port := range []int{0, 80, 1023, 65536} {
		bp := BrowserProfile{
			ProfileDir: "/tmp/x",
			CDPPort:    port,
			Channel:    BrowserChrome,
		}
		if err := bp.Validate(); err == nil {
			t.Errorf("expected error for port %d", port)
		}
	}
}

func TestBrowserProfile_Validate_BadChannel(t *testing.T) {
	bp := BrowserProfile{
		ProfileDir: "/tmp/x",
		CDPPort:    9333,
		Channel:    "firefox",
	}
	if err := bp.Validate(); err == nil {
		t.Fatal("expected error for invalid channel")
	}
}

func TestBrowserProfile_Provision_Teardown(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "browser-profile")
	bp := BrowserProfile{
		ProfileDir: dir,
		CDPPort:    9334,
		Channel:    BrowserBrave,
	}

	absDir, err := bp.Provision()
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}

	info, err := os.Stat(absDir)
	if err != nil {
		t.Fatalf("profile dir should exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected directory")
	}
	// Check permissions (owner-only).
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("expected 0700, got %o", perm)
	}

	if err := bp.Teardown(); err != nil {
		t.Fatalf("Teardown: %v", err)
	}
	if _, err := os.Stat(absDir); !os.IsNotExist(err) {
		t.Error("expected dir removed after teardown")
	}
}

func TestBrowserProfile_EnvVars(t *testing.T) {
	bp := BrowserProfile{
		ProfileDir: "/data/browser",
		CDPPort:    9335,
		Channel:    BrowserArc,
	}
	env := bp.EnvVars()

	if env["BROWSER_CHANNEL"] != "arc" {
		t.Errorf("channel: got %q", env["BROWSER_CHANNEL"])
	}
	if env["BROWSER_HEADLESS"] != "true" {
		t.Errorf("headless: got %q", env["BROWSER_HEADLESS"])
	}
	if env["BROWSER_ARGS"] == "" {
		t.Error("expected BROWSER_ARGS to be set")
	}
}

func TestValidateBrowserChannel(t *testing.T) {
	for _, c := range []BrowserChannel{BrowserBrave, BrowserChrome, BrowserArc, BrowserEdge} {
		if err := ValidateBrowserChannel(c); err != nil {
			t.Errorf("expected %q valid: %v", c, err)
		}
	}
	if err := ValidateBrowserChannel("safari"); err == nil {
		t.Error("expected error for safari")
	}
}
