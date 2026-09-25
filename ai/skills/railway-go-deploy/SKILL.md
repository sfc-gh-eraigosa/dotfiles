---
name: railway-go-deploy
description: Scaffold, containerize, and deploy a Go web service to Railway.app using a multi-stage Docker build and a GitHub Actions CI/CD workflow.
teams: [go, ai-ci]
---

# Railway Go Deploy

This skill scaffolds a production-ready Go web service under a `<app>/` subdirectory, adds a multi-stage `Dockerfile`, and wires a GitHub Actions workflow that auto-deploys to [Railway.app](https://railway.app) on every push to `main`.

## When to use

- Creating a new Go HTTP service that needs a public URL fast.
- Adding Railway deployment to an existing Go service in a monorepo.
- Automating Railway deploys from GitHub (no manual clicks after first setup).

## Teams involved

| Team slug | Why |
|-----------|-----|
| `go` → `go-godev` | Scaffolds idiomatic Go service code (`main.go`, `go.mod`). |
| `go` → `go-goqa` | Adds `/health` endpoint and basic smoke-test advice. |
| `ai-ci` → `the_ci_engineer` | Authors the GitHub Actions deploy workflow and pins all action SHAs. |

## Prerequisites

| What | Where to get it |
|------|----------------|
| Railway account | [railway.app](https://railway.app) — free trial, no card needed |
| `RAILWAY_TOKEN` secret | Railway dashboard → Account Settings → Tokens → New Token; store as a GitHub repo secret |
| `RAILWAY_SERVICE_ID` secret | Railway dashboard → your project → service → Settings → Service ID |

## Usage

### 1. Scaffold a new Go service

Ask `go-godev` to scaffold the service:

```
@go-godev scaffold a new Go HTTP service under playweb/ in this monorepo.
Expose PORT from env (default 8080). Add routes / and /health.
```

Expected output:
```
<app>/
  main.go      # HTTP server reading $PORT, routes / and /health
  go.mod       # module github.com/<owner>/<repo>/<app>
  Dockerfile   # multi-stage: golang:1.22-alpine builder + alpine:3.19 runtime
```

### 2. Add the Dockerfile

`go-godev` will produce the multi-stage Dockerfile. Key properties:
- Builder stage: `golang:1.22-alpine` — compiles static binary (`CGO_ENABLED=0`)
- Runtime stage: `alpine:3.19` — minimal image, no Go toolchain
- Reads `$PORT` at runtime (Railway injects this automatically)
- `EXPOSE 8080` as default

### 3. Wire the GitHub Actions deploy workflow

Ask `the_ci_engineer` to add the workflow:

```
@the_ci_engineer add a Railway deploy workflow for the playweb/ service.
Trigger on push to main, paths: playweb/**. Use RAILWAY_TOKEN and
RAILWAY_SERVICE_ID secrets. Pin all action SHAs per repo convention.
```

The workflow (`deploy-railway.yml`) will:
1. Checkout the repo.
2. Install the Railway CLI.
3. Run `railway up --service $RAILWAY_SERVICE_ID --detach` from the service subdirectory.

### 4. Configure Railway (one-time, in the dashboard)

1. **New Project → Deploy from GitHub** → select your repo.
2. **Service → Settings → Build → Root Directory**: set to `<app>/` (e.g. `playweb`).
3. **Service → Variables**: add `PORT=8080` (Railway also injects its own `PORT` — this is a safe default).
4. **Service → Settings → Networking → Generate Domain** → pick port `8080`.

After this first manual setup, every `git push` to `main` triggers an automatic deploy.

## Secrets reference

```
RAILWAY_TOKEN       # Railway API token (repo or org secret)
RAILWAY_SERVICE_ID  # Railway service ID for this specific service
```

## File reference

| File | Purpose |
|------|---------|
| `<app>/main.go` | Go HTTP server |
| `<app>/go.mod` | Go module definition |
| `<app>/Dockerfile` | Multi-stage Docker build |
| `.github/workflows/deploy-railway.yml` | CI/CD deploy workflow |

## Troubleshooting

| Error | Cause | Fix |
|-------|-------|-----|
| `go.sum not found` | Dockerfile references `go.sum` but no external deps | Remove `go.sum` from `COPY` line in Dockerfile |
| `PORT bind failed` | App hardcodes port instead of reading `$PORT` | Use `os.Getenv("PORT")` with fallback |
| `railway: command not found` | CLI not installed in workflow | Add `npm install -g @railway/cli` step |
| `Unauthorized` in deploy step | `RAILWAY_TOKEN` missing or expired | Re-generate token in Railway dashboard |
