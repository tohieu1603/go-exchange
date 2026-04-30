# Jenkins + SSH deploy setup (Backend)

[Jenkinsfile](../Jenkinsfile) drives a 3-stage backend pipeline:
1. **Vet + build** — `go vet` + `go build` per service in parallel.
2. **Tests** — `go test ./...` per service (fail-fast loop).
3. **Deploy to server** — on `main`, SSH into `$DEPLOY_HOST` and run an inline bash heredoc that pulls, rebuilds, restarts systemd units, and curls the gateway health endpoint.

Frontend has its own pipeline in [`tohieu1603/go_exchange_fe`](https://github.com/tohieu1603/go_exchange_fe) — same SSH credential, same patterns, separate repo.

## What the deploy stage does

```
ssh oceanroot@100.112.117.30 bash -s <<EOF
  cd /home/oceanroot/exchange
  git fetch origin && git reset --hard origin/main && git clean -fd
  go work sync || true
  for svc in auth-service … es-indexer; do
    go build -o /home/oceanroot/exchange/bin/$svc ./cmd
  done
  for svc in auth wallet market trading futures notification gateway es-indexer; do
    sudo -n /bin/systemctl restart "exchange-$svc"
  done
  curl -sf http://127.0.0.1:3079/api/health
EOF
```

Service-dir name has the `-service` suffix; unit name does NOT — `exchange-auth.service`, `exchange-gateway.service`, etc.

The otelgin shim is `go get`-installed on demand if `cmd/main.go` references it but `go.mod` doesn't list it. Drift catcher for the host's local module cache.

## One-time host setup

### 1. Deploy user + sudoers

```bash
sudo useradd -m -s /bin/bash oceanroot
# Least-privilege sudoers: only restart/status/journalctl on exchange-*
sudo tee /etc/sudoers.d/oceanroot-exchange <<'EOF'
oceanroot ALL=(ALL) NOPASSWD: /bin/systemctl restart exchange-*, /bin/systemctl status exchange-*, /bin/journalctl -u exchange-*
EOF
```

### 2. Add public key

Public side of the Jenkins `server-ssh-key` credential goes into `~oceanroot/.ssh/authorized_keys`.

### 3. Repo + Go toolchain

```bash
sudo -u oceanroot git clone https://github.com/tohieu1603/go-exchange /home/oceanroot/exchange

# Match go.work directive (1.25)
curl -L https://go.dev/dl/go1.25.7.linux-amd64.tar.gz | sudo tar -C /usr/local -xz
sudo ln -sf /usr/local/go/bin/go /usr/local/bin/go
```

### 4. Per-service env files

```bash
sudo mkdir -p /etc/exchange
for svc in auth-service wallet-service market-service trading-service \
           futures-service notification-service gateway es-indexer; do
  sudo cp /home/oceanroot/exchange/$svc/.env.example /etc/exchange/$svc.env
done
sudo chown -R oceanroot:oceanroot /etc/exchange
sudo chmod 640 /etc/exchange/*.env
sudo $EDITOR /etc/exchange/auth-service.env  # repeat for each
```

### 5. systemd units

```bash
cd /home/oceanroot/exchange/infra/systemd
sudo ./install-units.sh                 # writes 8 unit files + reloads
sudo systemctl enable --now exchange-{auth,wallet,market,trading,futures,notification,gateway,es-indexer}
sudo systemctl status exchange-gateway
```

The installer renders [`exchange-template.service`](../infra/systemd/exchange-template.service) for each service. Override `REPO_DIR`/`ENV_DIR`/`RUN_USER` if your layout differs.

### 6. Health endpoint

The deploy stage hits `http://127.0.0.1:3079/api/health`. Adjust the port in `Jenkinsfile` if your gateway listens elsewhere.

## Jenkins setup

### Add SSH credential

`Manage Jenkins` → `Credentials` → `(global)` → `Add Credentials`:

- **Kind**: SSH Username with private key
- **ID**: `server-ssh-key` ← exact, Jenkinsfile references this
- **Username**: `oceanroot`
- **Private Key**: paste the key whose public side is in `authorized_keys`

### Create the pipeline job

`New Item` → `micro-exchange-backend` → **Pipeline** → OK.
- **Pipeline → Definition**: *Pipeline script from SCM*
- **SCM**: Git → repo URL → branch `*/main`
- **Script Path**: `Jenkinsfile`
- (Optional) GitHub webhook to `/github-webhook/` for auto-trigger

## Manual deploy (without Jenkins)

```bash
ssh oceanroot@100.112.117.30 bash <<'EOF'
  set -euo pipefail
  cd /home/oceanroot/exchange
  git fetch origin && git reset --hard origin/main
  for svc in auth-service wallet-service market-service trading-service \
              futures-service notification-service gateway es-indexer; do
    cd /home/oceanroot/exchange/$svc
    go build -o /home/oceanroot/exchange/bin/$svc ./cmd
  done
  for svc in auth wallet market trading futures notification gateway es-indexer; do
    sudo -n /bin/systemctl restart "exchange-$svc"
  done
  curl -sf http://127.0.0.1:3079/api/health
EOF
```

## Troubleshooting

| Symptom | Likely cause / fix |
|---|---|
| `Permission denied (publickey)` | Pub key not in `~oceanroot/.ssh/authorized_keys`, or wrong credential ID. |
| `sudo: a password is required` | Sudoers entry missing — see step 1. |
| Build fails on otelgin import | The Jenkinsfile auto-runs `go get` for it, but only if cmd/main.go references it AND it's missing from go.mod. Manual fix: commit the dep to go.mod. |
| `exchange-<svc>` fails to start | `journalctl -u exchange-<svc> -n 50`. Most often `.env` value missing or DB unreachable. |
| Health curl fails after deploy | Bump the `sleep 5` to `sleep 15` for slower hosts, or move to a poll loop. |

## What lives where

| Path | Purpose |
|---|---|
| [`Jenkinsfile`](../Jenkinsfile) | Pipeline definition (deploy logic inline in heredoc). |
| [`infra/systemd/exchange-template.service`](../infra/systemd/exchange-template.service) | systemd unit template — placeholders rendered by `install-units.sh`. |
| [`infra/systemd/install-units.sh`](../infra/systemd/install-units.sh) | Renders 8 unit files + reloads systemd. |
| `/etc/exchange/<svc>.env` | Real env files (NOT in git — copied from `.env.example` then filled). |
| `/home/oceanroot/exchange/bin/<svc>` | Built binaries (overwritten on every deploy). |

## Unresolved questions

- **Rollback** — Current Jenkinsfile has no auto-revert if `curl health` fails post-restart. Worth adding a `<svc>.previous` snapshot + swap-back?
- **Zero-downtime** — `systemctl restart` produces a brief 502 window. Acceptable for now; revisit when traffic justifies blue/green.
- **Secrets** — `.env` files on the host are ergonomic but unaudited. Move to Vault / Doppler / SOPS as scale grows?
