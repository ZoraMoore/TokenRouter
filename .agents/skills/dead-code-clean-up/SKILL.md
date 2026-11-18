---
name: dead-code-clean-up
description: |
  Identify dead code, redundant implementations, and orphaned artifacts, then remove approved items. Use only when the user explicitly requests this skill by name, such as "$dead-code-clean-up" or "use the dead code clean up skill". General cleanup requests, code reviews, refactors, and suspected unused code must not trigger this skill.
license: MIT
metadata:
  author: petekp
  version: "0.1.0"
---

# Dead code clean up

Identify dead code, redundant implementations, and orphaned artifacts, and remove them after the user approves the proposed cleanup.

## Invocation boundary

Run this workflow only when the user explicitly asks to use `dead-code-clean-up` (or calls it "dead code clean up"). Merely mentioning the skill, asking to edit or install it, or requesting general code cleanup does not authorize running it. Do not invoke it automatically during reviews, refactors, or routine development.

Follow the repository's `AGENTS.md` and `project-doc` requirements before analyzing code. Display the report in the conversation first; approval to invoke the skill is not approval to perform its proposed cleanup.

## Why Agent-Maintained Codebases Accumulate Cruft

Coding agents operate within narrow context windows. When refactoring, they often:

- Re-implement functionality without finding and removing the original
- Leave compatibility shims for interfaces that no longer exist
- Abandon helper functions after inlining their logic
- Create new files without deleting the ones they replace
- Duplicate type definitions across module boundaries
- Leave imports for symbols they stopped using mid-refactor

This cruft compounds. Each orphaned artifact misleads the next agent (or human), who wastes context budget reading code that does nothing.

## Workflow

### Phase 1: Scope and Inventory

Determine the analysis boundary.

**If the user provides a scope** (directory, file pattern, module), constrain all analysis to that scope.

**If no scope is provided**, analyze the full codebase. Start by reading the project structure:

1. Identify the primary language(s) and framework(s)
2. Map entry points (main files, route definitions, exported modules, test runners)
3. Note the build system and dependency configuration
4. Check for monorepo structure — analyze each package as a unit

Produce a brief inventory:

```
Language(s): TypeScript, Python
Entry points: src/index.ts, src/cli.ts
Build: esbuild via package.json scripts
Packages: 1 (single package)
Estimated LOC: ~4,200
```

### Phase 2: Detection

Launch parallel sub-agents to scan for different categories of dead code. Read `references/cruft-patterns.md` for the full catalog of detection patterns.

Organize detection into these parallel tracks:

| Track | What It Finds |
|-------|--------------|
| **Orphaned files** | Files not imported, required, or referenced by any other file |
| **Unused exports** | Exported symbols (functions, classes, types, constants) never imported elsewhere |
| **Redundant implementations** | Multiple functions/classes doing the same thing under different names |
| **Stale compatibility code** | Shims, adapters, wrappers, and re-exports that bridge interfaces that no longer differ |
| **Dead branches** | Conditional paths that can never execute (always-true/false guards, unreachable returns) |
| **Orphaned tests** | Test files testing functions or modules that no longer exist |
| **Orphaned dependencies** | Packages in dependency manifests not imported anywhere in source |

For each track, the sub-agent should:

1. Read `references/cruft-patterns.md` for detection strategies specific to that track
2. Search the codebase using Grep and Glob
3. Verify each candidate by tracing references — a symbol is only dead if **zero live code paths** reach it
4. Record findings with file path, line range, and evidence

**Verification is critical.** Common false positives to watch for:

- Symbols used via dynamic dispatch, reflection, or string-based lookups
- Framework-magic exports (e.g., Next.js page components, pytest fixtures, Rails conventions)
- Public API surface intended for external consumers
- Conditional imports behind feature flags or environment checks
- Decorator-registered or plugin-registered handlers
- CLI entry points referenced in package.json `bin` fields
- CSS class names referenced in templates or JSX as dynamic strings

When uncertain, mark as "needs review" rather than "confirmed dead."

### Phase 3: Report

Consolidate all findings into a structured report and output the full report directly in the conversation. Do not save it to a file during this phase. Save it only after the user approves cleanup, as described in Phase 4.

Organize findings by confidence level, then by category:

```markdown
# Dead code clean up report

**Scope:** [full codebase | specific path]
**Date:** [date]
**Estimated removable lines:** [count]

## Confirmed Dead (high confidence)

### Orphaned Files
- `src/utils/old-parser.ts` — Not imported anywhere. Superseded by `src/parser/index.ts`.
- ...

### Unused Exports
- `formatDate()` in `src/helpers.ts:42-58` — Exported but zero imports across codebase.
- ...

[...other categories...]

## Needs Review (uncertain)

### Possibly Dynamic
- `handleLegacyEvent()` in `src/events.ts:91` — No static imports, but may be registered dynamically.
- ...
```

For each finding, include:

- **File path and line range**
- **What it is** (function, class, file, type, constant, dependency)
- **Why it appears dead** (no imports, no references, superseded by X)
- **Confidence** (confirmed / needs review)

### Phase 4: Cleanup with Approval

After displaying the full report, ask the user which findings or categories they approve for cleanup. Wait for that approval before saving the report or removing anything. If the user declines cleanup, only wants analysis, or has not replied, leave the report in the conversation without creating a report file.

Use an available user-input tool or a conversation reply to ask for approval, for example:

```
Found 12 confirmed dead items and 3 needing review.

Confirmed dead by category:
- 3 orphaned files (~280 lines)
- 5 unused exports (~120 lines)
- 2 redundant implementations (~90 lines)
- 2 orphaned dependencies

Which categories should I clean up?
```

Options: Remove all confirmed / Select categories / Review each item / Skip cleanup

Once cleanup is approved, save the full report to `.agents/plans/dead-code-clean-up-YYYYMMDD-HHMMSS.md` at the repository root before making changes. Create `.agents/plans/` if needed and choose an unused filename. Record the approved findings or categories in the saved report; unapproved items remain outside the cleanup scope. Do not save reports under `.claude/` or commit these local plans to Git.

For each approved category:

1. Remove the dead code
2. Clean up any imports that referenced the removed code
3. Remove empty files left after cleanup
4. Remove orphaned dependencies from the manifest
5. Run the project's lint/typecheck/build commands to verify nothing broke

If any removal causes a build or type error, immediately revert that specific removal and move the item to "needs review."

After cleanup, append the removed and retained items and validation results to the saved report, preserving the original findings and approval scope. Summarize the result and link to the saved report in the conversation.

## Detection Principles

### Trace from entry points, not from suspects

Start from known entry points and trace what's reachable, rather than starting from a suspect symbol and trying to prove it's used. The reachability approach has fewer false negatives.

### Respect the module boundary

A symbol exported from a package's public API may be consumed by external code not visible in this repository. When analyzing libraries or packages with external consumers, only flag internal (non-public-API) dead code as "confirmed." Flag public API dead code as "needs review."

### Look for clusters, not just individuals

Agent-generated cruft tends to cluster. When one dead function is found, examine its neighbors — the agent likely abandoned the entire section during a refactor. A dead file often has sibling dead files created in the same commit.

### Use git history as a signal

When available, check when suspect code was last meaningfully modified. Code untouched across several refactor commits is more likely dead. Use `git log --follow` to trace renames and detect superseded files.
