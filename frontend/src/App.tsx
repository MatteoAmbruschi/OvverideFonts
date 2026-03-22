import { useEffect, useRef, useState, useCallback } from "react";
import { GetFonts, ApplyFont, ResetFont, GetStatus } from "../wailsjs/go/main/App";
import { EventsOn } from "../wailsjs/runtime/runtime";

type BtnState = "idle" | "applying" | "resetting";

// SVG icons as inline components
const IconFont = () => (
  <svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
    <path d="M9.5 4L4 20H6.5L8 16H16L17.5 20H20L14.5 4H9.5ZM8.7 14L12 5.3L15.3 14H8.7Z"/>
  </svg>
);

const IconSearch = () => (
  <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" className="search-icon">
    <circle cx="7" cy="7" r="4.5"/>
    <path d="M10.5 10.5L13.5 13.5" strokeLinecap="round"/>
  </svg>
);

const IconCheck = () => (
  <svg viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" className="font-check">
    <polyline points="2,7 5.5,10.5 12,4"/>
  </svg>
);

const IconX = () => (
  <svg viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round">
    <path d="M2 2L10 10M10 2L2 10"/>
  </svg>
);

const IconInfo = () => (
  <svg viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
    <circle cx="7" cy="7" r="5.5"/>
    <path d="M7 6.5V10M7 4.5V4.6"/>
  </svg>
);

const IconAlert = () => (
  <svg viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
    <path d="M7 1.5L13 12H1L7 1.5Z"/>
    <path d="M7 5.5V8.5M7 10V10.1"/>
  </svg>
);

export default function App() {
  const [fonts, setFonts] = useState<string[]>([]);
  const [selected, setSelected] = useState("");
  const [activeFont, setActiveFont] = useState("");
  const [isActive, setIsActive] = useState(false);
  const [btnState, setBtnState] = useState<BtnState>("idle");
  const [error, setError] = useState("");
  const [query, setQuery] = useState("");
  const [kbIndex, setKbIndex] = useState(-1);

  const listRef = useRef<HTMLDivElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  const itemRefs = useRef<Map<number, HTMLDivElement>>(new Map());

  const filtered = query.trim()
    ? fonts.filter(f => f.toLowerCase().includes(query.toLowerCase()))
    : fonts;

  useEffect(() => {
    GetFonts().then((list: string[]) => {
      setFonts(list);
      if (list.length) setSelected(list[0]);
    });

    GetStatus().then(({ font, active }: { font: string; active: boolean }) => {
      setActiveFont(font);
      setIsActive(active);
      if (active && font) setSelected(font);
    });

    const unsub = EventsOn("fontChanged", (...args: unknown[]) => {
      const [font, active] = args as [string, boolean];
      setActiveFont(font);
      setIsActive(active);
    });
    return unsub;
  }, []);

  // Reset kb index when filtered list changes
  useEffect(() => {
    setKbIndex(-1);
  }, [query]);

  // Scroll keyboard-focused item into view
  useEffect(() => {
    if (kbIndex >= 0) {
      const el = itemRefs.current.get(kbIndex);
      el?.scrollIntoView({ block: "nearest", behavior: "smooth" });
    }
  }, [kbIndex]);

  const handleKeyDown = useCallback((e: React.KeyboardEvent<HTMLInputElement>) => {
    if (filtered.length === 0) return;

    if (e.key === "ArrowDown") {
      e.preventDefault();
      setKbIndex(i => Math.min(i + 1, filtered.length - 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setKbIndex(i => Math.max(i - 1, 0));
    } else if (e.key === "Enter") {
      e.preventDefault();
      const idx = kbIndex >= 0 ? kbIndex : 0;
      setSelected(filtered[idx]);
      setKbIndex(-1);
    } else if (e.key === "Escape") {
      setQuery("");
      searchRef.current?.blur();
    }
  }, [filtered, kbIndex]);

  async function handleApply() {
    if (!selected || btnState !== "idle") return;
    setError("");
    setBtnState("applying");
    try {
      await ApplyFont(selected);
    } catch (e) {
      setError(String(e));
    } finally {
      setBtnState("idle");
    }
  }

  async function handleReset() {
    if (btnState !== "idle") return;
    setError("");
    setBtnState("resetting");
    try {
      await ResetFont();
    } catch (e) {
      setError(String(e));
    } finally {
      setBtnState("idle");
    }
  }

  const busy = btnState !== "idle";

  return (
    <div className="app">
      {/* Header */}
      <div className="header">
        <div className="header-icon">
          <IconFont />
        </div>
        <div className="header-text">
          <h1>Font Override</h1>
          <p>System-wide typography control</p>
        </div>
      </div>

      {/* Status */}
      <div className={`status-bar${isActive ? " is-active" : ""}`}>
        <div className="status-left">
          <span className="status-dot" />
          <span className="status-label">{isActive ? "Override active" : "Not active"}</span>
        </div>
        {isActive && activeFont && (
          <span className="status-font-name">{activeFont}</span>
        )}
      </div>

      <div className="divider" />

      {/* Font picker */}
      <div className="font-section">
        <div className="section-label">Select Font</div>
        <div className="picker-box">
          {/* Search */}
          <div className="font-search">
            <IconSearch />
            <input
              ref={searchRef}
              type="text"
              placeholder="Search fonts…"
              value={query}
              onChange={e => setQuery(e.target.value)}
              onKeyDown={handleKeyDown}
              disabled={busy}
              autoComplete="off"
              spellCheck={false}
            />
            {query && (
              <button className="search-clear" onClick={() => { setQuery(""); searchRef.current?.focus(); }}>
                <IconX />
              </button>
            )}
          </div>

          {/* List */}
          <div className="font-list" ref={listRef}>
            {filtered.length === 0 ? (
              <div className="font-empty">No fonts match "{query}"</div>
            ) : (
              filtered.map((f, i) => (
                <div
                  key={f}
                  ref={el => { if (el) itemRefs.current.set(i, el); else itemRefs.current.delete(i); }}
                  className={`font-item${f === selected ? " selected" : ""}${i === kbIndex ? " keyboard-focus" : ""}`}
                  onClick={() => { setSelected(f); setKbIndex(-1); }}
                >
                  <span>{f}</span>
                  <IconCheck />
                </div>
              ))
            )}
          </div>
        </div>
      </div>

      {/* Live preview */}
      {selected && (
        <div className="preview-box">
          <span className="preview-label">Preview</span>
          <span className="preview-text" style={{ fontFamily: `'${selected}', system-ui` }}>
            The quick brown fox jumps over the lazy dog
          </span>
        </div>
      )}

      {/* Actions */}
      <div className="actions">
        <button
          className="btn primary"
          onClick={handleApply}
          disabled={busy || !selected}
        >
          {btnState === "applying" ? <><span className="spinner" />Applying…</> : "Apply Font"}
        </button>
        <button
          className="btn"
          onClick={handleReset}
          disabled={busy || !isActive}
        >
          {btnState === "resetting" ? <><span className="spinner" />Resetting…</> : "Reset"}
        </button>
      </div>

      {/* Error */}
      {error && (
        <div className="toast error">
          <IconAlert />
          {error}
        </div>
      )}

      {/* First-use hint */}
      {!isActive && !error && (
        <div className="toast hint">
          <IconInfo />
          First use: restart Chrome once after applying to load the extension.
        </div>
      )}
    </div>
  );
}