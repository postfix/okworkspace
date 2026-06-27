// FilterControls (GF-01): search input, visibility toggles, display sliders,
// groups button, and reset button — all live flat in the same panel as the
// EdgeToggles chip cluster (no collapsing, no sidebar, no Apply button).
//
// Every control reads/writes through useGraphFilters; changes propagate live
// through the client-side filtering pipeline in GraphView.
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

  return (
    <div className="filtercontrols" role="group" aria-label="Graph filters">
      {/* Search input */}
      <div className="filtercontrols-search">
        <span className="filtercontrols-search-icon" aria-hidden="true">
          &#x2315;
        </span>
        <input
          type="search"
          className="filtercontrols-search-input"
          placeholder="Search files..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          aria-label="Search graph files"
        />
      </div>

      {/* Visibility toggles — same chip pattern as EdgeToggles */}
      <div
        className="filtercontrols-toggles"
        role="group"
        aria-label="Visibility toggles"
      >
        <ToggleChip label="Tags" on={showTags} onToggle={toggleTag} />
        <ToggleChip
          label="Attachments"
          on={showAttachments}
          onToggle={toggleAttachments}
        />
        <ToggleChip
          label="Existing files only"
          on={existingFilesOnly}
          onToggle={toggleExistingFilesOnly}
        />
        <ToggleChip label="Orphans" on={showOrphans} onToggle={toggleOrphans} />
      </div>

      {/* Display controls */}
      <div className="filtercontrols-display">
        <ToggleChip label="Arrows" on={showArrows} onToggle={toggleArrows} />
        <SliderRow
          label="Text fade threshold"
          value={textFadeThreshold}
          min={0}
          max={1}
          step={0.01}
          onChange={setTextFadeThreshold}
        />
        <SliderRow
          label="Node size"
          value={nodeSize}
          min={0}
          max={1}
          step={0.01}
          onChange={setNodeSize}
        />
        <SliderRow
          label="Link thickness"
          value={linkThickness}
          min={0}
          max={1}
          step={0.01}
          onChange={setLinkThickness}
        />
      </div>

      {/* Groups */}
      <button
        type="button"
        className="filtercontrols-group-btn"
        onClick={addGroup}
      >
        New group
      </button>

      {/* Reset */}
      <button
        type="button"
        className="filtercontrols-reset"
        onClick={reset}
        aria-label="Reset graph filters"
      >
        &#x21BB; Reset
      </button>
    </div>
  );
}

function ToggleChip({
  label,
  on,
  onToggle,
}: {
  label: string;
  on: boolean;
  onToggle: () => void;
}) {
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

function SliderRow({
  label,
  value,
  min,
  max,
  step,
  onChange,
}: {
  label: string;
  value: number;
  min: number;
  max: number;
  step: number;
  onChange: (v: number) => void;
}) {
  return (
    <div className="filtercontrols-slider">
      <label className="filtercontrols-slider-label">{label}</label>
      <input
        type="range"
        className="filtercontrols-slider-input"
        value={value}
        min={min}
        max={max}
        step={step}
        onChange={(e) => onChange(parseFloat(e.target.value))}
        aria-label={label}
      />
    </div>
  );
}
