package ergo

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hop.top/wsm/pkg/irc/provider"
)

// apiServer starts an httptest server whose handler answers one path, and
// returns a Client pointed at it.
func apiServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return NewClient(&provider.APIConfig{BaseURL: srv.URL, Token: "token"})
}

func TestSARegisterFresh(t *testing.T) {
	client := apiServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got := r.URL.Path; got != "/v1/ns/saregister" {
			t.Errorf("path = %s", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Errorf("authorization = %q, want a bearer token", got)
		}

		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body["accountName"] != "alice" {
			t.Errorf("accountName = %q", body["accountName"])
		}

		io.WriteString(w, `{"success":true}`)
	})

	if err := client.SARegister(context.Background(), "alice", "secret"); err != nil {
		t.Fatalf("SARegister: %v", err)
	}
}

// TestSARegisterSendsPassphrase pins the field name the passphrase travels
// under, which is the one part of this request whose failure is invisible.
//
// ergo decodes the passphrase from "passphrase". A body naming it anything
// else is not an error: the unknown key is dropped, the account is registered
// and verified with an empty passphrase, and the server answers
// {"success":true}. Every caller therefore believes it provisioned an account,
// and the account cannot be logged into by anyone — including wsm's own bot,
// which makes managed mode fail completely and silently.
//
// Asserting only accountName, as the cases above do, cannot see that: the
// account name is the half that survives a misnamed passphrase.
func TestSARegisterSendsPassphrase(t *testing.T) {
	client := apiServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if got := body["passphrase"]; got != "secret" {
			t.Errorf("passphrase = %q, want the secret under the key ergo decodes; "+
				"a misnamed field registers an account with an empty passphrase "+
				"and still reports success", got)
		}
		io.WriteString(w, `{"success":true}`)
	})

	if err := client.SARegister(context.Background(), "alice", "secret"); err != nil {
		t.Fatalf("SARegister: %v", err)
	}
}

// TestPasswdSendsPassphrase pins the same field for the rotation path, which
// shares ergo's request shape and therefore shares the failure: a rotation
// that named the field wrongly would set the account's passphrase to empty
// while reporting that it had been changed.
func TestPasswdSendsPassphrase(t *testing.T) {
	client := apiServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/v1/ns/passwd" {
			t.Errorf("path = %s", got)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if got := body["passphrase"]; got != "rotated" {
			t.Errorf("passphrase = %q, want the new secret under the key ergo decodes", got)
		}
		io.WriteString(w, `{"success":true}`)
	})

	if err := client.Passwd(context.Background(), "alice", "rotated"); err != nil {
		t.Fatalf("Passwd: %v", err)
	}
}

// TestSARegisterExistingIsBodyNotStatus is the load-bearing case: ergo reports
// an already-registered account in the JSON BODY with HTTP 200. A client that
// decided on the status code would read this as success and hand back a
// passphrase that does not work, silently skipping the ErrExists path that
// keeps the existing secret.
func TestSARegisterExistingIsBodyNotStatus(t *testing.T) {
	client := apiServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `{"success":false,"error":"Name reserved due to a prior registration","errorCode":"ACCOUNT_EXISTS"}`)
	})

	err := client.SARegister(context.Background(), "alice", "secret")
	if !errors.Is(err, provider.ErrExists) {
		t.Fatalf("error = %v, want ErrExists from a 200 body", err)
	}
}

