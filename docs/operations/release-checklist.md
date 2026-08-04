# Release Checklist

$OOSHDAQ is distributed as source. A release is a reviewed repository revision,
not a project-published binary or container image.

## Product surface

- Normal startup includes only deliberately published catalogs. Fictional
  catalogs require the explicit demo option and a disposable database.
- Owned, available, and wanted quantities remain numeric in storage and the API.
- Member-facing summaries show available, wanted, and possible-swap counts only.
- The browser UI is checked at 900 CSS pixels and a typical wide desktop size.
  Mobile and narrower layouts are unsupported.
- Member-facing copy uses ordinary collection and swap language.

## Local checks

Run from a clean checkout with the Go version in `go.mod` and browser packages
installed by `npm ci`:

```sh
go mod verify
make check
make test-race
make test-web
(cd web && npm ci)
make test-browser
docker compose config --quiet
make container-smoke
make qualify
```

The container smoke test must use a working local Docker daemon. It verifies
the non-root read-only runtime, persistent SQLite volume, online backup,
isolated restore, graceful shutdown, immutable local image ID, and repository
source label. No registry digest is required for a locally built image.

## Repository review

- Verify the Go module, clone URL, and OCI source label name the same repository.
- Check documentation links and ensure commands still match the Makefile.
- Scan tracked files and the commit being published for credentials, tokens,
  personal email addresses, generated databases, local paths, and private data.
- Inspect screenshots for real identities, browser chrome, tokens, local paths,
  and third-party artwork without redistribution rights.
- Confirm application-created SQLite files, sidecars, backups, and restore files
  are owner-only on Unix systems.
- Review catalog manifests and artwork provenance before publishing catalog data.
- Confirm the intended contributor attribution and a clean worktree.

GitHub Actions, project-owned artifact publication, signing, and deployment
automation are intentionally outside this checklist.
