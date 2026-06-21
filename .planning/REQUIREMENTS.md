# Requirements: OKF Workspace

**Defined:** 2026-06-17
**Core Value:** A non-technical teammate can create, edit, and find knowledge — and get useful AI help on it — while every byte stays as plain Markdown + original files on disk, versioned in Git, with no proprietary store.

## v1 Requirements

Full MVP per SPEC.md (Phases 0–5). Each maps to a roadmap phase.

### Authentication & Users

- [x] **AUTH-01**: User can log in with a username and password
- [x] **AUTH-02**: User can log out from any page
- [x] **AUTH-03**: User session persists across browser refresh via a secure cookie
- [x] **AUTH-04**: An admin user is created automatically on first startup
- [x] **AUTH-05**: User actions are gated by role (admin / editor / reader)
- [x] **AUTH-06**: User has a display name shown in the UI

### Pages

- [x] **PAGE-01**: User can create a page in the selected folder
- [x] **PAGE-02**: User can edit a page's title, tags, description, and body
- [x] **PAGE-03**: User can save a page and view it rendered as Markdown
- [x] **PAGE-04**: User can rename a page
- [x] **PAGE-05**: User can move a page to another folder
- [x] **PAGE-06**: User can delete a page to trash
- [x] **PAGE-07**: User can restore a page from trash
- [x] **PAGE-08**: User can link from one page to another
- [ ] **PAGE-09**: When a page is saved with missing required frontmatter, the system fills in the required fields

### Editing (Live-Preview Editor)

- [x] **EDIT-01**: While editing, Markdown formatting (headings, bold/italic, lists, links, inline code, code blocks, inline images, and GFM tables) renders inline in the editor as the user types — not only in a separate preview pane
- [x] **EDIT-02**: User can toggle between live-preview and raw-source modes, and switching modes never alters the underlying Markdown bytes
- [x] **EDIT-03**: Saving from the live-preview editor produces byte-identical Markdown to the source-mode round-trip — the okf golden-corpus exit gate still holds (no lossy block model)
- [x] **EDIT-04**: Existing editor guarantees are preserved — autosave drafts, optimistic-concurrency save, and sanitized rendering (no raw HTML rendered from page content)

### Navigation

- [x] **NAV-01**: User can browse pages in a left-side tree
- [x] **NAV-02**: User can expand and collapse folders in the tree
- [x] **NAV-03**: User can create a folder
- [x] **NAV-04**: User sees the currently open page highlighted in the tree
- [x] **NAV-05**: User can see a list of recently visited pages

### Versioning (hidden Git)

- [ ] **VER-01**: The system commits to Git automatically after a page save, recording the user's identity
- [x] **VER-02**: User can view a page's version history
- [x] **VER-03**: User can restore a previous version of a page
- [x] **VER-04**: The system can push commits to a configured Git remote when enabled

### Attachments

- [ ] **ATT-01**: User can upload a file attachment to a page
- [ ] **ATT-02**: User can download an attachment in its original, unmodified form
- [ ] **ATT-03**: User sees an attachment card with original name, size, uploader, and date
- [ ] **ATT-04**: User can preview image attachments (PNG/JPG/SVG) inline
- [ ] **ATT-05**: User can replace an attachment with a new version
- [ ] **ATT-06**: User can remove an attachment link from a page
- [ ] **ATT-07**: The system deletes an attachment from the repo when no page references it
- [ ] **ATT-08**: The system extracts text from PDF/DOCX/TXT attachments for search and the agent
- [ ] **ATT-09**: Uploads are validated against a size limit and an allowed type (MIME-sniffed)
- [ ] **ATT-10**: The system commits to Git automatically after attachment upload or delete

### Search

- [ ] **SRCH-01**: User can search page titles
- [ ] **SRCH-02**: User can search page body full text
- [ ] **SRCH-03**: User can search by tag
- [ ] **SRCH-04**: User can search attachment filenames
- [ ] **SRCH-05**: User can search extracted attachment text
- [ ] **SRCH-06**: Search returns typed results (page / attachment / heading)

### Agent (Eino)

- [ ] **AGNT-01**: User can ask the agent a question about the current page
- [ ] **AGNT-02**: User can ask the agent about selected text
- [ ] **AGNT-03**: User can ask the agent about a selected attachment
- [ ] **AGNT-04**: User can ask the agent about the whole workspace
- [ ] **AGNT-05**: User can ask the agent to summarize a page
- [ ] **AGNT-06**: User can ask the agent to summarize an attachment
- [ ] **AGNT-07**: User can ask the agent to rewrite selected text and see the proposal
- [ ] **AGNT-08**: User can ask the agent to draft a new page
- [ ] **AGNT-09**: User can ask the agent to propose a patch to the current page, shown as a diff
- [ ] **AGNT-10**: User must explicitly approve an agent patch before it is applied and committed
- [ ] **AGNT-11**: The agent cannot write files directly, read secrets, run shell, escape the repository, or push to Git (enforced in the tool layer, not by prompt)

### Collaboration

- [ ] **COLL-01**: User can see when another user is currently editing a page (presence)
- [ ] **COLL-02**: The system applies a soft lock when a page is being edited; a user can still force-edit
- [ ] **COLL-03**: Saves use optimistic concurrency with a per-document revision
- [ ] **COLL-04**: On a save conflict, the user is shown a diff and can choose overwrite, manual merge, or save-as-copy

### Security & Audit