func TestSARegisterOtherFailureIsNotErrExists(t *testing.T) {
	// A different failure code must not be laundered into ErrExists, which
	// the caller treats as success.
	client := apiServer(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"success":false,"error":"bad passphrase","errorCode":"INVALID_PASSWORD"}`)
	})

	err := client.SARegister(context.Background(), "alice", "secret")
	if err == nil {
		t.Fatal("a failed registration should error")
	}
	if errors.Is(err, provider.ErrExists) {
		t.Error("an unrelated failure must not read as ErrExists")
	}
	if !strings.Contains(err.Error(), "bad passphrase") {
		t.Errorf("error %v should quote the server message", err)
	}
}

// TestUnauthorizedIsPlainText pins the other half of the decoding contract:
// auth failures come back as plain text, not JSON. A decoder that assumed
// JSON would report a syntax error and bury the real cause.
func TestUnauthorizedIsPlainText(t *testing.T) {
	client := apiServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, "Unauthorized\n")
	})

	err := client.SARegister(context.Background(), "alice", "secret")
	if !errors.Is(err, provider.ErrUnauthorized) {
		t.Fatalf("error = %v, want ErrUnauthorized", err)
	}
}

func TestMethodNotAllowedIsPlainText(t *testing.T) {
	client := apiServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusMethodNotAllowed)
		io.WriteString(w, "Method Not Allowed\n")
	})

	_, err := client.Status(context.Background())
	if err == nil {
		t.Fatal("a 405 should error")
	}
	if errors.Is(err, provider.ErrUnauthorized) {
		t.Error("a 405 is not an auth failure")
	}
	if !strings.Contains(err.Error(), "Method Not Allowed") {
		t.Errorf("error %v should quote the plain-text body", err)
	}
}

func TestStatusDecodesVersion(t *testing.T) {
	client := apiServer(t, func(w http.ResponseWriter, _ *http.Request) {
		// Verbatim from the capture, including the undocumented commit
		// field, which must not break decoding.
		io.WriteString(w, `{"success":true,"version":"2.19.1","go_version":"go1.26.5","commit":"63c743a","start_time":"2026-09-11T10:21:10.860Z","users":{"total":0},"channels":0,"servers":1}`)
	})

	info, err := client.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if info.Version != "2.19.1" {
		t.Errorf("version = %q, want 2.19.1", info.Version)
	}
}

// TestListMapsOwnerToFounder pins the field-name trap: /v1/list names the
// registered owner "owner", not "founder". Reading the interface's own
// vocabulary off the wire would blank every founder.
func TestListMapsOwnerToFounder(t *testing.T) {
	client := apiServer(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"success":true,"channels":[{"name":"#probe","userCount":0,"topic":"","registered":true,"owner":"botty","registeredAt":"2026-09-11T10:22:38.832Z"}]}`)
	})

	got, err := client.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("channels = %d, want 1", len(got))
	}
	if got[0].Founder != "botty" {
		t.Errorf("founder = %q, want botty (from the owner field)", got[0].Founder)
	}
	if !got[0].Registered {
		t.Error("registered should be carried through")
	}
	// userCount is live occupancy, not membership: a registered channel with
	// persistent roles and nobody present reports zero, and that zero is the
	// truth rather than a decode failure.
	if got[0].Users != 0 {
		t.Errorf("users = %d, want 0", got[0].Users)
	}
}

func TestListEmpty(t *testing.T) {
	client := apiServer(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"success":true,"channels":[]}`)
	})

	got, err := client.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("channels = %+v, want none", got)
	}
}

func TestListTolerAtesAbsentTopicSetAt(t *testing.T) {
	// topicSetAt is absent when no topic was ever set, so nothing may be
	// required from the payload.
	client := apiServer(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"success":true,"channels":[{"name":"#probe","registered":true,"owner":"botty"}]}`)
	})

	got, err := client.List(context.Background())
	if err != nil {
		t.Fatalf("List with an absent topicSetAt: %v", err)
	}
	if len(got) != 1 || got[0].Topic != "" {
		t.Errorf("channels = %+v", got)
	}
}

func TestOwnedChannels(t *testing.T) {
	client := apiServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/ns/info" {
			t.Errorf("path = %s", r.URL.Path)
		}
		io.WriteString(w, `{"success":true,"accountName":"botty","registeredAt":"2026-09-11T10:21:30.897Z","channels":["#deleg","#purgeme","#probe"]}`)
	})

	got, err := client.OwnedChannels(context.Background(), "botty")
	if err != nil {
		t.Fatalf("OwnedChannels: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("channels = %v, want 3", got)
	}
}

func TestPasswdAndRehash(t *testing.T) {
	var paths []string
	client := apiServer(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		io.WriteString(w, `{"success":true}`)
	})

	if err := client.Passwd(context.Background(), "alice", "next"); err != nil {
		t.Fatalf("Passwd: %v", err)
	}
	if err := client.Rehash(context.Background()); err != nil {
		t.Fatalf("Rehash: %v", err)
	}

	want := []string{"/v1/ns/passwd", "/v1/rehash"}
	for i, path := range want {
		if paths[i] != path {
			t.Errorf("path[%d] = %s, want %s", i, paths[i], path)
		}
	}
}

func TestNewClientNilConfig(t *testing.T) {
	// A nil API config yields a nil client, which is how the account
	// provider learns it must withhold the capability rather than fail at
	// call time.
	if got := NewClient(nil); got != nil {
		t.Errorf("NewClient(nil) = %+v, want nil", got)
	}
}

func TestBaseURLTrailingSlashTrimmed(t *testing.T) {
	client := NewClient(&provider.APIConfig{BaseURL: "http://example.test/", Token: "t"})
	if strings.HasSuffix(client.BaseURL, "/") {
		t.Errorf("BaseURL = %q, want no trailing slash", client.BaseURL)
	}
}
