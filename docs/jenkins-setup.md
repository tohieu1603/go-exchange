# Jenkins + Docker Hub setup

Pipelines: [`Jenkinsfile`](../Jenkinsfile) (this repo, backend) and `Jenkinsfile` in the frontend repo (`tohieu1603/go_exchange_fe`).

## What the pipeline does

**Backend** (every push to any branch):
1. `go vet` + `go build` per service in parallel (fail-fast).
2. `go test ./...` per service.
3. **On `main` only** — build + push 1 Docker image per service to Docker Hub:
   - `auth-service`, `wallet-service`, `market-service`, `trading-service`, `futures-service`, `notification-service`, `gateway`, `es-indexer`
   - Tags: `<git-sha>` (immutable) + `latest`.

**Frontend** (every push):
1. `npm ci` → `npm run lint` + `tsc --noEmit` (parallel) → `npm run build`.
2. **On `main`** — build + push `micro-exchange-frontend:<git-sha>` + `:latest`.

## Jenkins prerequisites

- Jenkins 2.400+ with **Docker Pipeline** + **Pipeline: SCM** + **Credentials Binding** plugins.
- Agent (node) with: Docker 24+, Go 1.25, Node 20. Or run pipelines on a Docker-in-Docker agent.

## One-time setup

### 1. Add Docker Hub credentials

`Manage Jenkins` → `Credentials` → `(global)` → `Add Credentials`:

- **Kind**: Username with password
- **Username**: your Docker Hub username (e.g. `tohieu1603`)
- **Password**: a Docker Hub **Personal Access Token** (Account Settings → Security → New Access Token, scope `Read & Write`). Avoid using your real password.
- **ID**: `dockerhub-credentials` ← exact, both Jenkinsfiles reference this.

### 2. Create the pipeline jobs

For **each repo** (backend + frontend):

`New Item` → name (`micro-exchange-backend`, `micro-exchange-frontend`) → **Pipeline** → OK.

Configure:
- **Pipeline → Definition**: *Pipeline script from SCM*
- **SCM**: Git → Repository URL → Branch `*/main` (or `*/*` for multibranch).
- **Script Path**: `Jenkinsfile`.
- (Optional) **Build Triggers** → *GitHub hook trigger for GITScm polling* and add a webhook in GitHub → `https://<jenkins>/github-webhook/`.

For multibranch (PR builds + main push), use **New Item → Multibranch Pipeline** instead — same Jenkinsfile, automatically picks up branches and PRs.

### 3. (Optional) Override defaults

Both Jenkinsfiles expose `DOCKERHUB_USER` as a parameter (default `tohieu1603`). Run the job with *Build with Parameters* to push under a different user/org.

## What ends up on Docker Hub

After a green build on `main`, you get 9 images:

```
tohieu1603/micro-exchange-auth-service:latest
tohieu1603/micro-exchange-wallet-service:latest
tohieu1603/micro-exchange-market-service:latest
tohieu1603/micro-exchange-trading-service:latest
tohieu1603/micro-exchange-futures-service:latest
tohieu1603/micro-exchange-notification-service:latest
tohieu1603/micro-exchange-gateway:latest
tohieu1603/micro-exchange-es-indexer:latest
tohieu1603/micro-exchange-frontend:latest
```

…each also tagged with the 7-char git SHA for rollback.

## Pulling + running

```bash
docker pull tohieu1603/micro-exchange-auth-service:latest
docker run --rm -e DB_HOST=... -e JWT_SECRET=... -p 8081:8081 \
  tohieu1603/micro-exchange-auth-service:latest
```

For the full stack, copy `docker-compose.yml` and replace each `build:` block with:

```yaml
auth-service:
  image: tohieu1603/micro-exchange-auth-service:latest
  env_file: ./auth-service/.env
  ports: ["8081:8081", "9081:9081"]
```

## Manual push (without Jenkins)

Local machine, one-off:

```bash
# Login once
docker login -u tohieu1603

# Backend services — repo root, image per service
for svc in auth-service wallet-service market-service trading-service \
           futures-service notification-service gateway es-indexer; do
  docker build -f $svc/Dockerfile -t tohieu1603/micro-exchange-$svc:latest .
  docker push tohieu1603/micro-exchange-$svc:latest
done

# Frontend — separate repo
cd ../frontendc
docker build -t tohieu1603/micro-exchange-frontend:latest .
docker push tohieu1603/micro-exchange-frontend:latest
```

## Troubleshooting

| Symptom | Fix |
|---|---|
| `docker: Cannot connect to the Docker daemon` | Add jenkins user to `docker` group on the agent: `sudo usermod -aG docker jenkins` then restart agent. |
| `denied: requested access to the resource is denied` on push | PAT scope too narrow — needs `Read & Write`. Or username case mismatch (Docker Hub names are lowercase). |
| `error: pathspec 'Jenkinsfile' did not match any file(s)` | Branch checked out doesn't have Jenkinsfile yet — push it first then re-run. |
| Image build fails on `COPY shared/` | Build context must be repo root, not service dir. The Jenkinsfile passes `-f <svc>/Dockerfile .` from root. |
| Frontend build OOMs | Bump agent memory or pin `NODE_OPTIONS=--max-old-space-size=4096` in the *Build (Next.js)* stage. |

## Unresolved questions

- Do we want **vulnerability scanning** in the pipeline (Trivy, Grype)? Easy to bolt on after the image build.
- Tag with **semver** (from git tag `v1.2.3`) in addition to `latest` + sha?
- **Multi-arch** (amd64 + arm64) builds via buildx — needed if anyone deploys to Apple Silicon servers / Graviton.
