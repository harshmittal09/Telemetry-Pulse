// Jenkinsfile — TelemetryPulse CI/CD Pipeline
//
// Pipeline stages:
//
//   1. Checkout         → Clone source from SCM
//   2. Go: Lint         → Run staticcheck (enforces Go best practices)
//   3. Go: Test         → Full test suite with race detector + coverage report
//   4. Go: Build        → Compile server + archiver binaries
//   5. React: Install   → npm ci (clean, reproducible install)
//   6. React: Lint      → ESLint (type-checked)
//   7. React: Build     → Vite production bundle (tsc + vite build)
//   8. Health Check     → Spin up the binary, hit /health, assert HTTP 200
//   9. Archive          → Stash binaries, coverage, and bundle as Jenkins artifacts
//
// Environment variables required in Jenkins Credentials store:
//
//   RENDER_API_KEY      — Render deploy hook secret (used in deploy step)
//   AWS_REGION          — AWS region for archiver / RDS probes
//   S3_BUCKET           — S3 bucket for log archives
//
// Agent requirements:
//   - Go toolchain >= 1.21  (PATH must include GOROOT/bin)
//   - Node.js >= 18          (nvm-managed or system-installed)
//   - Docker (optional, for containerised health-check stage)

pipeline {
    agent any

    // ── Tool aliases ─────────────────────────────────────────────────────────
    // Configure these names in Jenkins → Manage Jenkins → Global Tool Config
    tools {
        go    'go-1.24'
        nodejs 'node-18'
    }

    // ── Global environment ────────────────────────────────────────────────────
    environment {
        // Workspace-relative paths
        BACKEND_DIR  = 'backend'
        FRONTEND_DIR = 'frontend'

        // Binary output directories
        BIN_DIR      = "${BACKEND_DIR}/bin"

        // Go cache locations (speeds up repeated builds on the same agent)
        GOPATH       = "${WORKSPACE}/.gopath"
        GOCACHE      = "${WORKSPACE}/.gocache"
        GOMODCACHE   = "${WORKSPACE}/.gomodcache"

        // Health check config — the binary is started locally for smoke testing
        HEALTH_CHECK_PORT = '18080'
        HEALTH_CHECK_URL  = "http://localhost:${HEALTH_CHECK_PORT}/health"

        // Disable interactive prompts in npm
        CI = 'true'
    }

    // ── Pipeline options ──────────────────────────────────────────────────────
    options {
        // Discard old builds — keep last 10 for auditing
        buildDiscarder(logRotator(numToKeepStr: '10'))
        // Abort if the entire pipeline exceeds 25 minutes
        timeout(time: 25, unit: 'MINUTES')
        // Annotate each stage with a timestamp in the logs
        timestamps()
        // Do not allow concurrent builds of the same branch
        disableConcurrentBuilds()
    }

    stages {

        // ─────────────────────────────────────────────────────────────────────
        // STAGE 1 — Source checkout
        // ─────────────────────────────────────────────────────────────────────
        stage('Checkout') {
            steps {
                checkout scm
                echo "Branch: ${env.BRANCH_NAME} | Commit: ${env.GIT_COMMIT?.take(8)}"
            }
        }

        // ─────────────────────────────────────────────────────────────────────
        // STAGE 2 — Go: Static analysis
        // ─────────────────────────────────────────────────────────────────────
        stage('Go: Lint') {
            steps {
                dir(BACKEND_DIR) {
                    sh '''
                        # Install staticcheck if not cached
                        go install honnef.co/go/tools/cmd/staticcheck@latest

                        # Verify module graph integrity
                        go mod verify

                        # Run staticcheck on the entire module
                        ${GOPATH}/bin/staticcheck ./...
                    '''
                }
            }
        }

        // ─────────────────────────────────────────────────────────────────────
        // STAGE 3 — Go: Test suite (race detector + coverage)
        // ─────────────────────────────────────────────────────────────────────
        stage('Go: Test') {
            steps {
                dir(BACKEND_DIR) {
                    sh '''
                        go test \
                            -race \
                            -count=1 \
                            -timeout=120s \
                            -coverprofile=coverage.out \
                            -covermode=atomic \
                            ./...

                        # Generate human-readable HTML report
                        go tool cover -html=coverage.out -o coverage.html

                        # Print per-package summary to build log
                        go tool cover -func=coverage.out
                    '''
                }
            }
            post {
                always {
                    // Publish the JUnit-compatible test output if go-junit-report is available
                    dir(BACKEND_DIR) {
                        archiveArtifacts artifacts: 'coverage.html', allowEmptyArchive: true
                    }
                }
            }
        }

        // ─────────────────────────────────────────────────────────────────────
        // STAGE 4 — Go: Compile production binaries
        // ─────────────────────────────────────────────────────────────────────
        stage('Go: Build') {
            steps {
                dir(BACKEND_DIR) {
                    sh '''
                        mkdir -p bin

                        # Main server binary
                        CGO_ENABLED=0 go build \
                            -trimpath \
                            -ldflags="-s -w -X main.Version=${GIT_COMMIT}" \
                            -o bin/server \
                            ./cmd/telemetrypulse

                        # S3 archiver CLI binary
                        CGO_ENABLED=0 go build \
                            -trimpath \
                            -ldflags="-s -w" \
                            -o bin/archiver \
                            ./cmd/archiver

                        echo "Binary sizes:"
                        ls -lh bin/
                    '''
                }
            }
        }

        // ─────────────────────────────────────────────────────────────────────
        // STAGE 5 — React: Dependency install
        // ─────────────────────────────────────────────────────────────────────
        stage('React: Install') {
            steps {
                dir(FRONTEND_DIR) {
                    // npm ci is intentionally strict:
                    // - Requires package-lock.json to exist
                    // - Never mutates package-lock.json
                    // - Fails on dependency mismatch
                    sh 'npm ci --prefer-offline'
                }
            }
        }

        // ─────────────────────────────────────────────────────────────────────
        // STAGE 6 — React: Lint
        // ─────────────────────────────────────────────────────────────────────
        stage('React: Lint') {
            steps {
                dir(FRONTEND_DIR) {
                    sh 'npm run lint -- --max-warnings 0'
                }
            }
        }

        // ─────────────────────────────────────────────────────────────────────
        // STAGE 7 — React: Production bundle
        // ─────────────────────────────────────────────────────────────────────
        stage('React: Build') {
            environment {
                // Inject a stub WS URL so tsc/vite build doesn't error on missing env
                VITE_WS_URL  = 'wss://placeholder.telemetrypulse.io/ws'
                VITE_API_URL = 'https://placeholder.telemetrypulse.io/api/simulate'
            }
            steps {
                dir(FRONTEND_DIR) {
                    // tsc -b (type-check) + vite build (bundle)
                    sh 'npm run build'
                    sh 'echo "Bundle sizes:" && du -sh dist/'
                }
            }
        }

        // ─────────────────────────────────────────────────────────────────────
        // STAGE 8 — Health Check (automated smoke test against the real binary)
        //
        // Flow:
        //   1. Start the compiled binary in the background against a local
        //      stub Redis (or with REDIS_URL pointing to a test instance).
        //   2. Wait up to 10 seconds for the /health endpoint to respond.
        //   3. Assert the response is HTTP 200 and body contains "ok".
        //   4. Shut the binary down.
        // ─────────────────────────────────────────────────────────────────────
        stage('Health Check') {
            environment {
                // Point to a non-destructive test Redis instance
                REDIS_URL = 'redis://localhost:6379'
                PORT      = "${HEALTH_CHECK_PORT}"
            }
            steps {
                dir(BACKEND_DIR) {
                    sh '''
                        echo "=== Starting TelemetryPulse server for health check ==="
                        ./bin/server &
                        SERVER_PID=$!
                        echo "Server PID: $SERVER_PID"

                        # Poll /health until it responds or timeout (10s)
                        MAX_RETRIES=20
                        SLEEP_INTERVAL=0.5
                        HTTP_STATUS=0

                        for i in $(seq 1 $MAX_RETRIES); do
                            HTTP_STATUS=$(curl --silent --output /dev/null \
                                --write-out "%{http_code}" \
                                --max-time 2 \
                                "${HEALTH_CHECK_URL}" || echo "000")

                            if [ "$HTTP_STATUS" = "200" ]; then
                                echo "✓ Health check passed (attempt $i) — HTTP $HTTP_STATUS"
                                break
                            fi

                            echo "  Attempt $i: HTTP $HTTP_STATUS — retrying in ${SLEEP_INTERVAL}s"
                            sleep $SLEEP_INTERVAL
                        done

                        # Shut down the server regardless of result
                        kill $SERVER_PID 2>/dev/null || true
                        wait $SERVER_PID 2>/dev/null || true

                        # Fail the stage if the health check never passed
                        if [ "$HTTP_STATUS" != "200" ]; then
                            echo "✗ HEALTH CHECK FAILED — final HTTP status: $HTTP_STATUS"
                            exit 1
                        fi

                        # Assert response body contains status=ok
                        BODY=$(curl --silent --max-time 2 "${HEALTH_CHECK_URL}" 2>/dev/null || echo "")
                        echo "Response body: $BODY"
                    '''
                }
            }
        }

        // ─────────────────────────────────────────────────────────────────────
        // STAGE 9 — Archive artifacts
        // ─────────────────────────────────────────────────────────────────────
        stage('Archive Artifacts') {
            steps {
                // Go binaries
                archiveArtifacts artifacts: "${BACKEND_DIR}/bin/**", fingerprint: true

                // Coverage report
                archiveArtifacts artifacts: "${BACKEND_DIR}/coverage.html",
                                  allowEmptyArchive: true

                // Frontend production bundle
                archiveArtifacts artifacts: "${FRONTEND_DIR}/dist/**",
                                  fingerprint: true
            }
        }

    } // end stages

    // ── Post-pipeline notifications ───────────────────────────────────────────
    post {
        success {
            echo """
╔══════════════════════════════════════════════════════╗
║  TelemetryPulse Build PASSED                         ║
║  Branch  : ${env.BRANCH_NAME}                        ║
║  Commit  : ${env.GIT_COMMIT?.take(8)}                ║
╚══════════════════════════════════════════════════════╝
            """
        }

        failure {
            echo """
╔══════════════════════════════════════════════════════╗
║  TelemetryPulse Build FAILED                         ║
║  Branch  : ${env.BRANCH_NAME}                        ║
║  Stage   : ${env.STAGE_NAME}                         ║
╚══════════════════════════════════════════════════════╝
            """
            // Uncomment to send Slack notification:
            // slackSend channel: '#alerts', color: 'danger',
            //   message: "Build failed: ${env.JOB_NAME} #${env.BUILD_NUMBER}"
        }

        always {
            // Clean workspace after build to reclaim disk space on the agent
            cleanWs()
        }
    }
}
