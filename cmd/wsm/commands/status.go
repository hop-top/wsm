package commands

import (
	"context"

	"hop.top/kit/go/console/cli"
	"hop.top/wsm/pkg/config"
)

// wsmStatusData is the payload of the `wsm` status section: the
// resolved state the process would actually read and write with.
type wsmStatusData struct {
	Backend         string `json:"backend"          yaml:"backend"`
	DataDir         string `json:"data_dir"         yaml:"data_dir"`
	CacheDir        string `json:"cache_dir"        yaml:"cache_dir"`
	StateDir        string `json:"state_dir"        yaml:"state_dir"`
	BackendURL      string `json:"backend_url,omitempty" yaml:"backend_url,omitempty"`
	ConfigFile      string `json:"config_file"      yaml:"config_file"`
	ConfigFileFound bool   `json:"config_file_found" yaml:"config_file_found"`
	ConfigSource    string `json:"config_source"    yaml:"config_source"`
}

// wsmStatusProvider reports the resolved wsm configuration as a status
// section.
//
// Why this exists alongside kit's effective-config section: that
// section renders raw flag values, so an unset --backend / --data-dir
// reads as "" even though the process resolved real values from the
// config chain. The state wsm would write with — which backend, which
// data directory, which config file won — is exactly the implicit
// state F8 requires be inspectable, so it is resolved through
// config.Load (the same loader the commands use) rather than read off
// the flag set.
func wsmStatusProvider(_ context.Context) (cli.StatusSection, error) {
	cfg, err := config.Load(config.LoadOptions{})
	if err != nil {
		// A broken config is itself reportable state: surface it as an
		// error section rather than failing the whole status report.
		return cli.StatusSection{
			Title:        "wsm",
			Status:       "error",
			ErrorMessage: err.Error(),
			Priority:     1000,
		}, nil
	}
	if cfg.Backend == "" {
		// Mirrors newAppContext: an unset backend resolves to sqlite,
		// so status must report what the next command would use.
		cfg.Backend = "sqlite"
	}
	return cli.StatusSection{
		Title:  "wsm",
		Status: "ok",
		Data: wsmStatusData{
			Backend:         cfg.Backend,
			DataDir:         cfg.DataDir,
			CacheDir:        cfg.CacheDir,
			StateDir:        cfg.StateDir,
			BackendURL:      cfg.BackendURL,
			ConfigFile:      cfg.ConfigFile,
			ConfigFileFound: cfg.ConfigFileFound,
			ConfigSource:    cfg.Source,
		},
		// Adopter band starts at 1000 so kit's own sections keep their
		// relative order.
		Priority: 1000,
	}, nil
}
