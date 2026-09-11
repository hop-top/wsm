// Package managed renders the ergo configuration wsm owns, checks the
// prerequisites a managed server needs before it can start, and wraps the
// ergo subcommands that provision one.
//
// Everything here is managed-mode only: wsm generates the config, the
// certificates, the database and the API token, then supervises the resulting
// child. External mode never reaches this package, because wsm does not own
// the server there.
//
// The config is rendered from the template below rather than by editing
// ergo's shipped default.yaml. That is deliberate: default.yaml carries a
// commented `#password:` example hundreds of lines above the live
// `opers.admin` entry, so any first-match substitution over the upstream file
// silently edits the comment and leaves the real oper password unset. Owning
// the whole document makes that class of bug unrepresentable, and keeps the
// fixed security posture (registration closed, SASL required, loopback API)
// from depending on upstream defaults that may change between releases.
//
// This package holds no UI dependency (CI enforces `make verify-no-ui-leak`).
package managed

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"golang.org/x/crypto/bcrypt"

	"hop.top/wsm/pkg/irc"
)

// BcryptCost is the cost factor for the oper password hash. ergo's own
// genpasswd defaults low because the hash is checked on every OPER; 10 is the
// bcrypt library default and keeps a rendered config cheap to produce while
// staying well above ergo's shipped example.
const BcryptCost = 10

// OperClass is the name of the single oper class wsm renders, and the class
// the bot's oper block belongs to.
const OperClass = "wsm"

// OperCaps are the capabilities the bot's oper class carries. Deliberately
// narrow: account registration and channel registration to provision, sajoin
// and samode to repair a channel it was locked out of, kill to evict a stuck
// client, rehash to reload after a token rotation, and history for backfill.
var OperCaps = []string{
	"accreg", "chanreg", "sajoin", "samode", "kill", "rehash", "history",
}

// EnvLanguagesPath overrides where the rendered config points ergo at its
// translation files. ergo's shipped default.yaml sets `languages.path` to the
// relative "languages", which resolves only with the working directory inside
// the unpacked release: from wsm's data dir every ergo subcommand dies on
// "Could not load languages" before it reads anything else. The rendered
// config therefore always writes an absolute path, and this variable lets an
// operator point at a non-standard install.
const EnvLanguagesPath = "WSM_IRC_ERGO_LANGUAGES"

// DefaultHistoryMessages is the per-channel in-memory history depth, and the
// chathistory page ceiling the bot's backfill pages against.
const DefaultHistoryMessages = 1000

// renderData is the flattened view the template renders. The template is
// deliberately given primitives only: no method calls on irc types, so the
// rendered document cannot drift when an unrelated field is added upstream.
type renderData struct {
	ServerName  string
	NetworkName string

	PlainPort  int
	RemotePort int
	APIPort    int
	Bind       string

	Remote         bool
	RemoteHost     string
	TLSCert        string
	TLSKey         string
	AllowedOrigins []string

	DataDir       string
	DBPath        string
	LanguagesPath string

	BotAccount string
	OperClass  string
	OperCaps   []string
	OperHash   string

	APIToken string

	HistoryMessages int
}

// Renderer renders ergo configs against one workspace's resolved paths.
//
// Paths is a field rather than a RenderConfig argument because irc.Profile
// records no directory roots: it carries the allocated ports, the workspace
// Root and the remote flag, but the data dir, the database, the TLS dir and
// the API token file are all resolved by irc.ResolvePaths from roots only the
// caller knows. The rendered document needs those as absolute paths, so the
// caller supplies them once and renders as often as it likes.
type Renderer struct {
	// Paths locates the database, TLS material and API token the rendered
	// config points ergo at. Required.
	Paths irc.Paths
}

// RenderConfig produces the ergo.yaml for one managed workspace.
//
// Paths reach the renderer through DefaultPaths, which the caller sets once
// per workspace; irc.Profile records no directory roots, so there is nothing
// in cfg, p or s from which absolute paths could be derived. Callers that hold
// an irc.Paths directly (the daemon, the CLI) should prefer a Renderer value,
// which makes the dependency explicit instead of relying on package state.
//
// The security posture is fixed, not configurable: account registration is
// closed, SASL is required to connect, nick reservation is strict, channels
// default to +ntR, the HTTP API listens on loopback behind a bearer token, and
// the plaintext listener never leaves 127.0.0.1. Only the parts wsm allocates
// or the operator legitimately owns vary: ports, the remote listener, the
// advertised host and allowed WebSocket origins.
//
// The oper password is the bot's own secret, bcrypt-hashed. It is resolved
// through irc.ReadBotPassword so an aps-owned secret works exactly like one in
// secrets.yaml, and it is the only credential that reaches the rendered file
// in any form.
func RenderConfig(cfg irc.Config, p irc.Profile, s irc.Secrets) ([]byte, error) {
	return Renderer{Paths: DefaultPaths}.RenderConfig(cfg, p, s)
}

