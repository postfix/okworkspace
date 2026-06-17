# adhd
Open Knowledge Format (OKF) Collaborative AI Assisted Editing  wiki.

# OKF Workspace — Product and Technical Specification

Status: Draft v0.1
Target users: Small internal team, approximately 5 people
Backend: Go
Agent framework: CloudWeGo Eino
Frontend: TypeScript
Storage model: OKF-compatible Markdown files plus first-class attachments
Deployment goal: Single lightweight service, easy to run on a small VPS, homelab server, or internal VM

---

# 1. Product Summary

OKF Workspace is a lightweight internal wiki designed for the agent era.

It is not a full Notion clone. It is a small, fast, self-hosted workspace where human-readable Markdown files are the primary source of truth, Git provides version history, attachments remain downloadable as original files, and an AI agent can read, summarize, search, and propose edits to the knowledge base.

The system should feel simple for non-technical users. Users should not need to understand Git, Markdown internals, branches, commits, pull requests, or file paths.

The system should still keep the data open, portable, and agent-readable.

---

# 2. Core Idea

The product has three main UI areas:

```text
┌─────────────────────┬──────────────────────────────────────┐
│ Left navigation     │ Center read/edit pane                 │
│                     │                                      │
│ Wiki tree           │ Current OKF document                  │
│ Search              │ Markdown rendering / editing          │
│ Attachments         │ Attachment cards                      │
│ Recent pages        │                                      │
├─────────────────────┴──────────────────────────────────────┤
│ Bottom prompt                                               │
│ Ask agent about current page, attachments, or whole wiki     │
└─────────────────────────────────────────────────────────────┘
```

The right-side document outline panel is intentionally excluded from MVP.

If needed later, document outline can be added as a toggleable floating panel.

---

# 3. Product Goals

## 3.1 Human goals

The tool must allow a small team to:

* Create wiki pages.
* Edit wiki pages.
* Navigate pages through a left-side tree.
* Upload attachments.
* Download original attachments.
* Preview common attachments where practical.
* Ask questions about wiki content.
* Ask questions about attachments.
* Ask the agent to summarize, rewrite, or propose page updates.
* Collaborate without knowing Git.
* Recover previous versions.

## 3.2 Agent goals

The tool must allow agents to:

* Read the OKF Markdown repository directly.
* Search pages.
* Read metadata from YAML frontmatter.
* Read extracted text sidecars for uploaded PDF, DOCX, TXT, and other supported documents.
* Propose edits as diffs.
* Create new page drafts.
* Generate cross-links between pages.
* Update indexes or navigation pages.

## 3.3 Operational goals

The tool must be:

* Lightweight.
* Easy to self-host.
* Deployable as one Go binary plus a data directory.
* Usable without PostgreSQL, Redis, Elasticsearch, or Kubernetes.
* Backed up by ordinary filesystem backups and Git.
* Portable to another server by copying the repository and config.

---

# 4. Non-goals for MVP

The MVP will not include:

* Full Notion-style databases.
* Kanban boards.
* Complex tables as database objects.
* Public sharing.
* Mobile native app.
* Complex permission model per paragraph.
* Real-time Google Docs-level collaboration.
* Comments.
* Web publishing.
* Direct agent writes without user approval.
* In-browser editing of DOCX/PDF files.
* Complex workflow automation.
* Enterprise SSO.

---

# 5. Key Design Principles

## 5.1 Files are the source of truth

The primary knowledge base is a directory of Markdown files with YAML frontmatter.

The application must not trap knowledge inside a proprietary database.

## 5.2 Attachments are first-class objects

Attachments are not wiki pages.

A PDF, DOCX, image, spreadsheet, ZIP, or other uploaded file must remain downloadable in its original form.

The system may extract text from attachments for search and AI, but the original file must be preserved.

## 5.3 Git is hidden from users

Git is used for versioning, backup, and audit history.

Users interact through the web UI only.

The backend performs commits automatically.

## 5.4 Agent writes require review

The agent can suggest edits.

The user must approve before the backend applies changes.

## 5.5 Markdown round-trip must be protected

The frontend editor must not silently destroy Markdown structure.

Any rich editor integration must be tested against Markdown import/export.

---

# 6. MVP Feature Set

## 6.1 Pages

MVP must support:

* Create page.
* Rename page.
* Move page.
* Delete page to trash.
* Restore page from trash.
* Edit page title.
* Edit page body.
* Save page.
* View page history.
* Restore old version.
* Search pages.
* Add tags.
* Add description.
* Link to another page.

## 6.2 Navigation

MVP must support:

* Left-side file tree.
* Folder expand/collapse.
* Page search.
* Recent pages.
* Current page highlight.
* Create page inside selected folder.
* Create folder.

## 6.3 Attachments

MVP must support:

* Upload file to current page.
* Download original file.
* Show attachment card.
* Delete attachment link from page.
* Delete attachment from repository if no pages reference it.
* Replace attachment with new version.
* Store attachment metadata.
* Extract text from supported document types.
* Let the agent read extracted text.
* Let user ask: “summarize this attachment.”

Supported MVP formats:

* PDF
* DOCX
* TXT
* Markdown
* PNG
* JPG/JPEG
* SVG
* CSV
* XLSX as upload/download only in MVP
* ZIP as upload/download only in MVP

## 6.4 Agent prompt

The bottom prompt must support:

* Ask about current page.
* Ask about selected text.
* Ask about selected attachment.
* Ask about whole workspace.
* Summarize page.
* Summarize attachment.
* Rewrite selected text.
* Draft a new page.
* Propose patch to current page.
* Apply approved patch.

## 6.5 User management

MVP must support:

* Local username/password login.
* Admin user created on first startup.
* User display name.
* Basic roles:

  * admin
  * editor
  * reader
* Session cookies.
* Logout.

## 6.6 Versioning

MVP must support:

* Automatic Git commit after page save.
* Automatic Git commit after attachment upload/delete.
* Commit metadata containing user identity.
* Page history view.
* Restore previous page version.
* Basic repository health status.

---

# 7. Recommended Architecture

```text
Browser
  │
  │ TypeScript frontend
  ▼
Go HTTP server
  ├── Static frontend assets
  ├── REST API
  ├── WebSocket/SSE status events
  ├── Auth/session service
  ├── OKF file service
  ├── Attachment service
  ├── Search/index service
  ├── Git service
  ├── Agent service using Eino
  └── Job worker
        ├── text extraction
        ├── indexing
        ├── Git commit queue
        └── cleanup tasks

Data directory
  ├── repo/
  │   ├── index.md
  │   ├── runbooks/
  │   ├── architecture/
  │   └── assets/
  ├── app.db
  ├── config.yaml
  ├── logs/
  └── tmp/
```

The backend should be a single Go process.

The frontend should be built with TypeScript and bundled into static assets embedded or served by the Go backend.

---

# 8. Technology Stack

## 8.1 Backend

Language:

```text
Go
```

Main responsibilities:

* HTTP API.
* Static asset serving.
* File operations.
* Git operations.
* Auth/session management.
* Attachment upload/download.
* Text extraction pipeline.
* Search indexing.
* Eino agent orchestration.
* Background jobs.

Recommended Go components:

```text
HTTP router: chi, echo, or standard net/http
Config: YAML or environment variables
Database: SQLite for operational metadata only
Git integration: shell out to git CLI first
Markdown/frontmatter: Goldmark + YAML parser
Search: Bleve or SQLite FTS5
Agent orchestration: Eino
Logging: slog or zerolog
```

Important backend rule:

Content lives in files.

SQLite is only for operational data:

* users
* sessions
* jobs
* indexing cache
* attachment references
* UI preferences
* audit mirror

SQLite must not become the source of truth for wiki content.

## 8.2 Frontend

Language:

```text
TypeScript
```

Recommended stack:

```text
Vite
React or Vue
Plain CSS / Tailwind optional
Markdown renderer
Editor component
File upload component
Prompt/chat component
```

Recommended MVP editor approach:

```text
Phase 1:
Markdown textarea or Markdown-native editor with preview.

Phase 2:
TipTap-based rich editor, but only after Markdown round-trip tests pass.
```

Reason:

The canonical storage is Markdown. A simple Markdown editor is safer for MVP than a rich block editor that may corrupt edge cases.

## 8.3 Agent layer

Agent framework:

```text
CloudWeGo Eino
```

Eino should be used for:

* tool orchestration
* agent workflows
* ReAct-style question answering
* summarization pipeline
* patch proposal pipeline
* attachment Q&A pipeline
* cross-document reasoning

Eino should not be used for basic CRUD.

Basic CRUD should remain plain Go services.

---

