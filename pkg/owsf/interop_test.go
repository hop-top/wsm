package owsf

import (
	"testing"

	"hop.top/wsm/pkg/model"
)

func TestValidateAgentIdentity(t *testing.T) {
	tests := []struct {
		id      string
		wantErr bool
	}{
		{"oasf:agent:cursor", false},
		{"oasf:agent:claude-code", false},
		{"", true},
		{"cursor", true},
		{"agent:123", true},
	}

	for _, tt := range tests {
		err := ValidateAgentIdentity(tt.id)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateAgentIdentity(%q) error = %v, wantErr %v", tt.id, err, tt.wantErr)
		}
	}
}

func TestValidateSpaceCapabilities(t *testing.T) {
	tests := []struct {
		caps    []string
		wantErr bool
	}{
		{[]string{"oasf:cap:fs.read", "oasf:cap:fs.write"}, false},
		{[]string{"oasf:cap:net.egress"}, false},
		{[]string{"read"}, true},
		{[]string{"oasf:cap:fs.read", "admin"}, true},
	}

	for _, tt := range tests {
		err := ValidateSpaceCapabilities(tt.caps)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateSpaceCapabilities(%v) error = %v, wantErr %v", tt.caps, err, tt.wantErr)
		}
	}
}

func TestAlignEventToOCSF(t *testing.T) {
	tests := []struct {
		name    string
		ev      model.Event
		wantErr bool
	}{
		{
			name: "valid message",
			ev: model.Event{
				Type:     model.EventInteractionMessage,
				WriterID: "oasf:agent:user",
			},
			wantErr: false,
		},
		{
			name: "invalid message (no writer)",
			ev: model.Event{
				Type: model.EventInteractionMessage,
			},
			wantErr: true,
		},
		{
			name: "valid tool call",
			ev: model.Event{
				Type:    model.EventInteractionToolCall,
				SpaceID: "sp-1",
			},
			wantErr: false,
		},
		{
			name: "invalid tool call (no space)",
			ev: model.Event{
				Type: model.EventInteractionToolCall,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := AlignEventToOCSF(tt.ev)
			if (err != nil) != tt.wantErr {
				t.Errorf("AlignEventToOCSF() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMapBoundaryToObservables(t *testing.T) {
	tests := []struct {
		name     string
		boundary string
		wantType model.ObservableType
	}{
		{"file path", "/home/user", model.ObservableFile},
		{"relative path", "./data", model.ObservableFile},
		{"URL", "https://api.example.com", model.ObservableURL},
		{"Custom Protocol", "docker://app", model.ObservableURL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &model.Space{Boundary: tt.boundary}
			MapBoundaryToObservables(s)
			if len(s.Observables) == 0 {
				t.Fatal("expected observables to be populated")
			}
			if s.Observables[0].Type != tt.wantType {
				t.Errorf("got type %v, want %v", s.Observables[0].Type, tt.wantType)
			}
			if s.Observables[0].Value != tt.boundary {
				t.Errorf("got value %v, want %v", s.Observables[0].Value, tt.boundary)
			}
		})
	}
}
