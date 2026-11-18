---
name: tokenrouter-custom-upgrade
description: Safely sync a customized TokenFlux TokenRouter/Sub2API fork to a selected upstream release, preserve local behavior, validate migrations and tests, build an immutable custom Docker image, and optionally upgrade a production Compose deployment with verified backups and rollback points. Use for inspecting, updating, testing, or deploying this custom TokenRouter fork; do not use for routine account administration or an unmodified upstream installation.
---

# TokenRouter Custom Upgrade

Keep the custom fork reproducible while advancing it to an explicit upstream release. Treat source synchronization, image construction, and production deployment as separate authorization scopes.

## Start

1. Discover the repository root, deployment root, active branch, remotes, running image, and current application/database versions. Paths such as `/opt/sub2api` are hints from the current deployment, not universal assumptions.
2. Follow repository instructions before reading or changing business files. If the repository requires `project-doc`, load it and route through `docs/index.md` and the relevant operations documents.
3. Determine the authorized mode from the user's request:
   - **Audit:** inspect differences and risk; do not modify source or production.
   - **Sync:** merge a selected upstream release and preserve custom source; do not deploy.
   - **Build:** sync if requested, run validation, and build a custom image; do not deploy.
   - **Deploy:** perform the full backup, cutover, migration, verification, and rollback-ready workflow. Production mutation requires an explicit upgrade/deploy request.
4. Use an exact upstream tag and an exact custom image tag. Never substitute `latest` for a requested release.

## Route To Details

- For any source synchronization, read [references/repository-sync.md](references/repository-sync.md).
- Before accepting source or an image, read [references/validation.md](references/validation.md).
- Before touching production containers, data, `.env`, or Compose, read [references/deployment.md](references/deployment.md).
- After a merge, run `scripts/check_customizations.sh <repo-root>` to catch silent loss of the known custom behavior.

## Current Customization Contract

Preserve these behaviors unless the user explicitly changes them:

- EasyPay active order queries use `GET /api.php`; payment creation and refunds remain POST. New order IDs use the upstream `tr_` prefix and notifications use upstream field validation.
- EasyPay transport errors must not expose the query URL because it contains the merchant key.
- EasyPay provider and service-level draft tests assert GET query parameters and credential redaction.
- The frontend Docker build uses a 4096 MiB Node heap; this affects build time only, not runtime memory.
- `website-docs/` remains a separate versioned VitePress site and is excluded from the main Docker build context.
- Controlled TLS profiles retain provenance, local replay and target-handshake evidence; only approved, verified, target-compatible profiles may be bound or enter the random pool.
- Authenticated ingress capture retains its observer, authenticated request association, bounded queues/TTLs and shared Redis daily quotas. Automatically created profiles remain unapproved until an administrator publishes them.
- Main application and TLS observer use the same immutable custom image. Preserve the observer binary, Compose profile, trusted Nginx headers and public `/internal/` block.
- The already-applied `259_add_tls_fingerprint_profile_verification.sql` is immutable. For v0.1.277, upstream migrations 259–265 use fork IDs 260–266. For v0.2.10, upstream migrations 266–296 use fork IDs 267–297. Migration 273 archives existing group prices; migration 282 transfers group price cards and billing settings into separate pricing configurations. Custom migration 298 preserves independent image rates across ordinary requests, public quotes, creative tasks, and batch funding snapshots.
- Vite production builds avoid the duplicate checker process while retaining the preceding `vue-tsc` typecheck.
- Migration 278 uses PostgreSQL's constant default for OpenAI history, restores the future default to `unknown`, and updates differing platform values. Rehearsals include every historical table and compare all usage fields, platform totals, balances, prices, and TLS settings before production cutover.

This list is a guardrail, not the complete customization inventory. Derive the full inventory from Git history and the diff against the upstream base. If the customization contract intentionally evolves, update this skill and its checker in the same change.

## Non-Negotiable Invariants

- Do not use the in-app updater for this fork. It downloads the official binary and can overwrite custom code while leaving Compose pointed at an older custom image.
- Never push or print production `.env`, database dumps, Redis data, credentials, API keys, or account exports.
- Preserve a dirty worktree before merging. Create recoverable source artifacts and commit custom changes by purpose; do not discard or overwrite unknown changes.
- Review every upstream migration. Allocate migration IDs from the fork's actual maximum, update exact filename references, and never alter an already-applied migration.
- A binary/image rollback does not undo database migrations. Never delete `schema_migrations` rows as a rollback technique.
- Build with explicit `VERSION`, source commit, and build date. Verify the version from the finished image before production cutover.
- Keep the previous image and a validated final backup until post-deployment checks pass.
- Do not send a real payment, consume a user's API key, or call a paid upstream merely for smoke testing unless explicitly authorized.

## Completion

Report the upstream tag, custom source commit, image tag and ID, preserved customizations, tests run, backup location, applied migrations, health/log checks, data-count comparison, and any intentionally untested external workflow. Keep the worktree clean and identify anything not pushed to the user's GitHub repository.
