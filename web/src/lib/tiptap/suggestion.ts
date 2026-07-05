// suggestion — the LIVE-EDITOR `#`-tag + `/`-slash affordances (RE-03, D-02).
//
// Two @tiptap/suggestion plugins, each wired through an Extension's
// addProseMirrorPlugins:
//   • TagSuggest (char "#") — offers the workspace tag VOCABULARY
//     (getTagVocabulary() → GET /api/v1/tags/vocabulary) and inserts a DISPLAY-only
//     `tag` node (tagNode.ts). Tag DATA still materializes through the ONE Lezer
//     recognizer on save (the node serializes to literal `#tag`); this is a typing
//     affordance, not a second materializer.
//   • SlashCommands (char "/") — inserts bounded-slice blocks (heading/list/code/
//     quote) via EXISTING schema commands only (no new schema node → serialization
//     unchanged; a node absent from buildExtensions() would break round-trip).
//
// SECURITY (MAJ-03, popup-text-never-innerHTML): the popup DOM is 100%
// developer-supplied — @tiptap/suggestion has no built-in list renderer. Every item
// label is written via `textContent` / DOM text nodes, NEVER `el.innerHTML`, so a
// markup-bearing vocabulary tag (e.g. `<img onerror=…>`) stays INERT text — matching
// the shipped TagSuggest.tsx:442-443 locked stored-XSS guard. There is no innerHTML
// anywhere in this module (acceptance-checked).
import { Extension } from "@tiptap/core";
import type { Editor, Range } from "@tiptap/core";
import Suggestion from "@tiptap/suggestion";
import type {
  SuggestionProps,
  SuggestionKeyDownProps,
} from "@tiptap/suggestion";
import { PluginKey } from "@tiptap/pm/state";

// MAX_SUGGESTIONS caps the popup list so a large vocabulary can't render an unbounded
// number of rows (a keystroke-latency + DOM-size guard). The `#`-suggest is a
// prefix-filtered convenience, not a full tag browser.
const MAX_SUGGESTIONS = 20;

// SuggestPopup is the imperative handle over one open popup element. It owns the
// selection index + keyboard navigation; the render lifecycle below drives it.
interface SuggestPopup<I> {
  el: HTMLDivElement;
  update(items: I[]): void;
  move(delta: number): void;
  current(): I | undefined;
  remove(): void;
}

// createSuggestPopup builds the developer-supplied popup element (MAJ-03). It renders
// each item as ONE row whose text is set via `textContent` — never innerHTML — so a
// tag name carrying markup can never become live DOM. onSelect fires on click/Enter.
function createSuggestPopup<I>(
  labelOf: (item: I) => string,
  onSelect: (item: I) => void,
): SuggestPopup<I> {
  const el = document.createElement("div");
  el.className = "tiptap-suggest";
  el.setAttribute("role", "listbox");
  let items: I[] = [];
  let selected = 0;

  const paint = (): void => {
    el.replaceChildren();
    items.forEach((item, i) => {
      const row = document.createElement("div");
      row.className = i === selected ? "tiptap-suggest-item is-selected" : "tiptap-suggest-item";
      row.setAttribute("role", "option");
      row.setAttribute("aria-selected", String(i === selected));
      // MAJ-03: the item label flows through textContent (an inert text node), NEVER
      // el.innerHTML. A vocabulary tag with markup stays literal text in the popup.
      row.textContent = labelOf(item);
      // mousedown (not click) so the editor selection is not lost before we act; the
      // preventDefault keeps focus in the editor while the command runs.
      row.addEventListener("mousedown", (event) => {
        event.preventDefault();
        onSelect(item);
      });
      el.appendChild(row);
    });
  };

  return {
    el,
    update(next) {
      items = next;
      if (selected >= items.length) selected = Math.max(0, items.length - 1);
      paint();
    },
    move(delta) {
      if (items.length === 0) return;
      selected = (selected + delta + items.length) % items.length;
      paint();
    },
    current() {
      return items[selected];
    },
    remove() {
      el.remove();
    },
  };
}

// positionPopup anchors the popup under the suggestion's cursor rect. clientRect may be
// null (SSR / no layout / jsdom) — then the popup mounts un-positioned, which is fine
// for correctness (and for the headless tests, which assert on content, not geometry).
function positionPopup(
  el: HTMLElement,
  clientRect: (() => DOMRect | null) | null | undefined,
): void {
  el.style.position = "absolute";
  const rect = clientRect?.();
  if (!rect) return;
  el.style.left = `${rect.left + window.scrollX}px`;
  el.style.top = `${rect.bottom + window.scrollY}px`;
}