# 9. Repository Layout

The workspace repository should look like this:

```text
workspace-repo/
  index.md

  runbooks/
    index.md
    deploy-staging.md
    incident-response.md

  architecture/
    index.md
    auth-flow.md
    data-model.md

  decisions/
    index.md
    adr-0001-use-okf.md
    adr-0002-store-attachments-as-files.md

  assets/
    originals/
      2026/
        06/
          7f3a_contract.pdf
          91bc_network-diagram.png
          a82d_requirements.docx

    extracted/
      7f3a_contract.txt
      a82d_requirements.txt

    metadata/
      7f3a_contract.json
      91bc_network-diagram.json
      a82d_requirements.json

  .okf-workspace/
    manifest.json
    trash/
    locks/
```

The `.okf-workspace/` directory is application-specific metadata.

Agents may read it, but OKF content should not depend on it.

---

# 10. OKF Page Format

Each page is a Markdown file with YAML frontmatter.

Example:

```md
---
type: Runbook
title: Deploying Staging
description: Steps for deploying the staging environment.
resource: https://git.example.local/team/app
tags:
  - devops
  - deployment
timestamp: 2026-06-17T15:00:00Z
---

# Deploying Staging

This runbook explains how to deploy the staging environment.

## Steps

1. Merge into `develop`.
2. Confirm CI passed.
3. Run deployment command.
4. Verify health checks.

## Related attachments

[Deployment checklist](../assets/originals/2026/06/7f3a_deployment-checklist.pdf)
```

Required frontmatter fields:

```yaml
type: Page
title: Human readable title
description: Short summary
tags: []
timestamp: ISO-8601 timestamp
```

Optional fields:

```yaml
resource: External URL or internal resource
owner: User or team
status: draft | active | deprecated | archived
aliases: []
related: []
```

The system must tolerate missing optional fields.

The system should repair missing required fields when a user saves a page.

---

# 11. Attachment Model

Attachments are original files linked from pages.

An attachment has three parts:

```text
1. Original file
2. Metadata JSON
3. Optional extracted text sidecar
```

Example:

```text
assets/
  originals/
    2026/06/7f3a_contract.pdf
  metadata/
    7f3a_contract.json
  extracted/
    7f3a_contract.txt
```

Example metadata:

```json
{
  "id": "7f3a_contract",
  "original_name": "contract.pdf",
  "stored_name": "7f3a_contract.pdf",
  "mime_type": "application/pdf",
  "size_bytes": 482194,
  "sha256": "....",
  "uploaded_by": "janis",
  "uploaded_at": "2026-06-17T15:00:00Z",
  "linked_pages": [
    "legal/vendor-contract.md"
  ],
  "extraction": {
    "status": "done",
    "text_path": "assets/extracted/7f3a_contract.txt",
    "error": null
  }
}
```

Attachment rules:

* Original file must never be modified.
* Download must return the original file.
* File name shown to users must be the original file name.
* Stored file name must be safe and collision-resistant.
* File path must never be directly trusted from user input.
* Large files must be size-limited by config.
* Attachments must be scanned or restricted by MIME/type policy if deployed in a hostile environment.

---

# 12. Search

MVP search should support:

* Page title search.
* Markdown body full-text search.
* Tag search.
* Attachment file name search.
* Extracted attachment text search.

Search result types:

```text
Page result
Attachment result
Heading result
```

Search result example:

```json
{
  "type": "page",
  "path": "runbooks/deploy-staging.md",
  "title": "Deploying Staging",
  "snippet": "Run npm deploy staging after CI passes..."
}
```

Implementation options:

```text
Option A: SQLite FTS5
Option B: Bleve
Option C: simple grep-like search for earliest prototype
```

Recommended MVP:

```text
SQLite FTS5 or Bleve
```

---

# 13. Collaboration Model

## 13.1 MVP collaboration

For MVP, use lightweight collaboration instead of true realtime CRDT editing.

Features:

* Users can see if another user is currently editing a page.
* The system uses soft locks.
* A user can still force edit.
* Saves use optimistic concurrency with document revision.
* If a conflict happens, the backend shows a diff.
* User can choose:

  * overwrite
  * merge manually
  * save as copy

This is much simpler than full realtime collaboration and is enough for a 5-person internal team.

## 13.2 Later realtime collaboration

If realtime collaborative editing becomes necessary, add:

