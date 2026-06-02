# AI Agent Teams

This directory organizes specialized agent teams under `ai/teams/`. Each subdirectory is a self-contained team whose agents share a common domain, model tier policy, and handoff protocol.

See [`MODEL_PARAMS.md`](./MODEL_PARAMS.md) for the full model-parameter convention used in every agent file.

## Teams

| Team | Folder | Size | Model Tier |
|------|--------|------|------------|
| [Web Development](#web) | `web/` | 5 agents | Fast → Deep Think |
| [Go Development](#go) | `go/` | 4 agents | Fast → Deep Think |
| [Terraform & AWS](#terraform-aws) | `terraform-aws/` | 4 agents | Fast → Deep Think |
| [AI & CI](#ai-ci) | `ai-ci/` | 4 agents | Fast (CI/Ops) + Deep Think (Arch) |
| [Architecture](#architecture) | `architecture/` | 4 agents | Deep Think only |

---

## Model Parameter Format

Every agent declares a multi-tool `Model:` block in its frontmatter:

```
# Model:
#   claude:      <slug>   # effort: auto | think | think-hard
#   gemini:      <slug>   # think_budget: 0 | 1024 | 8192
#   antigravity: <slug>   # effort: low | medium | high | max
#   ollama:      <slug>   # local fallback, always present
```

This lets each AI coding tool pick the right model by reading the relevant line.

---

## Team Summaries

### Web

`web/` — Full-stack web development.

| Agent | Alias | Model Tier | Role |
|-------|-------|-----------|------|
| `the_frontend_engineer.md` | `fe` | Standard | React/TS components, a11y, Playwright |
| `the_api_designer.md` | `api` | Standard | REST/GraphQL contracts, migrations |
| `the_web_qa_engineer.md` | `webqa` | Fast | Cross-browser, Lighthouse, visual regression |
| `the_web_security_auditor.md` | `websec` | Think | OWASP, CSP, dependency audit |
| `the_web_architect.md` | `webarch` | Deep Think | Framework governance, SSR/CDN strategy, ADRs |

### Go

`go/` — Go services, CLIs, and libraries.

| Agent | Alias | Model Tier | Role |
|-------|-------|-----------|------|
| `the_go_developer.md` | `godev` | Standard | Idiomatic Go, Cobra CLIs, gRPC |
| `the_go_qa_engineer.md` | `goqa` | Fast | Race detector, fuzz, benchmarks, integration tests |
| `the_go_security_engineer.md` | `gosec` | Think | gosec, govulncheck, supply-chain, IAM |
| `the_go_architect.md` | `goarch` | Deep Think | Interface design, proto governance, pprof |

### Terraform & AWS

`terraform-aws/` — Infrastructure-as-code and AWS platform.

| Agent | Alias | Model Tier | Role |
|-------|-------|-----------|------|
| `the_infra_engineer.md` | `infra` | Standard | Terraform modules, state, tagging |
| `the_platform_engineer.md` | `platform` | Fast | CI/CD pipelines, ECR, ECS/EKS deploys |
| `the_cloud_security_engineer.md` | `cloudsec` | Think | checkov, tfsec, IAM least-privilege, KMS |
| `the_cloud_architect.md` | `cloudarch` | Deep Think | Landing zone, multi-account, Well-Architected |

### AI & CI

`ai-ci/` — AI feature development and CI automation.

| Agent | Alias | Model Tier | Role |
|-------|-------|-----------|------|
| `the_ci_engineer.md` | `ci` | Fast | GitHub Actions, caching, matrix builds |
| `the_ai_developer.md` | `aidev` | Standard | Prompts, RAG, evals, LLM integrations |
| `the_model_ops_engineer.md` | `modelops` | Fast | Ollama/vLLM serving, GPU, model registry |
| `the_ai_architect.md` | `aiarch` | Deep Think | Agentic design, eval frameworks, LLMOps |

### Architecture

`architecture/` — Cross-cutting technical leadership. All agents use thinking models.

| Agent | Alias | Model Tier | Role |
|-------|-------|-----------|------|
| `the_systems_architect.md` | `sysarch` | Deep Think | ADRs, RFCs, dependency graph, capacity |
| `the_principal_engineer.md` | `principal` | Deep Think | Standards, tech debt, code review, patterns |
| `the_security_architect.md` | `secarch` | Deep Think | Threat modeling, zero-trust, identity, compliance |
| `the_engineering_manager.md` | `em` | Deep Think | Roadmap, priority arbitration, stakeholder comms |

---

## Adding a New Team

1. Create a subdirectory: `ai/teams/<team-name>/`
2. Add 3–5 agent `.md` files following the header format in `MODEL_PARAMS.md`
3. Include a `README.md` in the subdirectory (copy from an existing team)
4. Add the team to the table above
5. Create `GEMINI.md` and `CLAUDE.md -> GEMINI.md` symlinks if the directory should be agent-discoverable

## Compatibility

Agents in this directory are compatible with:
- **Claude Code** — reads `claude:` model line + `Context-Strategy` for effort hints
- **Gemini CLI** — reads `gemini:` model line + `think_budget` hint
- **Anti-Gravity CLI** — reads `antigravity:` model line + `effort` hint
- **Ollama** (local) — reads `ollama:` model line as local fallback
