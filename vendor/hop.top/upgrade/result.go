package upgrade

import "time"

// Result holds the outcome of a version check.
type Result struct {
	Current     string
	Latest      string
	URL         string    // download URL for the latest asset
	Notes       string    // release notes / changelog excerpt
	CheckedAt   time.Time
	UpdateAvail bool
	Err         error
}