* Yjs document model.
* WebSocket collaboration endpoint.
* Per-document collaboration rooms.
* Cursor awareness.
* Debounced Markdown export.
* Git commit queue.

This should be Phase 2, not MVP.

---

# 14. Git Versioning

Git is managed only by the backend.

Users never run Git commands.

## 14.1 Commit triggers

Commit after:

* Page create.
* Page edit.
* Page rename.
* Page delete.
* Attachment upload.
* Attachment delete.
* Attachment metadata update.
* Agent-approved patch.

## 14.2 Commit batching

Do not commit every keystroke.

Recommended behavior:

```text
Autosave page content every few seconds to local working tree.
Commit after explicit save or after short idle period.
Batch multiple small edits into one commit.
```

Example commit message:

```text
Update runbooks/deploy-staging.md

User: janis
Action: page_edit
Source: web-ui
```

Agent-assisted commit:

```text
Apply agent patch to runbooks/deploy-staging.md

User: janis
Agent: okf-assistant
Action: approved_agent_patch
Prompt: "Update deployment steps from attached checklist"
```

## 14.3 Remote sync

Configurable options:

```yaml
git:
  enabled: true
  remote_enabled: true
  remote: "origin"
  branch: "main"
  push_on_commit: true
  pull_on_startup: true
```

For simple deployment, pushing to a private Gitea/GitLab repository is enough.

---

# 15. Agent Design with Eino

The agent is implemented in Go using Eino.

The agent must use tools instead of direct filesystem access where possible.

## 15.1 Agent tools

Required MVP tools:

```text
list_tree
read_page
search_pages
search_attachments
read_attachment_text
get_current_page_context
propose_page_patch
create_page_draft
summarize_page
summarize_attachment
```

Write tools must be controlled:

```text
apply_page_patch
create_page
attach_file_to_page
```

These require explicit user approval.

## 15.2 Agent modes

MVP modes:

```text
Ask mode:
Read-only. Answers questions.

Summarize mode:
Reads selected page or attachment and creates summary.

Rewrite mode:
Rewrites selected text and shows proposal.

Patch mode:
Creates unified diff for page update.

Create mode:
Drafts a new OKF page.
```

## 15.3 Agent safety rules

The agent must not:

* Delete files directly.
* Modify files without approval.
* Access paths outside workspace repository.
* Read application secrets.
* Execute shell commands unless a future admin-only tool explicitly allows it.
* Push to Git directly.
* Ignore permission checks.
* Download remote URLs by default.

## 15.4 Patch flow

```text
User prompt
  ↓
Agent reads page + relevant context
  ↓
Agent creates proposed patch
  ↓
Backend validates patch
  ↓
UI shows diff
  ↓
User approves
  ↓
Backend applies patch
  ↓
Backend commits to Git
```

---

# 16. Backend Service Design

Recommended Go packages/services:

```text
cmd/okf-workspace/
  main.go

internal/config/
internal/server/
internal/auth/
internal/users/
internal/repo/
internal/okf/
internal/attachments/
internal/search/
internal/gitstore/
internal/agent/
internal/jobs/
internal/audit/
internal/web/
```

## 16.1 Main backend modules

### Auth service

Responsibilities:

* Login.
* Logout.
* Session validation.
* Password hashing.
* Role checks.

### Repo service

Responsibilities:

* Resolve safe paths.
* Read Markdown files.
* Write Markdown files.
* Create folders.
* Rename files.
* Move files.
* Delete files to trash.
* List tree.

### OKF service

Responsibilities:

* Parse YAML frontmatter.
* Validate required fields.
* Render Markdown.
* Normalize metadata.
* Generate new page templates.

### Attachment service

Responsibilities:

* Accept uploads.
* Validate file size.
* Validate file type.
* Store original file.
* Create metadata JSON.
* Link file to page.
* Serve downloads.
* Queue text extraction job.

### Search service

Responsibilities:

* Index Markdown pages.
* Index attachment text sidecars.
* Search by title, tag, body, and attachment text.

### Git service

Responsibilities:

* Detect repository state.
* Initialize repository.
* Add files.
* Commit changes.
* Push to remote.
* Read history.
* Restore previous versions.

### Agent service

Responsibilities:

* Build Eino agent.
* Register tools.
* Handle prompt requests.
* Stream responses.
* Produce patches.
* Enforce read/write approval boundary.

### Job service

Responsibilities:

* Run text extraction.
* Rebuild search index.
* Run Git commit queue.
* Cleanup temp files.
* Retry failed jobs.

---

# 17. API Specification

Base path:

```text
/api/v1
```

## 17.1 Auth

```http
POST /api/v1/auth/login
POST /api/v1/auth/logout
GET  /api/v1/auth/me
```

Login request:

```json
{
  "username": "janis",
  "password": "secret"
}
```

Me response:

```json
{
  "username": "janis",
  "display_name": "Janis",
  "role": "admin"
}
```

## 17.2 Tree

```http
GET  /api/v1/tree
POST /api/v1/folders
POST /api/v1/pages
```

Tree response:

```json
{
  "items": [
    {
      "type": "folder",
      "path": "runbooks",
      "title": "runbooks",
      "children": [
        {
          "type": "page",
          "path": "runbooks/deploy-staging.md",
          "title": "Deploying Staging"
        }
      ]
    }
  ]
}
```

## 17.3 Pages

```http
GET    /api/v1/pages/{path}
PUT    /api/v1/pages/{path}
DELETE /api/v1/pages/{path}
POST   /api/v1/pages/{path}/rename
GET    /api/v1/pages/{path}/history
POST   /api/v1/pages/{path}/restore
```

Page response:

```json
{
  "path": "runbooks/deploy-staging.md",
  "frontmatter": {
    "type": "Runbook",
    "title": "Deploying Staging",
    "description": "Steps for deploying staging.",
    "tags": ["devops"],
    "timestamp": "2026-06-17T15:00:00Z"
  },
  "body": "# Deploying Staging\n\n...",
  "revision": "git-or-content-hash"
}
```

Update page request:

```json
{
  "frontmatter": {
    "type": "Runbook",
    "title": "Deploying Staging",
    "description": "Steps for deploying staging.",
    "tags": ["devops"]
  },
  "body": "# Deploying Staging\n\nUpdated body.",
  "base_revision": "previous-content-hash"
}
```

## 17.4 Attachments

```http
POST   /api/v1/pages/{path}/attachments
GET    /api/v1/attachments/{id}
GET    /api/v1/attachments/{id}/download
GET    /api/v1/attachments/{id}/text
DELETE /api/v1/attachments/{id}
POST   /api/v1/attachments/{id}/replace
```

Upload:

```http
POST multipart/form-data
file=<binary>
```

Attachment response:

```json
{
  "id": "7f3a_contract",
  "original_name": "contract.pdf",
  "mime_type": "application/pdf",
  "size_bytes": 482194,
  "download_url": "/api/v1/attachments/7f3a_contract/download",
  "extraction_status": "queued"
}
```

## 17.5 Search

```http
GET /api/v1/search?q=deploy&type=all
```

Search response:

```json
{
  "results": [
    {
      "type": "page",
      "path": "runbooks/deploy-staging.md",
      "title": "Deploying Staging",
      "snippet": "Deploy staging after CI passes..."
    },
    {
      "type": "attachment",
      "id": "7f3a_contract",
      "title": "contract.pdf",
      "snippet": "Payment terms..."
    }
  ]
}
```

## 17.6 Agent

```http
POST /api/v1/agent/chat
POST /api/v1/agent/summarize-page
POST /api/v1/agent/summarize-attachment
POST /api/v1/agent/propose-patch
POST /api/v1/agent/apply-patch
```

Chat request:

```json
{
  "message": "What are the deployment steps?",
  "context": {
    "page_path": "runbooks/deploy-staging.md",
    "attachment_ids": []
  }
}
```

Patch proposal response:

```json
{
  "summary": "Updated deployment steps based on the attached checklist.",
  "diff": "--- old\n+++ new\n...",
  "requires_approval": true
}
```

Apply patch request:

```json
{
  "page_path": "runbooks/deploy-staging.md",
  "diff": "--- old\n+++ new\n...",
  "approval": true
}
```

---

# 18. Frontend Specification

## 18.1 Main routes

```text
/login
/app
/app/page/:path
/admin
```

## 18.2 Main components

```text
AppShell
LeftTree
PageView
PageEditor
AttachmentCard
AttachmentPanel
PromptBar
AgentChat
SearchDialog
HistoryDialog
DiffReviewDialog
LoginForm
AdminSettings
```

## 18.3 Page modes

Each page has modes:

```text
Read mode
Edit mode
Diff review mode
History mode
```

