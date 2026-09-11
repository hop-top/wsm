// Package ergo implements the provider interfaces against an ergo server
// (https://github.com/ergochat/ergo), version 2.19 and later.
//
// The backend splits in two. Accounts go over ergo's HTTP API, which is the
// only way to create an account without a live nick; roles and channel state
// go over the IRC connection as ChanServ commands. Both halves are
// capability-gated on what the runtime can actually honour: the API half
// needs Deps.API, and the privileged half needs the bot to hold oper.
//
// Every command spelling and reply shape here was taken from a live ergo
// 2.19.1 capture under
// docs/plans/2026-09-10-irc-workspace-chat-design/probes/, not from the
// manual. Where the two disagree the capture wins, and the parser comments
// name the fixture that pinned them.
//
// Like the rest of pkg/*, this package must stay free of any UI dependency.
package ergo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"hop.top/wsm/pkg/irc/provider"
)

// APITimeout bounds every HTTP call. The API is loopback in managed mode and
// a LAN hop at worst in external mode, so a short bound turns a wedged server
// into a fast error rather than a hung reconcile.
const APITimeout = 5 * time.Second

// Client calls ergo's HTTP API. All endpoints are POST and take a bearer
// token; the zero value is unusable, build one with NewClient.
//
// It is exported because the daemon and CLI read Status, List and Rehash
// directly, without going through the provider interfaces: those three are
// server inspection rather than workspace reconciliation.
type Client struct {
	// BaseURL is the API root, e.g. http://127.0.0.1:8089, without a
	// trailing slash.
	BaseURL string
	// Token is the bearer token from `ergo gentoken`.
	Token string
	// HTTP is the client used for every call. Never nil after NewClient.
	HTTP *http.Client
}

// NewClient builds a Client from the provider's API config. A nil cfg yields
// a nil Client, which is how the account provider learns it has no API and
// must report no capability rather than failing at call time.
func NewClient(cfg *provider.APIConfig) *Client {
	if cfg == nil {
		return nil
	}
	return &Client{
		BaseURL: strings.TrimRight(cfg.BaseURL, "/"),
		Token:   cfg.Token,
		HTTP:    &http.Client{Timeout: APITimeout},
	}
}

// StatusInfo is the server identity reported by /v1/status. Only the fields
// wsm acts on are decoded; ergo sends more (go_version, an undocumented
// commit, user counts) and may send more still in a later release, which json
// ignores rather than failing on.
type StatusInfo struct {
	// Version is the ergo version string, e.g. "2.19.1".
	Version string `json:"version"`
}

// apiResult is the envelope every JSON response carries. ergo reports
// application failure in the body with HTTP 200, so success is the field that
// decides an error, never the status code.
type apiResult struct {
	Success   bool   `json:"success"`
	Error     string `json:"error"`
	ErrorCode string `json:"errorCode"`
}

// errAccountExists is the errorCode ergo returns for a saregister of a name
// that is already taken, verbatim from the probe. It arrives with HTTP 200.
const errAccountExists = "ACCOUNT_EXISTS"

// listChannel is one entry of /v1/list. The registered-owner field is named
// "owner", not "founder" — a mismatch that would silently blank every founder
// if copied from the interface's own vocabulary. topicSetAt is absent when no
// topic was ever set, so nothing here may be required. userCount is live
// occupancy, not membership: a registered channel with persistent AMODEs and
// nobody joined reports 0.
type listChannel struct {
	Name       string `json:"name"`
	Topic      string `json:"topic"`
	Registered bool   `json:"registered"`
	Owner      string `json:"owner"`
	UserCount  int    `json:"userCount"`
}

// listResponse is the /v1/list body.
type listResponse struct {
	apiResult
	Channels []listChannel `json:"channels"`
}

// statusResponse is the /v1/status body.
type statusResponse struct {
	apiResult
	StatusInfo
}

// infoResponse is the /v1/ns/info body: the channels one account owns. It is
// a cheaper founder lookup than one CS INFO per channel, since it answers for
// every channel at once.
type infoResponse struct {
	apiResult
	AccountName string   `json:"accountName"`
	Channels    []string `json:"channels"`
}

// SARegister creates account with passphrase, as a server admin would. An
// account that already exists is reported as provider.ErrExists, which
// callers treat as success and keep the existing credentials.
func (c *Client) SARegister(ctx context.Context, account, passphrase string) error {
	body := map[string]string{"accountName": account, "password": passphrase}
	var res apiResult
	if err := c.post(ctx, "/v1/ns/saregister", body, &res); err != nil {
		return err
	}
	if !res.Success {
		if res.ErrorCode == errAccountExists {
			return fmt.Errorf("irc/ergo: account %q: %w", account, provider.ErrExists)
		}
		return fmt.Errorf("irc/ergo: saregister %q: %s", account, res.message())
	}
	return nil
}

// Passwd rotates the passphrase of an existing account.
func (c *Client) Passwd(ctx context.Context, account, passphrase string) error {
	body := map[string]string{"accountName": account, "password": passphrase}
	var res apiResult
	if err := c.post(ctx, "/v1/ns/passwd", body, &res); err != nil {
		return err
	}
	if !res.Success {
		return fmt.Errorf("irc/ergo: passwd %q: %s", account, res.message())
	}
	return nil
}

