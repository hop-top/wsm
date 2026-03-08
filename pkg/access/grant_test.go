package access

import (
	"testing"
	"time"
)

func TestGrant_HasPermission(t *testing.T) {
	g := Grant{Permissions: []Permission{PermRead, PermWrite}}
	if !g.HasPermission(PermRead) {
		t.Error("expected read permission")
	}
	if g.HasPermission(PermAdmin) {
		t.Error("unexpected admin permission")
	}
}

func TestGrant_IsExpired(t *testing.T) {
	t.Run("no expiry", func(t *testing.T) {
		g := Grant{}
		if g.IsExpired() {
			t.Error("grant without expiry should not be expired")
		}
	})

	t.Run("future expiry", func(t *testing.T) {
		future := time.Now().Add(time.Hour)
		g := Grant{ExpiresAt: &future}
		if g.IsExpired() {
			t.Error("grant with future expiry should not be expired")
		}
	})

	t.Run("past expiry", func(t *testing.T) {
		past := time.Now().Add(-time.Hour)
		g := Grant{ExpiresAt: &past}
		if !g.IsExpired() {
			t.Error("grant with past expiry should be expired")
		}
	})
}

func TestGrantSet_AddAndList(t *testing.T) {
	gs := NewGrantSet()
	gs.Add(Grant{ID: "g1", SubjectID: "user1", ResourceID: "ws1"})
	gs.Add(Grant{ID: "g2", SubjectID: "user2", ResourceID: "ws2"})

	list := gs.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 grants, got %d", len(list))
	}
}

func TestGrantSet_Remove(t *testing.T) {
	gs := NewGrantSet()
	gs.Add(Grant{ID: "g1"})
	gs.Remove("g1")

	if len(gs.List()) != 0 {
		t.Error("expected empty set after remove")
	}
}

func TestGrantSet_Check(t *testing.T) {
	gs := NewGrantSet()
	gs.Add(Grant{
		ID:          "g1",
		SubjectID:   "user1",
		ResourceID:  "ws1",
		Permissions: []Permission{PermRead, PermWrite},
		GrantedAt:   time.Now(),
	})

	if !gs.Check("user1", "ws1", PermRead) {
		t.Error("expected read access")
	}
	if !gs.Check("user1", "ws1", PermWrite) {
		t.Error("expected write access")
	}
	if gs.Check("user1", "ws1", PermAdmin) {
		t.Error("unexpected admin access")
	}
	if gs.Check("user2", "ws1", PermRead) {
		t.Error("unexpected access for different subject")
	}
	if gs.Check("user1", "ws2", PermRead) {
		t.Error("unexpected access for different resource")
	}
}

func TestGrantSet_Check_Expired(t *testing.T) {
	gs := NewGrantSet()
	past := time.Now().Add(-time.Hour)
	gs.Add(Grant{
		ID:          "g1",
		SubjectID:   "user1",
		ResourceID:  "ws1",
		Permissions: []Permission{PermRead},
		ExpiresAt:   &past,
	})

	if gs.Check("user1", "ws1", PermRead) {
		t.Error("expired grant should not pass check")
	}
}

func TestGrantSet_AddOverwrite(t *testing.T) {
	gs := NewGrantSet()
	gs.Add(Grant{ID: "g1", SubjectID: "user1", Permissions: []Permission{PermRead}})
	gs.Add(Grant{ID: "g1", SubjectID: "user1", Permissions: []Permission{PermAdmin}})

	list := gs.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 grant after overwrite, got %d", len(list))
	}
	if !list[0].HasPermission(PermAdmin) {
		t.Error("expected overwritten grant to have admin permission")
	}
}