Read mode:

* Render Markdown.
* Show attachment cards.
* Show metadata.
* Allow search.
* Allow prompt.

Edit mode:

* Edit title.
* Edit tags.
* Edit description.
* Edit body.
* Upload attachments.
* Save.
* Cancel.

Diff review mode:

* Show agent proposed changes.
* Approve.
* Reject.
* Copy patch.

History mode:

* Show Git versions.
* View old version.
* Restore old version.

---

# 19. Attachment UX

When a user drags a file into a page:

```text
1. Frontend uploads file.
2. Backend stores original.
3. Backend creates metadata.
4. Backend inserts attachment link/card into page draft.
5. Backend queues extraction job.
6. UI shows extraction status.
```

Attachment card:

```text
┌──────────────────────────────────────────┐
│ 📄 contract.pdf                          │
│ 482 KB · uploaded by Janis · 2026-06-17  │
│ [Download] [Summarize] [Ask AI] [Remove] │
└──────────────────────────────────────────┘
```

For images:

```text
┌──────────────────────────────────────────┐
│ 🖼 network-diagram.png                   │
│ [Preview image]                          │
│ [Download] [Ask AI] [Remove]             │
└──────────────────────────────────────────┘
```

---

# 20. Deployment Specification

## 20.1 Single binary mode

Build frontend:

```bash
cd web
npm install
npm run build
```

Build backend:

```bash
go build -o okf-workspace ./cmd/okf-workspace
```

Run:

```bash
./okf-workspace serve --config ./config.yaml
```

## 20.2 Data directory

Default:

```text
/var/lib/okf-workspace
```

Example:

```text
/var/lib/okf-workspace/
  repo/
  app.db
  config.yaml
  logs/
  tmp/
```

## 20.3 Config file

Example:

```yaml
server:
  listen: "0.0.0.0:8080"
  public_url: "https://wiki.example.local"

storage:
  data_dir: "/var/lib/okf-workspace"
  repo_dir: "/var/lib/okf-workspace/repo"
  max_upload_mb: 100

git:
  enabled: true
  remote_enabled: true
  remote: "origin"
  branch: "main"
  push_on_commit: true
  pull_on_startup: true

auth:
  session_cookie_name: "okf_session"
  session_ttl_hours: 168

agent:
  enabled: true
  provider: "openai-compatible"
  model: "local-or-remote-model"
  base_url: "http://localhost:11434/v1"
  api_key_env: "OKF_LLM_API_KEY"

search:
  enabled: true
  engine: "sqlite_fts5"

attachments:
  extract_text: true
  allowed_extensions:
    - ".pdf"
    - ".docx"
    - ".txt"
    - ".md"
    - ".png"
    - ".jpg"
    - ".jpeg"
    - ".svg"
    - ".csv"
    - ".xlsx"
    - ".zip"
```

## 20.4 systemd service

```ini
[Unit]
Description=OKF Workspace
After=network.target

[Service]
User=okf
Group=okf
ExecStart=/usr/local/bin/okf-workspace serve --config /etc/okf-workspace/config.yaml
WorkingDirectory=/var/lib/okf-workspace
Restart=always
RestartSec=3
Environment=OKF_LLM_API_KEY=change-me

[Install]
WantedBy=multi-user.target
```

## 20.5 Docker mode

Dockerfile should produce one small image:

```text
Build frontend
Build Go binary
Copy binary into minimal runtime image
Run as non-root user
Mount data directory
```

Example compose:

```yaml
services:
  okf-workspace:
    image: okf-workspace:latest
    container_name: okf-workspace
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - ./data:/var/lib/okf-workspace
    environment:
      - OKF_LLM_API_KEY=${OKF_LLM_API_KEY}
```

---

# 21. Security Requirements

## 21.1 Path safety

The backend must prevent:

* `../` path traversal.
* absolute path access.
* symlink escape from repository.
* direct arbitrary file reads.
* direct arbitrary file writes.

All file paths must be resolved through a safe path resolver.

## 21.2 Upload safety

The backend must enforce:

* max upload size.
* allowed extensions.
* MIME detection.
* safe generated storage names.
* original filename escaping.
* no execution of uploaded files.
* no serving attachments with dangerous inline content by default.

Downloads should use:

```http
Content-Disposition: attachment
```

for risky formats.

## 21.3 Agent safety

Agent must not receive:

