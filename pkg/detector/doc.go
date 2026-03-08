// Package detector provides a plugin interface for scanning workspace spaces.
//
// Detectors inspect a space directory and report what tools, runtimes,
// and configurations are present. Results feed into strategy generation
// and can be customized via per-space or workspace-wide overrides.
package detector