// renderSuggestionPopup returns the @tiptap/suggestion render() lifecycle backed by the
// textContent-only popup. It is shared by BOTH the `#` and `/` plugins (item type I ===
// TSelected in both, so a selected item is passed straight to props.command). onKeyDown
// returns true to intercept navigation/commit keys, false to let the editor handle the key.
function renderSuggestionPopup<I>(labelOf: (item: I) => string) {
  return () => {
    let popup: SuggestPopup<I> | null = null;
    // command is captured from the latest onStart/onUpdate props: SuggestionKeyDownProps
    // (onKeyDown) carries only view/event/range, so the commit-on-Enter path reads the
    // command through this closure, kept in sync with the live suggestion state.
    let command: ((item: I) => void) | null = null;
    return {
      onStart: (props: SuggestionProps<I, I>) => {
        command = props.command;
        popup = createSuggestPopup<I>(labelOf, (item) => props.command(item));
        popup.update(props.items);
        document.body.appendChild(popup.el);
        positionPopup(popup.el, props.clientRect);
      },
      onUpdate: (props: SuggestionProps<I, I>) => {
        command = props.command;
        if (!popup) return;
        popup.update(props.items);
        positionPopup(popup.el, props.clientRect);
      },
      onKeyDown: (props: SuggestionKeyDownProps): boolean => {
        if (!popup) return false;
        const key = props.event.key;
        if (key === "ArrowDown") {
          popup.move(1);
          return true;
        }
        if (key === "ArrowUp") {
          popup.move(-1);
          return true;
        }
        if (key === "Enter") {
          const item = popup.current();
          if (item !== undefined) command?.(item);
          return true;
        }
        if (key === "Escape") {
          popup.remove();
          popup = null;
          return true;
        }
        return false;
      },
      onExit: () => {
        popup?.remove();
        popup = null;
        command = null;
      },
    };
  };
}

// TagSuggestOptions injects the vocabulary lazily: the live surface fetches it once
// (getTagVocabulary) and passes a getter, so the items() closure always reads the
// freshest list without re-creating the extension.
export interface TagSuggestOptions {
  getVocabulary: () => string[];
}

// TagSuggest — the `#`-triggered tag-vocabulary suggestion. Selecting an item inserts a
// DISPLAY-only `tag` node (name = the chosen vocabulary string) plus a trailing space so
// the caret escapes the inline atom. The default allowedPrefixes ([" "]) enforces the
// same start-of-block/after-whitespace boundary as the Lezer grammar's `#` start rule.
export const TagSuggest = Extension.create<TagSuggestOptions>({
  name: "tagSuggest",

  addOptions() {
    return { getVocabulary: () => [] };
  },

  addProseMirrorPlugins() {
    const getVocabulary = this.options.getVocabulary;
    return [
      Suggestion<string, string>({
        editor: this.editor,
        char: "#",
        pluginKey: new PluginKey("tagSuggest"),
        items: ({ query }) => {
          const q = query.toLowerCase();
          return getVocabulary()
            .filter((tag) => tag.toLowerCase().startsWith(q))
            .slice(0, MAX_SUGGESTIONS);
        },
        command: ({ editor, range, props }) => {
          editor
            .chain()
            .focus()
            .deleteRange(range)
            .insertContent([
              { type: "tag", attrs: { name: props } },
              { type: "text", text: " " },
            ])
            .run();
        },
        render: renderSuggestionPopup<string>((tag) => tag),
      }),
    ];
  },
});

// SlashItem is one `/`-slash block command: a display label + the transform to run. The
// transform runs an EXISTING bounded-slice schema command (heading/list/code/quote) after
// deleting the typed `/query` — it adds NO new schema node, so serialization is unchanged.
interface SlashItem {
  label: string;
  run: (opts: { editor: Editor; range: Range }) => void;
}

// SLASH_ITEMS — the bounded-slice block affordances. Each `run` deletes the `/query` and
// applies a schema command that already exists in buildExtensions() (StarterKit +
// extension-list). A command for a node NOT in the bounded slice would break round-trip,
// so the set is deliberately limited to modeled constructs.
const SLASH_ITEMS: SlashItem[] = [
  {
    label: "Heading 1",
    run: ({ editor, range }) =>
      editor.chain().focus().deleteRange(range).setNode("heading", { level: 1 }).run(),
  },
  {
    label: "Heading 2",
    run: ({ editor, range }) =>
      editor.chain().focus().deleteRange(range).setNode("heading", { level: 2 }).run(),
  },
  {
    label: "Heading 3",
    run: ({ editor, range }) =>
      editor.chain().focus().deleteRange(range).setNode("heading", { level: 3 }).run(),
  },
  {
    label: "Bullet list",
    run: ({ editor, range }) =>
      editor.chain().focus().deleteRange(range).toggleBulletList().run(),
  },
  {
    label: "Numbered list",
    run: ({ editor, range }) =>
      editor.chain().focus().deleteRange(range).toggleOrderedList().run(),
  },
  {
    label: "Code block",
    run: ({ editor, range }) =>
      editor.chain().focus().deleteRange(range).setCodeBlock().run(),
  },
  {
    label: "Quote",
    run: ({ editor, range }) =>
      editor.chain().focus().deleteRange(range).toggleBlockquote().run(),
  },
];

// SlashCommands — the `/`-triggered block-insert menu. command runs the selected item's
// transform (UI-only over the existing schema). The popup is the SAME textContent-only
// element as the tag suggest (MAJ-03).
export const SlashCommands = Extension.create({
  name: "slashCommands",

  addProseMirrorPlugins() {
    return [
      Suggestion<SlashItem, SlashItem>({
        editor: this.editor,
        char: "/",
        pluginKey: new PluginKey("slashCommands"),
        items: ({ query }) => {
          const q = query.toLowerCase();
          return SLASH_ITEMS.filter((item) => item.label.toLowerCase().includes(q));
        },
        command: ({ editor, range, props }) => props.run({ editor, range }),
        render: renderSuggestionPopup<SlashItem>((item) => item.label),
      }),
    ];
  },
});
