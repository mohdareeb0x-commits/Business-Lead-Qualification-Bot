# CI / CD

`.github/workflows/ci.yml` runs on every push and pull request.

## Jobs

1. **test** — `gofmt -l`, `go vet ./...`, `go test -race ./...` with coverage.
2. **build** — only on `push`. Multi-arch (`linux/amd64,linux/arm64`) Docker
   build with Buildx + GHA cache. Pushes to Docker Hub when
   `DOCKERHUB_REPO` secret is set; otherwise it runs a local dry-build
   so PRs still validate the Dockerfile.
3. **deploy** — only on `push` to `main`. POSTs to the Render deploy
   hook URL so Render pulls the freshly-pushed image and rolls it out.

## Required secrets

| Secret | Used by | Notes |
| --- | --- | --- |
| `DOCKERHUB_REPO` | build | e.g. `yourname/telegram-lead-bot`. If empty, the build still runs but only locally (no push). |
| `DOCKERHUB_USERNAME` | build | Docker Hub login. |
| `DOCKERHUB_TOKEN` | build | Docker Hub access token (not your password). |
| `RENDER_DEPLOY_HOOK_URL` | deploy | From Render → service → Settings → Deploy Hook. If empty, the deploy step is skipped. |

## Tagging

- Push to `main` → `latest` + short SHA.
- Git tag `v1.2.3` → `1.2.3`, `1.2`, `latest`.
- PRs → tests only, no image is built.

## Render side

The Render service must be configured to:

1. Watch the same Docker Hub repo (`DOCKERHUB_REPO`).
2. Use the `latest` tag (or whichever you want CI to roll out).
3. Have a deploy hook URL set in secrets. The CI only POSTs the hook;
   the actual image pull + rollout is done by Render.