// DefaultPaths is the path set the package-level RenderConfig renders against.
// The daemon and the CLI set it once, right after irc.ResolvePaths, for the
// workspace they are acting on. It exists only so the task's three-argument
// RenderConfig can name absolute paths that irc.Profile does not carry;
// prefer an explicit Renderer wherever a value can be threaded instead.
var DefaultPaths irc.Paths

// RenderConfig produces the ergo.yaml for one managed workspace against the
// renderer's paths. See the package-level RenderConfig for the fixed policy
// the rendered document encodes.
func (r Renderer) RenderConfig(cfg irc.Config, p irc.Profile, s irc.Secrets) ([]byte, error) {
	if !p.Ports.Valid() {
		return nil, fmt.Errorf(
			"irc managed render: profile has no allocated port triple; allocate before rendering",
		)
	}

	botAccount := cfg.Bot.Account
	if botAccount == "" {
		botAccount = irc.DefaultBotAccount
	}

	password, err := irc.ReadBotPassword(cfg, s)
	if err != nil {
		return nil, fmt.Errorf("irc managed render: bot password: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		return nil, fmt.Errorf("irc managed render: hash oper password: %w", err)
	}

	paths := r.Paths
	if paths.DataDir == "" {
		return nil, fmt.Errorf(
			"irc managed render: no resolved paths; set managed.DefaultPaths or use a Renderer",
		)
	}

	token, err := irc.LoadAPIToken(paths.APIToken)
	if err != nil {
		return nil, fmt.Errorf("irc managed render: api token: %w", err)
	}

	languages, err := LanguagesPath(cfg.Binary)
	if err != nil {
		return nil, err
	}

	serverName := cfg.ServerName
	if serverName == "" {
		serverName = irc.DefaultServerName
	}

	bind := cfg.Bind
	if bind == "" {
		bind = irc.DefaultBind
	}

	data := renderData{
		ServerName:  serverName,
		NetworkName: serverName,

		PlainPort:  p.Ports.Plain,
		RemotePort: p.Ports.Remote,
		APIPort:    p.Ports.API,
		Bind:       bind,

		Remote:         cfg.Remote.Enabled,
		RemoteHost:     cfg.Remote.Host,
		TLSCert:        tlsCertPath(cfg, paths),
		TLSKey:         tlsKeyPath(cfg, paths),
		AllowedOrigins: cfg.Remote.AllowedOrigins,

		DataDir:       paths.DataDir,
		DBPath:        paths.DB,
		LanguagesPath: languages,

		BotAccount: botAccount,
		OperClass:  OperClass,
		OperCaps:   OperCaps,
		OperHash:   string(hash),

		APIToken: token,

		HistoryMessages: DefaultHistoryMessages,
	}

	var buf bytes.Buffer
	if err := configTemplate.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("irc managed render: execute template: %w", err)
	}
	return buf.Bytes(), nil
}

// tlsCertPath prefers an operator-supplied certificate and falls back to the
// self-signed one wsm generates inside the workspace's TLS dir.
func tlsCertPath(cfg irc.Config, paths irc.Paths) string {
	if cfg.Remote.TLSCert != "" {
		return cfg.Remote.TLSCert
	}
	return paths.TLSCertPath()
}

// tlsKeyPath mirrors tlsCertPath for the private key.
func tlsKeyPath(cfg irc.Config, paths irc.Paths) string {
	if cfg.Remote.TLSKey != "" {
		return cfg.Remote.TLSKey
	}
	return paths.TLSKeyPath()
}

// LanguagesPath resolves the absolute directory holding ergo's translation
// files. ergo ships `languages.path` relative, which resolves only from inside
// the release directory; every subcommand fails with "Could not load
// languages" otherwise, so the rendered config must carry an absolute path.
//
// Resolution order: the explicit override, then the `languages` directory
// beside the resolved binary, then its sibling under the install prefix
// (…/bin/ergo → …/languages, the layout of the packaged releases).
func LanguagesPath(binary string) (string, error) {
	if override := os.Getenv(EnvLanguagesPath); override != "" {
		return override, nil
	}

	if binary == "" {
		binary = irc.DefaultBinary
	}

	resolved, err := exec.LookPath(binary)
	if err != nil {
		return "", fmt.Errorf(
			"irc managed render: locate %s to resolve its languages directory: %w", binary, err,
		)
	}
	if abs, err := filepath.Abs(resolved); err == nil {
		resolved = abs
	}

	dir := filepath.Dir(resolved)
	candidates := []string{
		filepath.Join(dir, "languages"),
		filepath.Join(filepath.Dir(dir), "languages"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}
	}

	return "", fmt.Errorf(
		"irc managed render: no languages directory found near %s; set %s to ergo's languages directory",
		resolved, EnvLanguagesPath,
	)
}

// bcryptCompare reports whether hash is the bcrypt digest of password. It
// exists so callers (and the render tests) can verify that the hash written
// into the config belongs to the bot secret actually in force, rather than
// trusting that the right value reached the template.
func bcryptCompare(hash, password string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return fmt.Errorf("irc managed render: oper hash does not match the bot secret: %w", err)
	}
	return nil
}

