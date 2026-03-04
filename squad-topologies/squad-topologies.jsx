import { useState } from "react";

const COLORS = {
  stream: { bg: "#0d3b2e", border: "#1aff8c", text: "#1aff8c", label: "Stream-Aligned" },
  enabling: { bg: "#1a1a3e", border: "#6c7fff", text: "#6c7fff", label: "Enabling" },
  platform: { bg: "#2a1a00", border: "#ff9500", text: "#ff9500", label: "Platform" },
  subsystem: { bg: "#2a0a1a", border: "#ff4d8f", text: "#ff4d8f", label: "Complicated Subsystem" },
};

const INTERACTION_COLORS = {
  xaas: "#ff9500",
  collab: "#6c7fff",
  facilitating: "#1aff8c",
};

const GENERIC_MODEL = {
  title: "Squad Topologies — Generic Framework",
  description: "Four squad types × three interaction modes. Each squad has a context boundary, an interaction contract, and an evolutionary pressure direction.",
  squads: [
    {
      id: "sa1", type: "stream", name: "Stream-Aligned Squad A",
      tagline: "Domain: Billing / Payments",
      details: "Owns pipeline end-to-end. Can complete a unit of work without external coordination. Context-bounded to billing domain.",
      interactions: [
        { to: "platform", mode: "xaas", label: "consumes infra" },
        { to: "cs1", mode: "xaas", label: "delegates complexity" },
        { to: "sa2", mode: "collab", label: "short-lived collaboration" },
      ]
    },
    {
      id: "sa2", type: "stream", name: "Stream-Aligned Squad B",
      tagline: "Domain: Content Extraction",
      details: "Owns its extraction pipeline. Consumes scraping capability from enabling squad once leveled up.",
      interactions: [
        { to: "platform", mode: "xaas", label: "consumes infra" },
        { to: "en1", mode: "facilitating", label: "receives capability uplift" },
      ]
    },
    {
      id: "en1", type: "enabling", name: "Enabling Squad",
      tagline: "Capability: Web Scraping / Tool Use",
      details: "Temporarily attached. Levels up stream squads then moves on. Goal: make itself unnecessary.",
      interactions: [
        { to: "sa2", mode: "facilitating", label: "teaches, then exits" },
      ]
    },
    {
      id: "cs1", type: "subsystem", name: "Complicated Subsystem Squad",
      tagline: "Domain: Legal Reasoning / Graph Traversal",
      details: "Hard problem, clean interface. No squad should reimplement this. Exposed as a service.",
      interactions: [
        { to: "platform", mode: "xaas", label: "uses shared memory" },
      ]
    },
    {
      id: "platform", type: "platform", name: "Platform Squad",
      tagline: "Infra: Tool Registry / Memory / Observability",
      details: "Self-service substrate. Provides shared memory primitives, tool registries, prompt versioning, cost telemetry. Golden path for new squads.",
      interactions: []
    },
  ],
  principles: [
    "Context load is the primary constraint — not cognitive load",
    "Interaction mode must be declared, not assumed",
    "X-as-a-Service is the target steady state for all squad pairs",
    "Platform squad has an API contract — no reaching into internals",
    "Squad topology IS the communication architecture (inverse Conway)",
  ]
};

