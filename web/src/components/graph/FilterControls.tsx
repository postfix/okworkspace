// FilterControls (GF-01): compact inline controls — search icon with popover,
// visibility toggles, display sliders in a popover, groups + reset as icon buttons.
// Lives in the same flex row as the Graph title + EdgeToggles.
import { useState, useRef, useEffect } from "react";
import { Search, SlidersHorizontal, Plus, RotateCcw } from "lucide-react";
import { useGraphFilters } from "../../stores/graphFilters";
import "./FilterControls.css";

export default function FilterControls() {
  const searchQuery = useGraphFilters((s) => s.searchQuery);
  const showTags = useGraphFilters((s) => s.showTags);
  const showAttachments = useGraphFilters((s) => s.showAttachments);
  const existingFilesOnly = useGraphFilters((s) => s.existingFilesOnly);
  const showOrphans = useGraphFilters((s) => s.showOrphans);
  const showArrows = useGraphFilters((s) => s.showArrows);
  const textFadeThreshold = useGraphFilters((s) => s.textFadeThreshold);
  const nodeSize = useGraphFilters((s) => s.nodeSize);
  const linkThickness = useGraphFilters((s) => s.linkThickness);
  const setSearchQuery = useGraphFilters((s) => s.setSearchQuery);
  const toggleTag = useGraphFilters((s) => s.toggleTag);
  const toggleAttachments = useGraphFilters((s) => s.toggleAttachments);
  const toggleExistingFilesOnly = useGraphFilters((s) => s.toggleExistingFilesOnly);
  const toggleOrphans = useGraphFilters((s) => s.toggleOrphans);
  const toggleArrows = useGraphFilters((s) => s.toggleArrows);
  const setTextFadeThreshold = useGraphFilters((s) => s.setTextFadeThreshold);
  const setNodeSize = useGraphFilters((s) => s.setNodeSize);
  const setLinkThickness = useGraphFilters((s) => s.setLinkThickness);
  const addGroup = useGraphFilters((s) => s.addGroup);
  const reset = useGraphFilters((s) => s.reset);

  const [searchOpen, setSearchOpen] = useState(false);
  const [displayOpen, setDisplayOpen] = useState(false);
  const searchRef = useRef<HTMLDivElement>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const displayRef = useRef<HTMLDivElement>(null);

  // Focus search input when popover opens.
  useEffect(() => {
    if (searchOpen) searchInputRef.current?.focus();
  }, [searchOpen]);

  // Close popovers on outside click.
  useEffect(() => {
    function onDown(e: MouseEvent) {
      if (searchRef.current && !searchRef.current.contains(e.target as Node))
        setSearchOpen(false);
      if (displayRef.current && !displayRef.current.contains(e.target as Node))
        setDisplayOpen(false);
    }
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, []);

  return (
    <div className="filtercontrols" role="group" aria-label="Graph filters">
      {/* Visibility toggle chips — same chip pattern as EdgeToggles */}
      <ToggleChip label="Tags" on={showTags} onToggle={toggleTag} />
      <ToggleChip label="Attachments" on={showAttachments} onToggle={toggleAttachments} />
      <ToggleChip label="Existing" on={existingFilesOnly} onToggle={toggleExistingFilesOnly} />
      <ToggleChip label="Orphans" on={showOrphans} onToggle={toggleOrphans} />
      <ToggleChip label="Arrows" on={showArrows} onToggle={toggleArrows} />

      {/* Search icon + popover */}
      <div className="filtercontrols-popover-host" ref={searchRef}>
        <button
          type="button"
          className={`filtercontrols-icon-btn${searchOpen ? " filtercontrols-icon-btn-active" : ""}${searchQuery ? " filtercontrols-icon-btn-on" : ""}`}
          onClick={() => { setSearchOpen((v) => !v); setDisplayOpen(false); }}
          aria-label="Search graph"
        >
          <Search size={15} />
        </button>
        {searchOpen && (
          <div className="filtercontrols-popover">
            <input
              ref={searchInputRef}
              type="search"
              className="filtercontrols-popover-search"
              placeholder="Filter by name, tag, alias…"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
            />
          </div>
        )}
      </div>

      {/* Display/settings icon + popover */}
      <div className="filtercontrols-popover-host" ref={displayRef}>
        <button
          type="button"
          className={`filtercontrols-icon-btn${displayOpen ? " filtercontrols-icon-btn-active" : ""}`}
          onClick={() => { setDisplayOpen((v) => !v); setSearchOpen(false); }}
          aria-label="Graph display settings"
        >
          <SlidersHorizontal size={15} />
        </button>
        {displayOpen && (
          <div className="filtercontrols-popover filtercontrols-popover-wide">
            <div className="filtercontrols-popover-sliders">
              <SliderRow label="Text fade" value={textFadeThreshold} min={0} max={1} step={0.01} onChange={setTextFadeThreshold} />
              <SliderRow label="Node size" value={nodeSize} min={0} max={1} step={0.01} onChange={setNodeSize} />
              <SliderRow label="Link thickness" value={linkThickness} min={0} max={1} step={0.01} onChange={setLinkThickness} />
            </div>
            <button type="button" className="filtercontrols-popover-group-btn" onClick={addGroup}>
              <Plus size={14} /> New group
            </button>
          </div>
        )}
      </div>

      {/* Reset */}
      <button
        type="button"
        className="filtercontrols-icon-btn"
        onClick={reset}
        aria-label="Reset graph filters"
      >
        <RotateCcw size={15} />
      </button>
    </div>
  );
}

function ToggleChip({ label, on, onToggle }: { label: string; on: boolean; onToggle: () => void }) {
  return (
    <button
      type="button"
      className={`edgetoggle-chip${on ? " edgetoggle-chip-on" : ""}`}
      aria-pressed={on}
      onClick={onToggle}
    >
      {label}
    </button>
  );
}

function SliderRow({ label, value, min, max, step, onChange }: {
  label: string; value: number; min: number; max: number; step: number;
  onChange: (v: number) => void;
}) {
  return (
    <label className="filtercontrols-popover-slider">
      <span className="filtercontrols-popover-slider-label">{label}</span>
      <input
        type="range"
        value={value}
        min={min}
        max={max}
        step={step}
        onChange={(e) => onChange(parseFloat(e.target.value))}
      />
    </label>
  );
}