// yamlList renders a string slice as a YAML flow sequence, quoting each entry.
// Used for oper capabilities and WebSocket origins, both of which are short
// and read better inline than as block sequences.
func yamlList(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	quoted := make([]string, 0, len(items))
	for _, item := range items {
		quoted = append(quoted, yamlQuote(item))
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// yamlQuote renders s as a double-quoted YAML scalar. Paths, hostnames, oper
// hashes and the API token all reach the document through this, so a value
// containing a colon, a '#' or a leading '*' cannot change the parse.
func yamlQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// configTemplate is the whole ergo document wsm owns. Written out in full
// rather than patched into upstream's default.yaml; see the package comment.
var configTemplate = template.Must(template.New("ergo.yaml").Funcs(template.FuncMap{
	"yamlList":  yamlList,
	"yamlQuote": yamlQuote,
}).Parse(`# Generated by wsm. Do not edit: every ` + "`wsm irc start`" + ` re-renders this file
# and any local change is lost. Change the wsm irc config instead.

network:
    name: {{ yamlQuote .NetworkName }}

server:
    name: {{ yamlQuote .ServerName }}

    listeners:
        # Plaintext, loopback only: the local bot and local clients.
        "127.0.0.1:{{ .PlainPort }}":
{{- if .Remote }}
        # Remote listener: TLS and WebSocket, both mandatory off-loopback.
        "{{ .Bind }}:{{ .RemotePort }}":
            tls:
                cert: {{ yamlQuote .TLSCert }}
                key: {{ yamlQuote .TLSKey }}
            websocket: true
{{- end }}

    sts:
        enabled: false

    casemapping: precis
    enforce-utf8: true

    relaymsg:
        enabled: true
        separators: "/"
        available-to-chanops: true

    websockets:
        allowed-origins: {{ yamlList .AllowedOrigins }}

    ip-cloaking:
        enabled: false

    max-sendq: 96k

    compatibility:
        force-trailing: true
        send-unprefixed-sasl: false

    ip-limits:
        count: false
        throttle: false

accounts:
    # wsm provisions every account through the HTTP API; self-registration is
    # closed so the member list is the only way onto this server.
    registration:
        enabled: false
        allow-before-connect: false

    authentication-enabled: true
    require-sasl:
        enabled: true

    login-throttling:
        enabled: true
        duration: 1m
        max-attempts: 10

    advertise-scram: true

    nick-reservation:
        enabled: true
        additional-nick-limit: 0
        method: strict
        allow-custom-enforcement: false
        guest-nickname-format: "Guest-*"
        force-guest-format: false
        force-nick-equals-account: true

    multiclient:
        enabled: true
        allowed-by-default: false
        always-on: opt-in
        auto-away: opt-in

    vhosts:
        enabled: false

channels:
    default-modes: "+ntR"

    max-channels-per-client: 100

    registration:
        enabled: true
        operator-only: false
        max-channels-per-account: 100

    list-delay: 0

oper-classes:
    "{{ .OperClass }}":
        title: "wsm"
        capabilities: {{ yamlList .OperCaps }}

opers:
    # The wsm bot. Password is the bot's own secret, bcrypt-hashed; the bot
    # also opers up automatically once SASL has authenticated the account, so
    # the password is a fallback rather than the usual path.
    {{ .BotAccount }}:
        class: {{ yamlQuote .OperClass }}
        whois-line: "is the wsm workspace bot"
        password: {{ yamlQuote .OperHash }}
        auto: true
        hidden: true

logging:
    -
        method: stderr
        type: "* -userinput -useroutput"
        level: info

debug:
    recover-from-errors: true

limits:
    nicklen: 32
    identlen: 20
    channellen: 64
    awaylen: 390
    kicklen: 390
    topiclen: 390
    monitor-entries: 100
    whowas-entries: 100
    chan-list-modes: 60
    registration-messages: 1024

    multiline:
        max-bytes: 4096
        max-lines: 100

history:
    enabled: true
    channel-length: {{ .HistoryMessages }}
    client-length: {{ .HistoryMessages }}
    autoresize-window: 3d
    autoreplay-on-join: 0
    chathistory-maxmessages: {{ .HistoryMessages }}
    znc-maxmessages: 2048

    restrictions:
        expire-time: 1w
        query-cutoff: 'none'
        grace-period: 1h

    persistent:
        enabled: false

    retention:
        allow-individual-delete: false
        enable-account-indexing: false

datastore:
    path: {{ yamlQuote .DBPath }}
    autoupgrade: true

languages:
    # Absolute: ergo ships this relative, and a relative path makes every
    # subcommand fail with "Could not load languages" from wsm's data dir.
    enabled: true
    path: {{ yamlQuote .LanguagesPath }}
    default: en

api:
    # Loopback only, behind a bearer token wsm generated. A misconfigured
    # listener here is a soft failure in ergo: the server logs and keeps
    # running without the API, so readiness is probed over HTTP, never assumed.
    enabled: true
    listener: "127.0.0.1:{{ .APIPort }}"
    tls: null
    bearer-tokens: [{{ yamlQuote .APIToken }}]
`))