const IC_MODEL = {
  title: "Squad Topologies — Idea Crafters Applied",
  description: "Federated model across portfolio products. Each product owns stream-aligned squads. SlimMCP anchors the platform layer. Enabling squads are shared across ventures.",
  squads: [
    {
      id: "castor", type: "stream", name: "Castor Guide Squad",
      tagline: "Product: Canadian marketplace (castor.guide)",
      details: "Stream-aligned to the Canadian product matching domain. Owns crawl → match → present pipeline. Consumes matching logic from subsystem squad.",
      interactions: [
        { to: "platform_ic", mode: "xaas", label: "SlimMCP / shared infra" },
        { to: "matching", mode: "xaas", label: "CA product matching" },
        { to: "research", mode: "facilitating", label: "competitive intel" },
      ]
    },
    {
      id: "lesexperts", type: "stream", name: "Les Experts AI Squad",
      tagline: "Product: AI consulting delivery for SMBs",
      details: "Stream-aligned to consulting engagements. Owns discovery → recommendation → delivery pipeline. Elevated context on SMB patterns.",
      interactions: [
        { to: "platform_ic", mode: "xaas", label: "SlimMCP / shared infra" },
        { to: "content", mode: "facilitating", label: "content generation" },
        { to: "castor", mode: "collab", label: "cross-product intel" },
      ]
    },
    {
      id: "launchpit", type: "stream", name: "LaunchPit Squad",
      tagline: "Product: Survey & engagement platform",
      details: "Stream-aligned to survey lifecycle. Owns form generation → response analysis → insight delivery.",
      interactions: [
        { to: "platform_ic", mode: "xaas", label: "SlimMCP / shared infra" },
        { to: "research", mode: "xaas", label: "market research layer" },
      ]
    },
    {
      id: "research", type: "enabling", name: "Research Enabling Squad",
      tagline: "Capability: Competitive research / market signals",
      details: "Shared across all ventures. Temporary attachment. Delivers market intel capability then exits. Castor gets CA alternatives data; LaunchPit gets survey benchmarks.",
      interactions: [
        { to: "platform_ic", mode: "xaas", label: "tool registry" },
      ]
    },
    {
      id: "content", type: "enabling", name: "Content Enabling Squad",
      tagline: "Capability: Content generation / social media",
      details: "Cross-cutting content capability across all Twitter accounts and product marketing. Teaches stream squads content patterns, then hands off.",
      interactions: [
        { to: "platform_ic", mode: "xaas", label: "prompt versioning" },
      ]
    },
    {
      id: "matching", type: "subsystem", name: "CA Product Matching Squad",
      tagline: "Subsystem: Canadian alternative matching logic",
      details: "Hard, domain-specific logic for Castor. US product → CA equivalent matching, with quality scoring and availability checking. Clean interface, don't touch internals.",
      interactions: [
        { to: "platform_ic", mode: "xaas", label: "shared memory / cache" },
      ]
    },
    {
      id: "platform_ic", type: "platform", name: "SlimMCP Platform Squad",
      tagline: "Infra: SlimMCP middleware / tool registry / observability",
      details: "60-85% token reduction layer. Provides all stream squads with: versioned tool schemas, shared session state, cost telemetry per product, golden path onboarding. The invisible substrate.",
      interactions: []
    },
  ],
  principles: [
    "SlimMCP is the platform contract — stream squads consume, never extend",
    "Research and content enabling squads are shared resources — schedule their attachment deliberately",
    "CA matching logic is a complicated subsystem — Castor's other squads cannot replicate it",
    "Each product has budget-aware stream squads (LaunchPit / Les Experts / Castor)",
    "Tiny Bets methodology → squad shapes evolve based on measured interaction load",
  ]
};

const INTERACTION_LEGEND = [
  { mode: "xaas", color: INTERACTION_COLORS.xaas, label: "X-as-a-Service", desc: "Stable, self-service API. No coordination needed." },
  { mode: "collab", color: INTERACTION_COLORS.collab, label: "Collaboration", desc: "High bandwidth, time-boxed. For unknowns." },
  { mode: "facilitating", color: INTERACTION_COLORS.facilitating, label: "Facilitating", desc: "Enabling squad teaches, then exits." },
];

