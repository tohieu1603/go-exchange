// Backend CI/CD pipeline. Builds + tests every Go service in parallel,
// then on `main` builds and pushes Docker images to Docker Hub.
//
// Required Jenkins credentials:
//   - dockerhub-credentials: Username/password (or PAT) for Docker Hub.
//
// Required Jenkins env / pipeline params (set globally or via Manage Jenkins):
//   - DOCKERHUB_USER: Docker Hub username/org (eg "tohieu1603"). Images are
//     tagged as ${DOCKERHUB_USER}/micro-exchange-<service>:<tag>.
//
// Tags pushed:
//   - <git-short-sha>  — immutable, traceable
//   - latest           — only on main branch
//
// To wire up:
//   1. New Pipeline job → SCM = this repo → Script Path = Jenkinsfile
//   2. Add credentials: Manage Jenkins → Credentials → Global → Add
//      "Username with password", ID = dockerhub-credentials.
//   3. Job parameters → string DOCKERHUB_USER (default tohieu16).
//   4. Make sure the Jenkins agent has docker + go 1.25 (or use a Docker
//      agent: agent { docker { image 'golang:1.25' } } — but you still need
//      docker-in-docker for the image-build stage).

pipeline {
  agent any

  parameters {
    string(name: 'DOCKERHUB_USER', defaultValue: 'tohieu16',
           description: 'Docker Hub user/org for image tags')
  }

  options {
    timestamps()
    timeout(time: 30, unit: 'MINUTES')
    disableConcurrentBuilds()
    buildDiscarder(logRotator(numToKeepStr: '20'))
  }

  environment {
    GIT_SHA = "${env.GIT_COMMIT?.take(7) ?: 'dev'}"
    IS_MAIN = "${env.BRANCH_NAME == 'main' || env.GIT_BRANCH == 'origin/main' || env.GIT_BRANCH == 'main'}"
  }

  stages {
    stage('Vet + build (Go)') {
      // Each module compiled in parallel — independent go.mod files.
      // Failure in any one fails the build before any image is pushed.
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
      // Per-service `go test`. Skipped if a service has no tests yet (script
      // exits 0 when no test files match).
      steps {
        sh '''
          for svc in shared auth-service wallet-service market-service trading-service futures-service notification-service gateway es-indexer; do
            echo "── testing $svc ──"
            ( cd $svc && go test ./... ) || exit 1
          done
        '''
      }
    }

    stage('Build + push Docker images') {
      // Image build context is the repo root (Dockerfiles COPY shared/ + their own dir + go.work).
      when {
        anyOf {
          branch 'main'
          expression { return env.GIT_BRANCH == 'origin/main' || env.GIT_BRANCH == 'main' }
        }
      }
      steps {
        withCredentials([usernamePassword(credentialsId: 'dockerhub-credentials',
                                          usernameVariable: 'DH_USER',
                                          passwordVariable: 'DH_PASS')]) {
          sh '''
            set -eu
            echo "$DH_PASS" | docker login -u "$DH_USER" --password-stdin

            for svc in auth-service wallet-service market-service trading-service futures-service notification-service gateway es-indexer; do
              IMAGE="${DOCKERHUB_USER}/micro-exchange-${svc}"
              echo "── building $IMAGE ──"
              docker build -f ${svc}/Dockerfile -t ${IMAGE}:${GIT_SHA} -t ${IMAGE}:latest .
              docker push ${IMAGE}:${GIT_SHA}
              docker push ${IMAGE}:latest
            done

            docker logout
          '''
        }
      }
    }
  }

  post {
    always {
      // Reclaim disk on the agent — image builds accumulate fast.
      sh 'docker image prune -f --filter "until=24h" || true'
    }
    success {
      echo "Pipeline OK. Images tagged ${env.GIT_SHA}${IS_MAIN == 'true' ? ' + latest (pushed to Docker Hub)' : ' (not pushed — non-main branch)'}"
    }
  }
}
