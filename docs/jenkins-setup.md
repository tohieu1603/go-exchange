# Jenkins + SSH deploy setup

[Jenkinsfile](../Jenkinsfile) drives a 4-stage backend pipeline:
1. **Vet + build** — `go vet` + `go build` per service in parallel.
2. **Tests** — `go test -race -cover -coverprofile=coverage.out ./...` per service.
3. **Coverage gate** — soft floor 2% per *tested* service; raise as tests grow.
4. **Deploy to server** — SSH into `$DEPLOY_HOST`, run [`infra/jenkins/exchange-deploy.sh`](../infra/jenkins/exchange-deploy.sh) (streamed via SSH stdin so the script is version-controlled in this repo, not on the Jenkins server).

Frontend has its own Jenkinsfile in [`tohieu1603/go_exchange_fe`](https://github.com/tohieu1603/go_exchange_fe).

## How deploy actually works

```
Jenkins agent                            Deploy host (100.112.117.30)
─────────────                            ──────────────────────────────
ssh -i $KEY oceanroot@host \
  "BRANCH=main GIT_SHA=abc bash -s" \
  < infra/jenkins/exchange-deploy.sh  ─►  bash reads script from stdin
                                          ├─ git fetch + reset --hard origin/main
                                          ├─ go build -o <svc>/bin/<svc> for all 8
                                          ├─ sudo systemctl restart exchange-*
                                          └─ curl /api/health (15× 2s retry)
```

**No Docker on the host** — services are native Go binaries managed by systemd. Logs go to journald (and Filebeat ships them to ES via the dev compose stack).

## One-time host setup

### 1. User + repo location

```bash
# As root on the deploy host:
useradd -m -s /bin/bash oceanroot
# Allow oceanroot to restart only the exchange-* units (least privilege):
cat > /etc/sudoers.d/oceanroot-exchange <<'EOF'
oceanroot ALL=(ALL) NOPASSWD: /bin/systemctl restart exchange-*, /bin/systemctl status exchange-*, /bin/journalctl -u exchange-*
EOF

# Clone repo to a stable path:
mkdir -p /srv && chown oceanroot:oceanroot /srv
sudo -u oceanroot git clone https://github.com/tohieu1603/go-exchange /srv/micro-exchange
cd /srv/micro-exchange
sudo -u oceanroot git checkout main
```

### 2. Go toolchain

Match `go.work` directive (1.25):

```bash
curl -L https://go.dev/dl/go1.25.7.linux-amd64.tar.gz | sudo tar -C /usr/local -xz
ln -sf /usr/local/go/bin/go /usr/local/bin/go
```

### 3. Per-service env files

```bash
sudo mkdir -p /etc/exchange
# Copy each .env.example from the repo and fill secrets:
for svc in auth-service wallet-service market-service trading-service \
           futures-service notification-service gateway es-indexer; do
  sudo cp /srv/micro-exchange/$svc/.env.example /etc/exchange/$svc.env
done
sudo chown -R oceanroot:oceanroot /etc/exchange
sudo chmod 640 /etc/exchange/*.env  # only owner reads secrets
sudo $EDITOR /etc/exchange/auth-service.env  # repeat for each
```

### 4. systemd units

The repo ships a template + renderer:

```bash
cd /srv/micro-exchange/infra/systemd
sudo ./install-units.sh                 # writes 8 unit files + reloads systemd
sudo systemctl enable --now exchange-{auth,wallet,market,trading,futures,notification,es-indexer,gateway}.service
sudo systemctl status exchange-gateway.service
```

If you rename the deploy user or paths, override before running:

```bash
USER=deploy REPO_DIR=/srv/exchange ENV_DIR=/srv/exchange/env sudo -E ./install-units.sh
```

### 5. Health endpoint

The deploy script polls `http://localhost:8080/api/health` (gateway). Make sure the gateway exposes a 200-on-ready handler — adjust `HEALTH_URL` in `exchange-deploy.sh` if your gateway listens elsewhere.

## Jenkins setup

### Add SSH credential

`Manage Jenkins` → `Credentials` → `(global)` → `Add Credentials`:

- **Kind**: SSH Username with private key
- **ID**: `server-ssh-key` ← exact, Jenkinsfile references this
- **Username**: `oceanroot`
- **Private Key**: paste the deploy key (whose public side is in `~oceanroot/.ssh/authorized_keys` on the host)

### Create the pipeline job

`New Item` → name `micro-exchange-backend` → **Pipeline** → OK.

- **Pipeline → Definition**: *Pipeline script from SCM*
- **SCM**: Git → repo URL → branch `*/main` (or `*/*` for multibranch)
- **Script Path**: `Jenkinsfile`
- (Optional) **Build Triggers** → "GitHub hook trigger for GITScm polling" + GitHub webhook to `https://<jenkins>/github-webhook/`

## Manual deploy (without Jenkins)

If Jenkins is down or for first-time bring-up:

```bash
ssh oceanroot@100.112.117.30 \
  "BRANCH=main GIT_SHA=$(git rev-parse --short HEAD) bash -s" \
  < /path/to/repo/infra/jenkins/exchange-deploy.sh
```

## Troubleshooting

| Symptom | Likely cause / fix |
|---|---|
| `Permission denied (publickey)` on SSH stage | Pub key not in `~oceanroot/.ssh/authorized_keys`, or wrong credential ID. |
| `sudo: a password is required` mid-deploy | Sudoers entry above missing. Check `/etc/sudoers.d/oceanroot-exchange`. |
| `exchange-<svc>.service: failed to start` | `journalctl -u exchange-<svc> -n 50`. Most often `.env` value missing or DB unreachable. |
| Build OOMs on the host | `GOMEMLIMIT=512MiB` in the env file or build with fewer parallel jobs in `exchange-deploy.sh` (`-P2` instead of `-P4`). |
| Health gate flakes after restart | DB warm-up >30s. Bump the loop in `exchange-deploy.sh` to 30 iterations or move the health check into the unit's `ExecStartPost`. |

## What lives where

| Path | Purpose |
|---|---|
| [`Jenkinsfile`](../Jenkinsfile) | Pipeline definition (build/test/deploy stages). |
| [`infra/jenkins/exchange-deploy.sh`](../infra/jenkins/exchange-deploy.sh) | The actual deploy logic (runs on the host via SSH stdin). |
| [`infra/systemd/exchange-template.service`](../infra/systemd/exchange-template.service) | systemd unit template — placeholders rendered by `install-units.sh`. |
| [`infra/systemd/install-units.sh`](../infra/systemd/install-units.sh) | Renders the template into 8 unit files + reloads systemd. |
| `/etc/exchange/<svc>.env` | Real env files (NOT in git — copied from `.env.example` then filled). |

## Unresolved questions

- **Roll-forward only**? On a failed health check the deploy script aborts but doesn't roll back the binaries. Worth adding an automatic revert if the previous binary is still on disk?
- **Zero-downtime**? `systemctl restart` produces a brief 502 window. Switch to `systemctl reload` (if services support SIGHUP) or run two instances behind a proxy with sticky tagging?
- **Secrets management**? `.env` files on the host are ergonomic but unaudited. Move to Vault / Doppler / SOPS as scale grows?
