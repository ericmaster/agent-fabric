# Agent Fabric Documentation Blueprint

This document defines how documentation is organized in this repository: the divide between
versioned and unversioned material, the directory layout, the filing rules that keep folders
from overlapping, and the frontmatter every document carries.

---

## 1. The Clean Divide: Versioned Wiki vs. Scratch Space

Two environments, deliberately separated:

| | **Versioned wiki** (`docs/`) | **Scratch space** (unversioned) |
|---|---|---|
| **Contains** | Invariants, contracts, decisions, runbooks | Raw notes, meeting transcripts, clippings, work-in-progress drafts, full research dumps |
| **Trust** | High — reviewed, current, safe to act on | Low — unreviewed, possibly stale or contradictory |
| **Lifecycle** | Git-tracked, reviewed, diffable | Freely edited, never reviewed, disposable |
| **Audience** | Humans + agents, as source of truth | Humans thinking out loud; agents reading for *input* only |

**Rules:**

1. **The repository is the SSOT.** `docs/` is the single source of truth for architecture,
   decisions, contracts, and procedures. Nothing outside it is authoritative.
2. **Scratch is input, never output.** An agent may *read* the scratch space to synthesize a
   document, but the synthesized result is committed to `docs/` — the scratch original is not
   the deliverable and is never linked as the canonical reference.
3. **No shared write surface.** Do not mount or sync the scratch space into the repository
   working tree. Concurrent human edits and agent writes in the same directory produce
   collisions and lock contention. Read it over its own interface (API, read-only path, or a
   copy), synthesize, then commit.
4. **Cite, don't inline.** When a `docs/` file distills something large, it states the takeaway
   and links back to the full source rather than pasting it in.

Where the scratch space physically lives is deliberately unspecified — a wiki, a notes app, a
shared drive, or a `scratch/` directory listed in `.gitignore` all satisfy the contract.
Record the choice once, here:

> **This project's scratch space:** <!-- TODO: record location (e.g. a gitignored `scratch/` dir, a wiki, or a shared notes app) -->

---

## 2. Organization System

A **hybrid folder/tag structure**:

* **Folders for namespacing** — physical directories isolate document *kinds*. This scopes
  search paths and prevents naming collisions.
* **Tags for typing** — YAML frontmatter classifies the document type (adr, spec, runbook) and
  cross-cutting concerns, so views can be compiled across folders without reorganizing them.

---

## 3. Directory Layout

```
agent-fabric/
├── AGENTS.md                        # Core agent operational context (SSOT)
├── CLAUDE.md                        # Thin pointer to AGENTS.md
├── CONTEXT.md                       # Domain glossary
├── README.md                        # Human-facing overview
└── docs/
    ├── index.md                     # Wiki landing page / document map
    ├── docs-organization-blueprint.md  # This file
    ├── adr/                         # Architectural Decision Records (technical choices)
    ├── specs/                       # Module contracts and feature specifications
    ├── architecture/                # System designs, topologies, C4 structural diagrams
    ├── processes/                   # Standing policies, governance, boundaries
    ├── workflows/                   # Lifecycle flows spanning multiple systems
    ├── runbooks/                    # Step-by-step command recipes
    └── research/                    # Takeaways and reports (full sources in scratch)
```

Create a folder when it earns its first real file. `adr/` and `specs/` always exist — every
project accrues both. Do not scaffold seven empty directories.

### Bounded Categorization Rules (Filing Best Practices)

To prevent duplication and context drift between operational guides, maintain strict
distinctions. **When two folders could plausibly hold a document, this table decides.**

| Location | Target | Audience / Use | Primary Question Answered |
| :--- | :--- | :--- | :--- |
| **`agents/`** *(root)* | Portable canonical agent definitions | **Agent-harness only.** Evaluated by adapters to generate harness-native agent files. | *What portable workflows and contracts define each agent role?* |
| **`adapters/`** *(root)* | Target harness profile & model mappings | **Build / sync tooling.** Harness-specific mappings, permissions, and tool IDs. | *How do canonical definitions project into specific harness formats?* |
| **`docs/adr/`** | Decision Records | **Human + Agent.** Choices that are hard to reverse, surprising without context, and the result of a real trade-off. | *Why is it built this way?* |
| **`docs/specs/`** | Module & Feature Contracts | **Human + Agent.** Behavioral contract per module — what it must do, and why. | *What must this module do?* |
| **`docs/architecture/`** | Structural Designs | **Human + Agent.** Topologies, component maps, C4 diagrams. | *What is wired to what?* |
| **`docs/processes/`** | Standing Policies & Guidelines | **Human + Agent.** Rules, governance, and write discipline. | *What are the rules and boundaries of this system?* |
| **`docs/workflows/`** | Lifecycle Pathways | **Human + Agent.** Coordination sequences and pipeline lifecycles across systems. | *How does work flow from start to completion?* |
| **`docs/runbooks/`** | Technical Action Recipes | **Human (or authorized agent).** Exact CLI checklists for troubleshooting or deployment. | *What is the exact sequence of commands?* |
| **`docs/research/`** | Research Takeaways | **Human + Agent.** Summarized findings and recommendations. Full documents stay in the scratch space. | *What did we learn, and what do we recommend?* |
| **`docs/templates/`** | Document Templates | **Human + Agent.** Reusable markdown skeletons. | *What layout should this document follow?* |

Two tie-breakers worth stating explicitly:

* **ADR vs. spec** — an ADR records a *choice between alternatives* and freezes at the moment of
  decision; a spec records the *current required behavior* and is updated whenever behavior
  changes. If it would need editing after a code change, it is a spec.
* **Workflow vs. runbook** — a workflow explains the *path and its transitions*; a runbook gives
  the *commands*. If a reader could copy-paste it, it is a runbook.

---

## 4. Metadata Schema & Frontmatter Conventions

Every document carries standard YAML frontmatter so parsers and query tooling can filter it.

### A. Project / Concept Document

```yaml
---
title: "Multi-Harness Adapter Layer"
type: concept | entity | index
status: active | review-needed | stale
confidence: strong | moderate | weak
sources:
  - "<link to the scratch-space original this was synthesized from>"
tags:
  - agents
  - adapters
last_checked: 2026-08-18
---
```

### B. Decision Record (ADR)

```yaml
---
title: "ADR-0001: <Decision>"
type: adr
status: proposed | accepted | superseded
decided_by: <name>
date: 2026-08-18
supersedes: "docs/adr/<nnnn>-<slug>.md"   # omit if none
---
```

### C. Spec

```yaml
---
title: "Spec: <module>"
type: spec
status: active | superseded
covers: <source path this spec governs>
last_checked: 2026-08-18
---
```

`status` and `last_checked` are what make staleness detectable — a document that no longer
matches the code should be marked `stale` rather than silently left to mislead.

---

## 5. Compiled Views

The frontmatter above exists so index pages can be compiled rather than hand-maintained. Any
tool that reads YAML frontmatter (a static-site generator, a notes app with a query language, or
a small script) can produce views such as:

* All documents with `status: stale`, oldest `last_checked` first — the review queue.
* All `type: adr` sorted by `date` — the decision log.
* Everything tagged with a given topic, across folders.

Record which tool compiles these views for this project, if any:

> **View compiler:** none — indexes are hand-maintained
