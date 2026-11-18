# Production Deployment And Rollback

Read this reference only when the user explicitly authorizes production upgrade/deployment.

## Pre-Cutover

Confirm:

- source tests and image probe passed;
- the current application, PostgreSQL, and Redis are healthy;
- enough disk space exists for at least two fresh dumps and both images;
- the old image tag and ID are recorded and still present;
- the final target image resolves through the actual Compose `.env`;
- migration compatibility and rollback requirements have been reviewed.

Record non-sensitive baseline counts for stable business tables and the current maximum migration. Check health and the home page before downtime.

## Final Consistent Backup

For the safest cutover, stop only the application container; leave PostgreSQL and Redis running. Then create a timestamped backup directory outside the repository containing:

- PostgreSQL custom-format dump;
- Redis snapshot;
- production `.env` with mode `0600`;
- Compose file;
- application `config.yaml`, installation lock, and model-pricing files when present;
- baseline counts and old image metadata.

Validate the dump with `pg_restore -l`, then create and verify SHA-256 checksums for every backup file.

Redis authentication must reflect the deployment. If Redis has no password but the container carries an empty `REDISCLI_AUTH`, run the save command with that variable unset:

```bash
docker exec <redis-container> sh -c 'env -u REDISCLI_AUTH redis-cli save'
```

Do this only after confirming Redis is actually passwordless. With password authentication enabled, preserve the configured authentication instead.

## Switch The Image

Follow `docs/operations/upgrade_notes.md` in the repository for release-specific Compose and proxy changes. For an image-only upgrade, update the image value in the production environment file. Preserve permissions and compare hashes with the image line excluded. Never print the whole file.

Validate Compose resolution before mutation, then recreate only the application:

```bash
docker compose --env-file <deploy-root>/.env -f <compose-file> config --images
docker compose --env-file <deploy-root>/.env -f <compose-file> \
  up -d --no-deps --force-recreate <app-service>
```

Wait for a terminal health state while watching startup/migration logs. Do not treat `health: starting` as success.

## Post-Cutover Verification

Verify all of the following:

- binary version/commit and actual container image;
- health and home page return 200;
- new authenticated routes exist (an unauthenticated 401 is useful evidence that routing and auth are active);
- every expected migration is present with the intended filename;
- expected tables, columns, constraints/indexes, and functions exist;
- stable business-table counts match the pre-cutover baseline;
- container restart count is zero and it was not OOM-killed;
- logs contain no migration checksum error, ERROR, PANIC, or FATAL after background workers have run several cycles;
- invalid gateway credentials are rejected;
- feature flags such as `creative_enabled` have the intended effective value.

Do not perform a real payment or paid model request without explicit authorization.

## Rollback

If the new container fails before applying migrations, restore the old image value and recreate the application.

If migrations were applied, first determine whether the old binary is explicitly compatible with the new schema. Do not assume compatibility merely because migrations looked additive. When compatibility is uncertain or the documented rollback requires it:

1. stop the new application;
2. restore PostgreSQL from the validated final dump;
3. restore Redis and application configuration when their state is part of the failure;
4. restore the old image value and recreate the old application;
5. repeat health, log, and baseline checks.

Never roll back by deleting `schema_migrations` rows or editing historical migration files. Keep the backup and old image until the user accepts the upgraded service.