* application config secrets.
* session cookies.
* raw environment variables.
* unrestricted filesystem access.
* unrestricted shell access.

Agent must only interact through approved tools.

## 21.4 Auth

MVP auth:

* Password hashing with Argon2id or bcrypt.
* Secure session cookies.
* HTTPOnly cookies.
* SameSite=Lax or Strict.
* CSRF protection for mutating requests.
* Admin bootstrap on first startup.

## 21.5 Audit

Audit log should capture:

* login
* logout
* page create
* page edit
* page delete
* attachment upload
* attachment download
* attachment delete
* agent prompt
* agent patch approval
* config changes

---

# 22. Testing Requirements

## 22.1 Backend tests

Required tests:

* safe path resolver
* frontmatter parser
* OKF page read/write
* page creation
* page rename
* page delete/restore
* attachment upload
* attachment metadata generation
* attachment download
* text extraction job
* Git commit creation
* Git history read
* search index update
* agent tool permission checks

## 22.2 Frontend tests

Required tests:

* login flow
* tree rendering
* page load
* page edit/save
* upload attachment
* download attachment
* prompt submission
* patch review
* search dialog

## 22.3 Round-trip tests

The most important editor tests:

```text
Markdown input
  ↓
frontend editor model
  ↓
Markdown output
  ↓
compare with expected Markdown
```

Must test:

* headings
* lists
* nested lists
* code blocks
* tables
* links
* images
* attachment links
* frontmatter preservation

---

# 23. MVP Roadmap

## Phase 0 — Skeleton

Deliver:

* Go server.
* TypeScript frontend.
* Login page.
* Static asset serving.
* Data directory initialization.
* Git repository initialization.

## Phase 1 — OKF pages

Deliver:

* Left navigation tree.
* Page read.
* Page edit.
* Page create.
* Page delete to trash.
* Frontmatter parsing.
* Markdown rendering.
* Git commits.

## Phase 2 — Attachments

Deliver:

* Upload attachments.
* Download originals.
* Attachment cards.
* Attachment metadata.
* Text extraction for PDF/DOCX/TXT.
* Attachment search.

## Phase 3 — Search

Deliver:

* Page search.
* Tag search.
* Attachment search.
* Extracted text search.

## Phase 4 — Eino agent

Deliver:

* Bottom prompt.
* Read-only Q&A.
* Page summarization.
* Attachment summarization.
* Proposed page patches.
* Diff review and approval.

## Phase 5 — Collaboration improvements

Deliver:

* Soft locks.
* Presence indicator.
* Conflict detection.
* Manual merge UI.
* Optional realtime Yjs research/prototype.

---

# 24. First Implementation Milestone

The first useful prototype should do only this:

```text
1. Start one Go binary.
2. Open web UI.
3. Login as admin.
4. See left tree.
5. Open index.md.
6. Edit Markdown.
7. Save.
8. Backend commits to Git.
9. Upload PDF.
10. PDF appears as attachment card.
11. Download PDF.
12. Ask agent to summarize current page.
```

This proves the whole product direction.

---

# 25. Success Criteria

MVP is successful when:

* A non-technical user can create and edit pages without Git knowledge.
* Uploaded files remain downloadable exactly as uploaded.
* The agent can answer questions from pages and extracted attachments.
* All content can be copied from the server as plain files.
* Git history is readable and useful.
* The system can be deployed on a small server without complex infrastructure.
* The team prefers it over a shared folder plus random documents.

---

# 26. Default Decisions

Use these defaults unless changed later:

```text
Backend: Go
Agent framework: Eino
Frontend: TypeScript + Vite
Database: SQLite for app metadata only
Content storage: OKF Markdown files
Versioning: Git
Attachment storage: local filesystem inside repo
Search: SQLite FTS5 or Bleve
Deployment: single Go binary
Auth: local users/passwords
Collaboration MVP: soft locks + optimistic concurrency
Realtime collaboration: later, optional
Editor MVP: Markdown editor with preview
Rich editor: later, only if Markdown round-trip is safe
```

---

# 27. Product Definition

OKF Workspace is a lightweight, self-hosted, OKF-native internal wiki with hidden Git versioning, downloadable first-class attachments, Markdown-based knowledge storage, and an Eino-powered agent interface for search, summarization, and controlled document updates.

It is built for small teams that want Notion-like simplicity without vendor lock-in, high monthly cost, or a proprietary knowledge database.

