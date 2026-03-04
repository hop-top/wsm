package owsf

import "github.com/oklog/ulid/v2"

type HandoffType string

const (
	HandoffOffer   HandoffType = "control.handoff.offer"
	HandoffAccept  HandoffType = "control.handoff.accept"
	HandoffDecline HandoffType = "control.handoff.decline"
)

type HandoffEvent struct {
	ID       string      `json:"id"`
	Type     HandoffType `json:"type"`
	DocID    string      `json:"doc_id"`
	EventRef string      `json:"event_ref"`
	OfferRef string      `json:"offer_ref,omitempty"`
	Terminal bool        `json:"terminal"`
}

func NewHandoffOffer(docID, eventRef string, terminal bool) HandoffEvent {
	return HandoffEvent{
		ID:       ulid.Make().String(),
		Type:     HandoffOffer,
		DocID:    docID,
		EventRef: eventRef,
		Terminal: terminal,
	}
}

func AcceptHandoff(offer HandoffEvent) HandoffEvent {
	return HandoffEvent{
		ID:       ulid.Make().String(),
		Type:     HandoffAccept,
		DocID:    offer.DocID,
		OfferRef: offer.ID,
	}
}

func DeclineHandoff(offer HandoffEvent) HandoffEvent {
	return HandoffEvent{
		ID:       ulid.Make().String(),
		Type:     HandoffDecline,
		DocID:    offer.DocID,
		OfferRef: offer.ID,
	}
}

func HandoffFork(parent Document, offer HandoffEvent) Document {
	child := NewDocument(parent.Producer.Name, parent.Producer.Version)
	child.Fork = &ForkRef{
		ParentDocID:   parent.DocID,
		ParentEventID: offer.ID,
		Kind:          "handoff",
	}
	return child
}
