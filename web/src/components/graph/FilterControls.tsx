// FilterControls (GF-01): inline controls — visibility toggles, always-visible
// search, always-visible display sliders, groups + reset. Lives in the
// graphview-header flex row alongside the Graph title and EdgeToggles.
import { useRef } from "react";
import { Plus, RotateCcw } from "lucide-react";
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

  const searchRef = useRef<HTMLInputElement>(null);

  return (
    <div className="filtercontrols" role="group" aria-label="Graph filters">
      {/* Visibility toggle chips */}
      <ToggleChip label="Tags" on={showTags} onToggle={toggleTag} />
      <ToggleChip label="Attachments" on={showAttachments} onToggle={toggleAttachments} />
      <ToggleChip label="Existing" on={existingFilesOnly} onToggle={toggleExistingFilesOnly} />
      <ToggleChip label="Orphans" on={showOrphans} onToggle={toggleOrphans} />
      <ToggleChip label="Arrows" on={showArrows} onToggle={toggleArrows} />

      {/* Inline search — always visible */}
      <input
        ref={searchRef}
        type="search"
        className="filtercontrols-search"
        placeholder="Filter by name, tag, alias…"
        value={searchQuery}
        onChange={(e) => setSearchQuery(e.target.value)}
      />

      {/* Inline display sliders — always visible, compact horizontal */}
      <div className="filtercontrols-sliders" role="group" aria-label="Display settings">
        <SliderRow label="Fade" value={textFadeThreshold} min={0} max={1} step={0.01} onChange={setTextFadeThreshold} />
        <SliderRow label="Size" value={nodeSize} min={0} max={1} step={0.01} onChange={setNodeSize} />
        <SliderRow label="Thick" value={linkThickness} min={0} max={1} step={0.01} onChange={setLinkThickness} />
      </div>

      {/* New group */}
      <button type="button" className="filtercontrols-group-btn" onClick={addGroup}>
        <Plus size={14} /> New group
      </button>

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
    <label className="filtercontrols-slider">
      <span className="filtercontrols-slider-label">{label}</span>
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
