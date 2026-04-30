// Backend CI/CD pipeline — Micro-Exchange.
// Place at repo root of `tohieu1603/go-exchange` as `Jenkinsfile`.
//
// Required Jenkins credentials:
//   - server-ssh-key: SSH private key (oceanroot user) for deploy host.

pipeline {
  agent any

  options {
    timestamps()
    timeout(time: 30, unit: 'MINUTES')
    disableConcurrentBuilds()
    buildDiscarder(logRotator(numToKeepStr: '20'))
  }

  environment {
    GIT_SHA     = "${env.GIT_COMMIT?.take(7) ?: 'dev'}"
    DEPLOY_HOST = '100.112.117.30'
    DEPLOY_USER = 'oceanroot'
  }

  stages {
    stage('Vet + build (Go)') {
      // Each module compiled in parallel — independent go.mod files.
      parallel {
        stage('shared')              { steps { sh 'cd shared && go vet ./... && go build ./...' } }
        stage('auth-service')        { steps { sh 'cd auth-service && go vet ./... && go build ./...' } }
        stage('wallet-service')      { steps { sh 'cd wallet-service && go vet ./... && go build ./...' } }
        stage('market-service')      { steps { sh 'cd market-service && go vet ./... && go build ./...' } }
        stage('trading-service')     { steps { sh 'cd trading-service && go vet ./... && go build ./...' } }
        stage('futures-service')     { steps { sh 'cd futures-service && go vet ./... && go build ./...' } }
        stage('notification-service'){ steps { sh 'cd notification-service && go vet ./... && go build ./...' } }
        stage('gateway')             { steps { sh 'cd gateway && go vet ./... && go build ./...' } }
        stage('es-indexer')          { steps { sh 'cd es-indexer && go vet ./... && go build ./...' } }
      }
    }

    stage('Tests') {
      steps {
        sh '''
          for svc in shared auth-service wallet-service market-service trading-service futures-service notification-service gateway es-indexer; do
            echo "── testing $svc ──"
            ( cd $svc && go test ./... ) || exit 1
          done
        '''
      }
    }

    stage('Deploy to server') {
      // Native-binary deploy via SSH heredoc. Repo lives at
      //   /home/oceanroot/exchange
      // and 8 systemd units exchange-{auth,wallet,market,trading,futures,
      // notification,gateway,es-indexer} run binaries from bin/.
      when {
        anyOf {
          branch 'main'
          expression { return env.GIT_BRANCH == 'origin/main' || env.GIT_BRANCH == 'main' }
        }
      }
      steps {
        withCredentials([sshUserPrivateKey(credentialsId: 'server-ssh-key',
                                           keyFileVariable: 'KEY',
                                           usernameVariable: 'SSH_USER')]) {
          // The closing `EOF` MUST be at column 0 — bash heredoc terminators
          // can't have leading whitespace. Don't re-indent the EOF below.
          sh '''
ssh -i "$KEY" \\
    -o StrictHostKeyChecking=no \\
    -o UserKnownHostsFile=/dev/null \\
    -o ConnectTimeout=10 \\
    "$SSH_USER@$DEPLOY_HOST" bash -s <<'EOF'
set -euo pipefail
cd /home/oceanroot/exchange
git fetch origin
git reset --hard origin/main
git clean -fd
export PATH=$PATH:/usr/local/go/bin

# go.work sync is best-effort — first-time hosts won't have a
# go.work.sum yet, and a mismatch between checked-in sum and
# the resolved deps shouldn't block deploy.
go work sync || true

# Each service builds its own cmd into a shared bin/. The otelgin shim
# grew into multiple cmd/main.go files but isn't always reflected in
# go.mod (drift from manual edits) — pull it on demand.
for svc in auth-service wallet-service market-service \\
            trading-service futures-service notification-service \\
            gateway es-indexer; do
  echo "── build $svc ──"
  cd "/home/oceanroot/exchange/$svc"
  if grep -q otelgin cmd/main.go 2>/dev/null && ! grep -q otelgin go.mod; then
    go get go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin@latest
  fi
  go build -o "/home/oceanroot/exchange/bin/$svc" ./cmd
done

# Restart 8 units. Names omit the -service suffix:
#   exchange-auth, exchange-wallet, …
# Sudoers entry must NOPASSWD-allow systemctl restart exchange-*.
for svc in auth wallet market trading futures notification gateway es-indexer; do
  sudo -n /bin/systemctl restart "exchange-$svc"
done

# Health gate. Gateway exposes /api/health on :3079 once all
# downstream services finish their cold-start.
sleep 5
curl -sf http://127.0.0.1:3079/api/health
echo
echo "Deploy OK"
EOF
          '''
        }
      }
    }
  }

  post {
    success { echo "Pipeline OK. Deployed (sha ${env.GIT_SHA})" }
    failure { echo "Pipeline FAILED at sha ${env.GIT_SHA}" }
  }
}
