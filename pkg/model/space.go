package model

import "time"

type SpaceType string

const (
	SpaceFilesystem SpaceType = "filesystem"
	SpaceContainer  SpaceType = "container"
	SpaceVM         SpaceType = "vm"
	SpaceAPI        SpaceType = "api"
	SpaceDatabase   SpaceType = "database"
	SpaceMachine    SpaceType = "machine"
	SpaceRemote     SpaceType = "remote"
)

type TrustLevel string

const (
	TrustLocal     TrustLevel = "local"
	TrustVerified  TrustLevel = "verified"
	TrustExternal  TrustLevel = "external"
	TrustUntrusted TrustLevel = "untrusted"
)

// ObservableType maps to OCSF Observable Type IDs.
type ObservableType string

const (
	ObservableFile      ObservableType = "file"      // File Path
	ObservableProcess   ObservableType = "process"   // Process ID (PID)
	ObservableContainer ObservableType = "container" // Container ID
	ObservableURL       ObservableType = "url"       // API Endpoint / URL
)

// Observable represents an OCSF Observable.
type Observable struct {
	Type  ObservableType `json:"type"`
	Value string         `json:"value"`
}

type Space struct {
	ID           string       `json:"id"`
	WriterID     string       `json:"writer_id,omitempty"` // OASF Agent Identity link
	Type         SpaceType    `json:"type"`
	Parent       string       `json:"parent"`
	Boundary     string       `json:"boundary"`              // Primary boundary identifier
	Observables  []Observable `json:"observables,omitempty"` // OCSF Observables
	Capabilities []string     `json:"capabilities"`          // OASF Capability Taxonomy refs
	TrustLevel   TrustLevel   `json:"trust_level"`
	CreatedAt    time.Time    `json:"created_at"`
}
