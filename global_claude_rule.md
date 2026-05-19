# Repository Global Instructions

These instructions apply to ALL tasks in this repository.

They define mandatory guardrails that must be followed before any code, SQL, schema, or configuration changes are made.

---

# 1. Mandatory Pre-Execution Workflow

Before performing ANY of the following:
- writing code
- editing files
- refactoring
- generating SQL
- modifying schema
- changing collectors
- altering dashboards

Claude MUST complete this mental workflow.

## Step 1 — Repository Awareness

Always review the existing repository structure and patterns before making changes.

This includes:
- folder structure
- naming conventions
- existing SQL patterns
- existing collectors
- existing schema design
- existing dashboard queries

Never assume structure. Always align with what already exists.

---

## Step 2 — Apply Repository Skills

Before proposing changes, Claude must internally apply knowledge from:

- project-architecture skill
- expert-dba-engineering skill

These skills define the source of truth for:
- architecture
- performance standards
- monitoring philosophy
- SQL design patterns
- collector safety rules

No change may violate those skills.

---

## Step 3 — Breaking Change Prevention

Before modifying anything, Claude must perform a mental dry-run and verify:

- No existing APIs are broken
- No schema changes break ingestion
- No dashboard queries will fail
- No collectors will stop working
- No naming conventions are violated

Backward compatibility is the default expectation.

Breaking changes must be explicitly justified.

---

# 2. Change Philosophy

Changes must be:

- incremental
- safe
- consistent with existing patterns
- production-ready
- maintainable long term

Avoid large rewrites unless explicitly requested.

Prefer refactoring over replacement.

---

# 3. Monitoring Safety Rule

This repository monitors production databases.

Therefore ALL generated work must prioritize:

1. Low overhead
2. Predictable execution
3. Safe queries
4. Scalable design

If a change risks production performance, it must be redesigned.

---

# 4. Consistency Over Creativity

When multiple design choices exist:

Prefer:
- consistency with existing code
- established project patterns
- simplicity
- maintainability

Do not introduce new patterns unless necessary.

---

# 5. Output Expectations

When proposing changes, Claude should:

- Explain reasoning briefly
- Highlight risks if any
- Prefer minimal, targeted edits
- Avoid unnecessary rewrites