function SquadCard({ squad, isSelected, onClick }) {
  const c = COLORS[squad.type];
  return (
    <div
      onClick={() => onClick(squad)}
      style={{
        background: c.bg,
        border: `1.5px solid ${isSelected ? "#fff" : c.border}`,
        borderRadius: 10,
        padding: "14px 18px",
        cursor: "pointer",
        transition: "all 0.2s",
        boxShadow: isSelected ? `0 0 24px ${c.border}88` : `0 0 8px ${c.border}22`,
        transform: isSelected ? "scale(1.02)" : "scale(1)",
        minWidth: 180,
        flex: "1 1 180px",
        maxWidth: 260,
      }}
    >
      <div style={{ fontSize: 10, color: c.text, fontFamily: "'Courier New', monospace", letterSpacing: 2, marginBottom: 4, textTransform: "uppercase", opacity: 0.8 }}>
        {COLORS[squad.type].label}
      </div>
      <div style={{ fontSize: 13, fontWeight: 700, color: "#fff", marginBottom: 4, lineHeight: 1.3 }}>{squad.name}</div>
      <div style={{ fontSize: 11, color: c.text, opacity: 0.9, lineHeight: 1.4 }}>{squad.tagline}</div>
    </div>
  );
}

function InteractionBadge({ mode }) {
  const c = INTERACTION_COLORS[mode];
  const labels = { xaas: "X-as-a-Service", collab: "Collaboration", facilitating: "Facilitating" };
  return (
    <span style={{
      display: "inline-block",
      padding: "2px 8px",
      borderRadius: 4,
      border: `1px solid ${c}`,
      color: c,
      fontSize: 10,
      fontFamily: "'Courier New', monospace",
      letterSpacing: 1,
      textTransform: "uppercase",
    }}>{labels[mode]}</span>
  );
}

