package owsf

import (
	"time"

	"github.com/oklog/ulid/v2"
)

const OWSFCoreVersion = "0.1"

type Producer struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type ForkRef struct {
	ParentDocID   string `json:"parent_doc_id"`
	ParentEventID string `json:"parent_event_id"`
	Kind          string `json:"kind,omitempty"`
}

type Document struct {
	OWSFVersion string    `json:"owsf_version"`
	DocID       string    `json:"doc_id"`
	CreatedAt   time.Time `json:"created_at"`
	Producer    Producer  `json:"producer"`
	Fork        *ForkRef  `json:"fork,omitempty"`
	Writers     []Writer  `json:"writers,omitempty"`
	InputRefs   []string  `json:"input_refs,omitempty"`
}

func NewDocument(producerName, producerVersion string) Document {
	return Document{
		OWSFVersion: OWSFCoreVersion,
		DocID:       ulid.Make().String(),
		CreatedAt:   time.Now().UTC(),
		Producer: Producer{
			Name:    producerName,
			Version: producerVersion,
		},
	}
}
