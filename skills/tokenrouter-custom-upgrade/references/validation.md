# Validation Matrix

Use this reference before accepting a synced source tree or custom image. Expand the matrix when upstream changes touch additional domains.

## Customization Gate

Run `git diff --check` and review the diff against the selected upstream commit or tag. Validate the specific fix and the fork’s current behavior with relevant security regression and compatibility tests. The older customization checker is applicable only to the historical layouts described in [repository-sync.md](repository-sync.md).

## Backend

Use the repository-declared Go version. If Go is absent on the host, run tests in the matching official Go container with the repository mounted read-only and persistent module/build cache volumes.

Match the production Docker build with `CGO_ENABLED=0` for tests, lint and vulnerability scans. Tool images can otherwise default to CGO and rebuild a different dependency graph. On deployment hosts with 8 GiB RAM or less, run backend test compilation, lint, vulnerability scans, frontend typechecking and image builds serially. Individual container limits do not prevent host OOM when their combined peaks exceed available memory; reserve capacity for the live application and databases. Use bounded Go parallelism and a single Vitest worker on these hosts.

Minimum focused coverage:

- EasyPay provider tests;
- payment service draft/provider tests;
- migration runner and every new migration regression test;
- new feature service/repository tests;
- Ent schema tests;
- full unit suite when production deployment is intended.

Some Ent schema tests create a temporary `.entc` directory. A read-only source mount will fail with `mkdir .entc: read-only file system`; rerun that package in a writable temporary copy or a controlled writable mount, then confirm no generated residue remains.

Large service suites can emit extensive simulated upstream-error logs. Use `go test -json` and filter fail/pass actions when output volume would hide the actual failing test. Rerun each failure directly with `-run '^TestName$' -count=3 -v` before classifying it as flaky or environmental.

## Frontend And Documentation Site

Use the lockfile-compatible pnpm version:

```bash
npx --yes pnpm@9 --dir frontend install --frozen-lockfile
make test-frontend
npx --yes pnpm@9 --dir frontend run build
```

Also run tests added or materially changed by the target release, especially Creative Studio, model marketplace, settings, subscriptions, and the EasyPay UI flow. Build `website-docs/` separately when it is part of the fork and audit production dependencies separately from build-only development dependencies.

Warnings about Vite chunk size or a dependency's use of `eval` do not by themselves fail a build; record them and require a zero exit status. A native `canvas` install script can be non-blocking in an Alpine frontend-builder when browser-only code is built, but accept the image only if the actual production frontend step finishes successfully.

## Image Build And Probe

Use an immutable custom tag that includes the upstream version and customization identity. Pass explicit metadata:

```bash
docker build \
  --file deploy/Dockerfile \
  --build-arg VERSION="$target_version" \
  --build-arg COMMIT="$source_commit" \
  --build-arg DATE="$build_date" \
  --tag "$custom_image" \
  .
```

Do not accept the image until this succeeds:

```bash
docker run --rm --entrypoint /app/sub2api "$custom_image" --version
```

The reported version, commit, and date must match the intended source. If the old image still exists locally, leave it available through production verification. Do not export or archive it unless the user explicitly requests an image backup.

## External Side Effects

Default smoke tests may use health/public pages, unauthenticated Creative API checks, and a deliberately invalid gateway key. Do not read a real user key from the database or trigger live EasyPay/upstream requests without explicit authorization. Report those flows as not exercised when applicable.