// Status reports the server version and liveness.
func (c *Client) Status(ctx context.Context) (StatusInfo, error) {
	var res statusResponse
	if err := c.post(ctx, "/v1/status", struct{}{}, &res); err != nil {
		return StatusInfo{}, err
	}
	if !res.Success {
		return StatusInfo{}, fmt.Errorf("irc/ergo: status: %s", res.message())
	}
	return res.StatusInfo, nil
}

// List enumerates the channels the server knows. Topic is empty when unset,
// and Users is live occupancy rather than membership, so a registered channel
// with persistent roles and nobody present reports zero.
func (c *Client) List(ctx context.Context) ([]provider.ChannelInfo, error) {
	var res listResponse
	if err := c.post(ctx, "/v1/list", struct{}{}, &res); err != nil {
		return nil, err
	}
	if !res.Success {
		return nil, fmt.Errorf("irc/ergo: list: %s", res.message())
	}

	out := make([]provider.ChannelInfo, 0, len(res.Channels))
	for _, ch := range res.Channels {
		out = append(out, provider.ChannelInfo{
			Name:       ch.Name,
			Topic:      ch.Topic,
			Registered: ch.Registered,
			Founder:    ch.Owner,
			Users:      ch.UserCount,
		})
	}
	return out, nil
}

// OwnedChannels reports the channels account owns, via /v1/ns/info.
func (c *Client) OwnedChannels(ctx context.Context, account string) ([]string, error) {
	body := map[string]string{"accountName": account}
	var res infoResponse
	if err := c.post(ctx, "/v1/ns/info", body, &res); err != nil {
		return nil, err
	}
	if !res.Success {
		return nil, fmt.Errorf("irc/ergo: ns info %q: %s", account, res.message())
	}
	return res.Channels, nil
}

// Rehash reloads the server config, which is how a re-rendered ergo.yaml
// takes effect without dropping connections.
func (c *Client) Rehash(ctx context.Context) error {
	var res apiResult
	if err := c.post(ctx, "/v1/rehash", struct{}{}, &res); err != nil {
		return err
	}
	if !res.Success {
		return fmt.Errorf("irc/ergo: rehash: %s", res.message())
	}
	return nil
}

// message renders the server's failure text for an error string, falling back
// to the code when the prose is empty and to a constant when both are.
func (r apiResult) message() string {
	switch {
	case r.Error != "" && r.ErrorCode != "":
		return fmt.Sprintf("%s (%s)", r.Error, r.ErrorCode)
	case r.Error != "":
		return r.Error
	case r.ErrorCode != "":
		return r.ErrorCode
	default:
		return "request failed without an error message"
	}
}

// maxErrorBody bounds how much of a non-JSON failure body is quoted back. The
// plain-text failures are one short line; the cap is against a server that
// answers with a page.
const maxErrorBody = 512

// post sends body as JSON to path and decodes the response into out.
//
// Two shapes of failure are distinguished, because ergo uses both. A 401 is
// mapped to provider.ErrUnauthorized so callers can react to a stale token
// rather than parsing prose. Every other non-200 is quoted as text: auth and
// method failures come back as plain text ("Unauthorized", "Method Not
// Allowed"), not JSON, so decoding them would report a JSON syntax error and
// bury the real cause.
//
// A 200 is decoded as JSON and its success field left to the caller: ergo
// reports application failure (an account that already exists) in the body
// with a 200 status, so a status-only check would swallow it.
func (c *Client) post(ctx context.Context, path string, body, out any) error {
	if c == nil {
		return fmt.Errorf("irc/ergo: %s: %w", path, provider.ErrUnsupported)
	}

	encoded, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("irc/ergo: encode %s request: %w", path, err)
	}

	ctx, cancel := context.WithTimeout(ctx, APITimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("irc/ergo: build %s request: %w", path, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("irc/ergo: call %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.statusError(path, resp)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("irc/ergo: decode %s response: %w", path, err)
	}
	return nil
}

// statusError turns a non-200 into an error, mapping 401 to the sentinel and
// quoting the (plain-text) body otherwise.
func (c *Client) statusError(path string, resp *http.Response) error {
	text, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	if err != nil {
		text = nil
	}
	detail := strings.TrimSpace(string(text))

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("irc/ergo: %s: %w: %s", path, provider.ErrUnauthorized, detail)
	}
	return fmt.Errorf("irc/ergo: %s: http %d: %s", path, resp.StatusCode, detail)
}

// httpClient returns the configured client, or a timeout-bounded default when
// the Client was built by hand rather than through NewClient.
func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: APITimeout}
}

// errNoAPI reports a call that needs the HTTP API on a provider built without
// one. It is wrapped around ErrUnsupported so callers match the sentinel.
var errNoAPI = fmt.Errorf("%w: ergo HTTP API not configured", provider.ErrUnsupported)

// unsupported wraps ErrUnsupported with what was attempted, so a skipped
// operation says which capability was missing rather than only that one was.
func unsupported(op string) error {
	return fmt.Errorf("irc/ergo: %s: %w", op, provider.ErrUnsupported)
}
