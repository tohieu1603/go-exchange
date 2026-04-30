// Backend CI/CD pipeline — Micro-Exchange.
//
// On every push to any branch:
//   1. go vet + go build per service (parallel, fail-fast).
//   2. go test -race -cover per service.
//   3. Coverage floor 2% per tested service.
//
// On `main` only:
//   4. SSH into the deploy host, run infra/jenkins/exchange-deploy.sh
//      (streamed from the Jenkins workspace via stdin so the script is
//      version-controlled in this repo, not on the Jenkins server).
//
// Required Jenkins credentials:
//   - server-ssh-key  : SSH private key (Username with private key) for
//                       the deploy user on $DEPLOY_HOST.
//
// Wire-up:
//   1. Manage Jenkins → Credentials → Global → Add → SSH Username with
//      private key. ID = server-ssh-key. Username = oceanroot. Paste key.
//   2. New Item → Pipeline → SCM = this repo → Script Path = Jenkinsfile.
//   3. (Optional) Build Triggers → "GitHub hook trigger for GITScm polling"
//      and add a webhook on GitHub.

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
    BRANCH_NAME = "${env.BRANCH_NAME ?: 'main'}"
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
      // -race catches concurrency bugs in matching engine + WS hub.
      // -coverprofile feeds the next stage's floor check.
      steps {
        sh '''
          for svc in shared auth-service wallet-service market-service trading-service futures-service notification-service gateway es-indexer; do
            echo "── testing $svc ──"
            ( cd $svc && go test -race -cover -coverprofile=coverage.out ./... ) || exit 1
          done
        '''
      }
    }

    stage('Coverage gate') {
      // Soft floor — fails the build if any *tested* service drops below
      // the threshold. Services reporting 0% (no tests yet) are skipped.
      // Raise this number as new tests land.
      environment { COVERAGE_MIN = '2.0' }
      steps {
        sh '''
          set -eu
          fail=0
          for svc in shared auth-service wallet-service market-service trading-service futures-service notification-service gateway es-indexer; do
            f="${svc}/coverage.out"
            [ -s "$f" ] || continue
            pct=$(cd $svc && go tool cover -func=coverage.out | awk '/^total:/ {gsub("%","",$3); print $3}')
            if awk -v p="$pct" 'BEGIN { exit (p+0 == 0) }'; then
              echo "── $svc coverage: 0% (no tests, skipped)"
              continue
            fi
            echo "── $svc coverage: ${pct}% (floor ${COVERAGE_MIN}%)"
            awk -v got="$pct" -v min="$COVERAGE_MIN" 'BEGIN { exit (got+0 < min+0) }' || {
              echo "FAIL: $svc below coverage floor"
              fail=1
            }
          done
          [ "$fail" = "0" ]
        '''
      }
    }

    stage('Deploy to server') {
      // Native-binary deploy. The script lives in the repo so it's
      // versioned alongside the code that depends on it; we stream it
      // over SSH stdin so no copy step is needed.
      when {
        anyOf {
          branch 'main'
          expression { return env.GIT_BRANCH == 'origin/main' || env.GIT_BRANCH == 'main' }
        }
      }
      steps {
        withCredentials([sshUserPrivateKey(credentialsId: 'server-ssh-key',
                                           keyFileVariable: 'SSH_KEY',
                                           usernameVariable: 'SSH_USER')]) {
          sh '''
            set -eu
            ssh -i "$SSH_KEY" \\
                -o StrictHostKeyChecking=no \\
                -o UserKnownHostsFile=/dev/null \\
                -o ConnectTimeout=10 \\
                "$SSH_USER@$DEPLOY_HOST" \\
                "BRANCH=$BRANCH_NAME GIT_SHA=$GIT_SHA bash -s" \\
                < infra/jenkins/exchange-deploy.sh
          '''
        }
      }
    }
  }

  post {
    success {
      echo "Pipeline OK. sha=${env.GIT_SHA} branch=${env.BRANCH_NAME}"
    }
    failure {
      echo "Pipeline FAILED at sha ${env.GIT_SHA}"
    }
  }
}
