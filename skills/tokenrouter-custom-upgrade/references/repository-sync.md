# Repository Synchronization

Use this reference when the user requests a security patch, upstream source sync, or a version upgrade. Compare and preserve the current refactored fork. This does not authorize production deployment or site backups.

## Establish The Baseline

Inspect, without exposing secrets:

- repository and deployment roots;
- `AGENTS.md`, the Project Doc root, and upstream-sync documentation;
- current branch, worktree status, remotes, tags, and recent commits;
- current source `VERSION`, deployed image tag/ID, and highest applied migration;
- diff between the fork and its upstream base;
- untracked source such as `website-docs/` that would otherwise be omitted from GitHub.

Classify differences as runtime behavior, tests, build/deployment, documentation, generated files, or production-only configuration. Production data and `.env` are not source customizations.

## Preserve Existing Custom Work

Before a merge:

1. Record the starting commit. Use an isolated source checkout if needed to preserve existing changes. Keep production images and data out of source sync.
2. Run the repository's secret scan. If its scanner is unavailable, report that drift and use a conservative filename/high-confidence fallback; never interpret a broken scanner as success.
3. Run focused tests for the affected security behavior and current customizations. Follow repository branch and worktree rules. Keep unrelated local changes intact.

If Git author identity is absent, use a repository-local neutral identity rather than changing global Git configuration or impersonating the user. Tell the user so they can replace it before future GitHub commits.

## Fetch And Review The Target

Fetch the selected security fix or exact release tag. Verify the prerequisite commits and review the smallest useful patch before considering a complete merge:

- the tag resolves to the expected repository and commit;
- the current upstream base is an ancestor of the target;
- commit and file counts match the reviewed comparison;
- release notes and the code diff agree on major behavior changes;
- changed custom files and new migrations are identified before merging.

Use `upstream` for the official TokenFlux remote and `origin` for the user's private repository when that remote model has been configured. Do not change remotes or push unless the user authorized those external mutations.

## Migration Numbering

Before merging, record the fork's highest migration number and the ordered upstream migration list. Allocate new fork IDs sequentially from that maximum. Rename migration files and update tests, runner exceptions, documentation, and every exact filename reference.

When the upstream prefixes already equal the fork's required next sequence, record that the allocation was checked and happens to match. Do not renumber historical fork migrations for neatness. Never modify the contents or filename of a migration already present in `schema_migrations`.

## Merge And Reconcile

Apply the security patch or merge the exact release the user requested. Resolve conflicts by preserving the fork’s current product, payment, security, deployment, and documentation semantics. The checks below apply only if these customizations still exist in the refactored branch:

- EasyPay `QueryOrder` still calls the GET helper and its tests read `r.URL.Query()`;
- URL-bearing transport errors cannot expose `pkey`;
- Docker `NODE_OPTIONS` remains at the intentional custom value;
- the documentation site and its Project Doc description remain present;
- upstream README additions are routed into the repository's maintained docs according to local instructions;
- generated code and lockfiles match the merged source.

Use the bundled checker as a reference for compatible historical layouts. It encodes older customization markers; assess which checks still apply before treating a failed check as a regression. For a compatible layout run:

```bash
skills/tokenrouter-custom-upgrade/scripts/check_customizations.sh "$(git rev-parse --show-toplevel)"
```

Official release tags can precede the workflow's `VERSION` file writeback. Ensure the custom branch contains the target version and still pass `--build-arg VERSION=<target>` during image construction.

Finish source sync with `git diff --check` and a concise comparison against the target commit or tag. Report unrelated worktree changes left in place. Do not push until the user supplies/authorizes the private GitHub destination.