function DetailPanel({ squad, allSquads, onClose }) {
  const c = COLORS[squad.type];
  return (
    <div style={{
      background: "#0a0a0f",
      border: `1px solid ${c.border}`,
      borderRadius: 12,
      padding: "24px 28px",
      marginTop: 20,
      boxShadow: `0 0 40px ${c.border}33`,
      fontFamily: "'Courier New', monospace",
    }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", marginBottom: 16 }}>
        <div>
          <div style={{ fontSize: 10, color: c.text, letterSpacing: 2, textTransform: "uppercase", marginBottom: 6 }}>{COLORS[squad.type].label} Squad</div>
          <div style={{ fontSize: 18, fontWeight: 800, color: "#fff", marginBottom: 4 }}>{squad.name}</div>
          <div style={{ fontSize: 12, color: c.text }}>{squad.tagline}</div>
        </div>
        <button onClick={onClose} style={{ background: "none", border: "none", color: "#666", cursor: "pointer", fontSize: 18, padding: 4 }}>✕</button>
      </div>
      <div style={{ fontSize: 12, color: "#ccc", lineHeight: 1.7, marginBottom: 20, borderLeft: `3px solid ${c.border}`, paddingLeft: 14 }}>
        {squad.details}
      </div>
      {squad.interactions.length > 0 && (
        <div>
          <div style={{ fontSize: 10, color: "#555", letterSpacing: 2, textTransform: "uppercase", marginBottom: 10 }}>Interaction Contracts</div>
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            {squad.interactions.map((ix, i) => {
              const target = allSquads.find(s => s.id === ix.to);
              return (
                <div key={i} style={{ display: "flex", alignItems: "center", gap: 10 }}>
                  <InteractionBadge mode={ix.mode} />
                  <span style={{ fontSize: 11, color: "#888" }}>→</span>
                  <span style={{ fontSize: 11, color: "#ddd" }}>{target?.name || ix.to}</span>
                  <span style={{ fontSize: 10, color: "#555" }}>// {ix.label}</span>
                </div>
              );
            })}
          </div>
        </div>
      )}
      {squad.interactions.length === 0 && (
        <div style={{ fontSize: 11, color: "#555", fontStyle: "italic" }}>No outbound interactions — this is the substrate. Others consume you.</div>
      )}
    </div>
  );
}

export default function SquadTopologies() {
  const [activeLayer, setActiveLayer] = useState("generic");
  const [selectedSquad, setSelectedSquad] = useState(null);
  const model = activeLayer === "generic" ? GENERIC_MODEL : IC_MODEL;

  const handleSquadClick = (squad) => {
    setSelectedSquad(selectedSquad?.id === squad.id ? null : squad);
  };

  const squadsByType = {
    stream: model.squads.filter(s => s.type === "stream"),
    enabling: model.squads.filter(s => s.type === "enabling"),
    subsystem: model.squads.filter(s => s.type === "subsystem"),
    platform: model.squads.filter(s => s.type === "platform"),
  };

  return (
    <div style={{
      background: "#06060c",
      minHeight: "100vh",
      fontFamily: "'Courier New', monospace",
      padding: "32px 24px",
      color: "#fff",
    }}>
      {/* Header */}
      <div style={{ maxWidth: 900, margin: "0 auto" }}>
        <div style={{ marginBottom: 6, fontSize: 10, color: "#444", letterSpacing: 3, textTransform: "uppercase" }}>
          Team Topologies → Agent Systems
        </div>
        <div style={{ fontSize: 28, fontWeight: 900, letterSpacing: -1, marginBottom: 4, background: "linear-gradient(90deg, #1aff8c, #6c7fff)", WebkitBackgroundClip: "text", WebkitTextFillColor: "transparent" }}>
          Squad Topologies
        </div>
        <div style={{ fontSize: 12, color: "#666", marginBottom: 28, lineHeight: 1.6 }}>
          Organizing autonomous agent units for maximum flow and minimum context load.
        </div>

        {/* Layer toggle */}
        <div style={{ display: "flex", gap: 8, marginBottom: 28 }}>
          {[["generic", "Generic Framework"], ["ic", "Idea Crafters Applied"]].map(([k, label]) => (
            <button
              key={k}
              onClick={() => { setActiveLayer(k); setSelectedSquad(null); }}
              style={{
                padding: "8px 16px",
                borderRadius: 6,
                border: `1px solid ${activeLayer === k ? "#1aff8c" : "#333"}`,
                background: activeLayer === k ? "#0d3b2e" : "transparent",
                color: activeLayer === k ? "#1aff8c" : "#555",
                cursor: "pointer",
                fontSize: 11,
                letterSpacing: 1,
                textTransform: "uppercase",
                transition: "all 0.2s",
              }}
            >{label}</button>
          ))}
        </div>

        <div style={{ fontSize: 12, color: "#888", marginBottom: 28, lineHeight: 1.6, maxWidth: 680 }}>
          {model.description}
        </div>

        {/* Legend */}
        <div style={{ display: "flex", gap: 20, marginBottom: 32, flexWrap: "wrap" }}>
          {Object.entries(COLORS).map(([type, c]) => (
            <div key={type} style={{ display: "flex", alignItems: "center", gap: 6 }}>
              <div style={{ width: 8, height: 8, borderRadius: 2, background: c.bg, border: `1.5px solid ${c.border}` }} />
              <span style={{ fontSize: 10, color: "#666", textTransform: "uppercase", letterSpacing: 1 }}>{c.label}</span>
            </div>
          ))}
        </div>

        {/* Topology map */}
        <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>

          {/* Stream row */}
          <div>
            <div style={{ fontSize: 9, color: "#1aff8c66", letterSpacing: 3, textTransform: "uppercase", marginBottom: 8 }}>— Stream-Aligned (value flow) —</div>
            <div style={{ display: "flex", gap: 12, flexWrap: "wrap" }}>
              {squadsByType.stream.map(s => (
                <SquadCard key={s.id} squad={s} isSelected={selectedSquad?.id === s.id} onClick={handleSquadClick} />
              ))}
            </div>
          </div>

          {/* Enabling + Subsystem row */}
          <div style={{ display: "flex", gap: 20, flexWrap: "wrap" }}>
            {squadsByType.enabling.length > 0 && (
              <div style={{ flex: 1 }}>
                <div style={{ fontSize: 9, color: "#6c7fff66", letterSpacing: 3, textTransform: "uppercase", marginBottom: 8 }}>— Enabling (capability uplift) —</div>
                <div style={{ display: "flex", gap: 12, flexWrap: "wrap" }}>
                  {squadsByType.enabling.map(s => (
                    <SquadCard key={s.id} squad={s} isSelected={selectedSquad?.id === s.id} onClick={handleSquadClick} />
                  ))}
                </div>
              </div>
            )}
            {squadsByType.subsystem.length > 0 && (
              <div style={{ flex: 1 }}>
                <div style={{ fontSize: 9, color: "#ff4d8f66", letterSpacing: 3, textTransform: "uppercase", marginBottom: 8 }}>— Complicated Subsystem (clean interface) —</div>
                <div style={{ display: "flex", gap: 12, flexWrap: "wrap" }}>
                  {squadsByType.subsystem.map(s => (
                    <SquadCard key={s.id} squad={s} isSelected={selectedSquad?.id === s.id} onClick={handleSquadClick} />
                  ))}
                </div>
              </div>
            )}
          </div>

          {/* Platform row */}
          <div>
            <div style={{ fontSize: 9, color: "#ff950066", letterSpacing: 3, textTransform: "uppercase", marginBottom: 8 }}>— Platform (substrate, consumed by all) —</div>
            <div style={{ display: "flex", gap: 12, flexWrap: "wrap" }}>
              {squadsByType.platform.map(s => (
                <SquadCard key={s.id} squad={s} isSelected={selectedSquad?.id === s.id} onClick={handleSquadClick} />
              ))}
            </div>
          </div>
        </div>

        {/* Detail panel */}
        {selectedSquad && (
          <DetailPanel
            squad={selectedSquad}
            allSquads={model.squads}
            onClose={() => setSelectedSquad(null)}
          />
        )}

        {/* Interaction mode legend */}
        <div style={{ marginTop: 32, padding: "20px 24px", background: "#0a0a0f", border: "1px solid #1a1a2e", borderRadius: 10 }}>
          <div style={{ fontSize: 9, color: "#444", letterSpacing: 3, textTransform: "uppercase", marginBottom: 14 }}>Interaction Modes</div>
          <div style={{ display: "flex", gap: 24, flexWrap: "wrap" }}>
            {INTERACTION_LEGEND.map(({ mode, color, label, desc }) => (
              <div key={mode} style={{ display: "flex", gap: 10, alignItems: "flex-start" }}>
                <div style={{ width: 3, background: color, borderRadius: 2, minHeight: 32, marginTop: 2 }} />
                <div>
                  <div style={{ fontSize: 11, color, letterSpacing: 1, textTransform: "uppercase", marginBottom: 2 }}>{label}</div>
                  <div style={{ fontSize: 10, color: "#555", lineHeight: 1.5 }}>{desc}</div>
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* Design principles */}
        <div style={{ marginTop: 20, padding: "20px 24px", background: "#0a0a0f", border: "1px solid #1a1a2e", borderRadius: 10 }}>
          <div style={{ fontSize: 9, color: "#444", letterSpacing: 3, textTransform: "uppercase", marginBottom: 14 }}>Design Constraints / Principles</div>
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            {model.principles.map((p, i) => (
              <div key={i} style={{ display: "flex", gap: 10, alignItems: "flex-start" }}>
                <span style={{ color: "#333", fontSize: 10, minWidth: 20, fontFamily: "monospace" }}>{String(i + 1).padStart(2, "0")}.</span>
                <span style={{ fontSize: 11, color: "#888", lineHeight: 1.6 }}>{p}</span>
              </div>
            ))}
          </div>
        </div>

        <div style={{ marginTop: 24, fontSize: 9, color: "#333", textAlign: "center", letterSpacing: 2 }}>
          SQUAD TOPOLOGIES · CLICK ANY SQUAD TO INSPECT INTERACTION CONTRACTS
        </div>
      </div>
    </div>
  );
}
