package users

import (
	"context"
	"fmt"
	"time"

	"github.com/postfix/okworkspace/internal/gitstore"
	"github.com/postfix/okworkspace/internal/repo"
)

// Seeder is the subset of *gitstore.GitStore the seed needs: an emptiness check
// and the single-writer Commit. Defined as an interface so the seed is testable
// and never reaches for a raw git call (D-10).
type Seeder interface {
	IsEmpty(ctx context.Context) (bool, error)
	Commit(ctx context.Context, spec gitstore.CommitSpec) error
}

// starterPage is one seeded index page (repo-relative path + OKF body).
type starterPage struct {
	rel  string
	body string
}

// SeedStarterRepo writes the SPEC §9 starter layout (root index.md plus
// runbooks/architecture/decisions index pages, each with valid SPEC §10 OKF
// frontmatter, and a .okf-workspace scaffold) and commits it as ONE commit
// through the single-writer Git service (D-08/D-10). It seeds ONLY when the repo
// is genuinely new and empty; a pulled/populated repo is left untouched
// (seeded=false, D-09). Files are written via the repo resolver (SEC-01), never
// a raw filesystem or git path.
func SeedStarterRepo(ctx context.Context, gs Seeder, r *repo.Repo, adminUser string) (bool, error) {
	empty, err := gs.IsEmpty(ctx)
	if err != nil {
		return false, fmt.Errorf("seed: check repo empty: %w", err)
	}
	if !empty {
		return false, nil
	}

	ts := time.Now().UTC().Format(time.RFC3339)
	pages := []starterPage{
		{
			rel: "index.md",
			body: page("Page", "Welcome to your workspace",
				"The home page for your team's knowledge base.",
				[]string{"home"}, ts,
				"# Welcome to your workspace\n\nThis is your team's knowledge base. Browse the starter sections in the navigation, or create your own pages once editing is available.\n"),
		},
		{
			rel: "runbooks/index.md",
			body: page("Page", "Runbooks",
				"Operational runbooks and step-by-step procedures.",
				[]string{"runbooks"}, ts,
				"# Runbooks\n\nStep-by-step operational procedures live here.\n"),
		},
		{
			rel: "architecture/index.md",
			body: page("Page", "Architecture",
				"System architecture notes and diagrams.",
				[]string{"architecture"}, ts,
				"# Architecture\n\nArchitecture notes, diagrams, and design context live here.\n"),
		},
		{
			rel: "decisions/index.md",
			body: page("Page", "Decisions",
				"Architecture decision records (ADRs).",
				[]string{"decisions"}, ts,
				"# Decisions\n\nArchitecture decision records (ADRs) live here.\n"),
		},
	}

	paths := make([]string, 0, len(pages)+1)
	for _, p := range pages {
		if err := r.Write(p.rel, []byte(p.body)); err != nil {
			return false, fmt.Errorf("seed: write %q: %w", p.rel, err)
		}
		paths = append(paths, p.rel)
	}

	// .okf-workspace application-metadata scaffold (SPEC §9). A manifest.json
	// gives the directory a tracked file so it is committed (Git does not track
	// empty directories).
	const manifestRel = ".okf-workspace/manifest.json"
	manifest := fmt.Sprintf("{\n  \"version\": 1,\n  \"seeded_at\": %q,\n  \"seeded_by\": %q\n}\n", ts, adminUser)
	if err := r.Write(manifestRel, []byte(manifest)); err != nil {
		return false, fmt.Errorf("seed: write %q: %w", manifestRel, err)
	}
	paths = append(paths, manifestRel)

	if adminUser == "" {
		adminUser = "admin"
	}
	if err := gs.Commit(ctx, gitstore.CommitSpec{
		Paths:   paths,
		Message: "Seed starter workspace",
		User:    adminUser,
		Action:  "seed",
		Source:  "bootstrap",
	}); err != nil {
		return false, fmt.Errorf("seed: commit starter layout: %w", err)
	}
	return true, nil
}

// page renders an OKF Markdown page: a YAML frontmatter block carrying all SPEC
// §10 required fields, followed by the body.
func page(typ, title, description string, tags []string, timestamp, body string) string {
	tagLines := ""
	for _, t := range tags {
		tagLines += fmt.Sprintf("  - %s\n", t)
	}
	if tagLines == "" {
		tagLines = "  []\n"
	}
	return fmt.Sprintf(`---
type: %s
title: %s
description: %s
tags:
%stimestamp: %s
---

%s`, typ, title, description, tagLines, timestamp, body)
}
