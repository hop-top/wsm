package owsf

import "testing"

func TestHandoff_OfferAcceptDecline(t *testing.T) {
	offer := NewHandoffOffer("doc-1", "evt-10", false)
	if offer.Type != HandoffOffer {
		t.Fatal("type must be offer")
	}
	if offer.Terminal {
		t.Fatal("default non-terminal")
	}

	accept := AcceptHandoff(offer)
	if accept.Type != HandoffAccept {
		t.Fatal("type must be accept")
	}
	if accept.OfferRef != offer.ID {
		t.Fatal("must ref offer")
	}

	decline := DeclineHandoff(offer)
	if decline.Type != HandoffDecline {
		t.Fatal("type must be decline")
	}
	if decline.OfferRef != offer.ID {
		t.Fatal("must ref offer")
	}
}

func TestHandoff_TerminalStopsAppend(t *testing.T) {
	offer := NewHandoffOffer("doc-1", "evt-10", true)
	if !offer.Terminal {
		t.Fatal("should be terminal")
	}
}

func TestHandoff_ForkLinkage(t *testing.T) {
	parent := NewDocument("wsm", "0.1.0")
	offer := NewHandoffOffer(parent.DocID, "evt-10", false)
	child := HandoffFork(parent, offer)
	if child.Fork == nil {
		t.Fatal("fork required")
	}
	if child.Fork.Kind != "handoff" {
		t.Fatal("fork.kind must be handoff")
	}
	if child.Fork.ParentEventID != offer.ID {
		t.Fatal("must ref offer event")
	}
}