- [x] **SEC-01**: All file paths are resolved through a safe resolver that blocks `../`, absolute paths, and symlink escape
- [ ] **SEC-02**: Risky attachment downloads are served with `Content-Disposition: attachment`
- [x] **SEC-03**: Passwords are hashed with Argon2id (or bcrypt)
- [x] **SEC-04**: Session cookies are HTTPOnly with SameSite set, and mutating requests are CSRF-protected
- [x] **SEC-05**: Key actions (login, page/attachment changes, agent prompts and approvals, config changes) are recorded in an audit log

## v2 Requirements

Deferred to a future release. Tracked but not in the current roadmap.

### Attachments

- **ATT2-01**: Text extraction for XLSX and ZIP attachments
- **ATT2-02**: OCR for scanned / image-only PDFs

### Editing

- **EDIT2-01**: TipTap rich block editor (gated on Markdown round-trip tests passing)

### Collaboration

- **COLL2-01**: Real-time collaborative editing (Yjs/CRDT, presence cursors, collaboration rooms)

## Out of Scope

Explicitly excluded (SPEC §4). Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Notion-style databases, Kanban boards, DB-object tables | Not core to an open-Markdown wiki; high complexity |
| Public sharing / web publishing | Internal tool; outside MVP value |
| Comments | Not core to MVP knowledge capture |
| Direct agent writes without user approval | Violates the safety model — all agent writes require human review |
| In-browser editing of DOCX/PDF | Originals are immutable; only extracted text is read |
| Mobile native app | Web-first |
| Per-paragraph permission model | Over-complex for a 5-person team |
| Enterprise SSO | Local username/password is sufficient for v1 |
| Complex workflow automation | Not core to MVP |

## Traceability

Which phases cover which requirements. **Populated during roadmap creation.**

| Requirement | Phase | Status |
|-------------|-------|--------|
| AUTH-01 | Phase 0 | Complete |
| AUTH-02 | Phase 0 | Complete |
| AUTH-03 | Phase 0 | Complete |
| AUTH-04 | Phase 0 | Complete |
| AUTH-05 | Phase 0 | Complete |
| AUTH-06 | Phase 0 | Complete |
| SEC-01 | Phase 0 | Complete |
| SEC-03 | Phase 0 | Complete |
| SEC-04 | Phase 0 | Complete |
| SEC-05 | Phase 0 | Complete |
| PAGE-01 | Phase 1 | Complete |
| PAGE-02 | Phase 1 | Complete |
| PAGE-03 | Phase 1 | Complete |
| PAGE-04 | Phase 1 | Complete |
| PAGE-05 | Phase 1 | Complete |
| PAGE-06 | Phase 1 | Complete |
| PAGE-07 | Phase 1 | Complete |
| PAGE-08 | Phase 1 | Complete |
| PAGE-09 | Phase 1 | Pending |
| NAV-01 | Phase 1 | Complete |
| NAV-02 | Phase 1 | Complete |
| NAV-03 | Phase 1 | Complete |
| NAV-04 | Phase 1 | Complete |
| NAV-05 | Phase 1 | Complete |
| VER-01 | Phase 1 | Pending |
| VER-02 | Phase 1 | Complete |
| VER-03 | Phase 1 | Complete |
| VER-04 | Phase 1 | Complete |
| ATT-01 | Phase 2 | Pending |
| ATT-02 | Phase 2 | Pending |
| ATT-03 | Phase 2 | Pending |
| ATT-04 | Phase 2 | Pending |
| ATT-05 | Phase 2 | Pending |
| ATT-06 | Phase 2 | Pending |
| ATT-07 | Phase 2 | Pending |
| ATT-08 | Phase 2 | Pending |
| ATT-09 | Phase 2 | Pending |
| ATT-10 | Phase 2 | Pending |
| SEC-02 | Phase 2 | Pending |
| SRCH-01 | Phase 3 | Pending |
| SRCH-02 | Phase 3 | Pending |
| SRCH-03 | Phase 3 | Pending |
| SRCH-04 | Phase 3 | Pending |
| SRCH-05 | Phase 3 | Pending |
| SRCH-06 | Phase 3 | Pending |
| AGNT-01 | Phase 4 | Pending |
| AGNT-02 | Phase 4 | Pending |
| AGNT-03 | Phase 4 | Pending |
| AGNT-04 | Phase 4 | Pending |
| AGNT-05 | Phase 4 | Pending |
| AGNT-06 | Phase 4 | Pending |
| AGNT-07 | Phase 4 | Pending |
| AGNT-08 | Phase 4 | Pending |
| AGNT-09 | Phase 4 | Pending |
| AGNT-10 | Phase 4 | Pending |
| AGNT-11 | Phase 4 | Pending |
| COLL-01 | Phase 5 | Pending |
| COLL-02 | Phase 5 | Pending |
| COLL-03 | Phase 5 | Pending |
| COLL-04 | Phase 5 | Pending |
| EDIT-01 | Phase 6 | Complete |
| EDIT-02 | Phase 6 | Complete |
| EDIT-03 | Phase 6 | Complete |
| EDIT-04 | Phase 6 | Complete |

**Coverage:**

- v1 requirements: 64 total (60 original + 4 EDIT added with Phase 6, 2026-06-21)
- Mapped to phases: 64 ✓
- Unmapped: 0 ✓

**Per-phase counts:** Phase 0 = 10 · Phase 1 = 18 · Phase 2 = 11 · Phase 3 = 6 · Phase 4 = 11 · Phase 5 = 4 · Phase 6 = 4 (total 64). Phase 7 (Obsidian-style File Tree) requirements to be formalized at its plan time.

---
*Requirements defined: 2026-06-17*
*Last updated: 2026-06-17 after roadmap creation (traceability filled, 60/60 mapped)*
