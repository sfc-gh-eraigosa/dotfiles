# Source Code & Agent Skills (src)

This directory contains the source code for custom tools and specialized agent skills.

## Projects

> **Go modules now live in [`sdk/`](../sdk/AGENTS.md)**, not here — `gss`, `gsl`, `wol`, and `tmux-mgr` moved out of `src/` in the `src/` → `sdk/` cutover. `src/` holds non-Go tooling and agent skills.

- `ssh-host-finder/`: A skill for finding SSH hosts on the local network.
  - [Agent Skill Instructions](./ssh-host-finder/SKILL.md)

## Development Workflow

- Custom tools are typically developed in Go or Python.
- Agent skills are defined using `SKILL.md` files which provide specialized guidance.

## Library standards & license selection

All third-party code that ships in `src/**` — Go modules, Python packages,
embedded vendored snippets, and CLI tools we recommend or wrap in agent
skills — must satisfy the license and library standards below. There is
no exception path; if a candidate library only exists under a banned
license, the answer is "don't take that dependency, find another way".

### Allowed licenses (permissive)

Take these freely:

- **Apache License 2.0** (preferred; explicit patent grant).
- **MIT** / **Expat**.
- **BSD-2-Clause**, **BSD-3-Clause**.
- **ISC**.
- **Public domain dedications**: **Unlicense**, **CC0-1.0**.
- **Zlib**, **BSL-1.0** (Boost).

### Flag-for-review licenses

Allowed only after explicit review on the PR introducing the dependency.
Note in the PR description why a more permissive alternative wasn't
viable and confirm any isolation requirements are met:

- **MPL-2.0** — file-scoped copyleft; OK if the dependency stays in
  its own files / package and we don't fork-and-modify its source.
- **Apache-2.0 with LLVM Exception** — fine, but call out the
  exception clause in the PR.
- **EPL-2.0**, **CDDL-1.0** — borderline; usually decline unless no
  permissive alternative exists.

### Banned licenses (copyleft)

Do not take, do not vendor, do not recommend in a skill:

- **GPL** (any version, including GPL-2.0, GPL-3.0).
- **AGPL** (any version).
- **LGPL** (any version) — viral linking risk for our statically-linked
  Go binaries; never assume "but we link dynamically".
- **SSPL**, **BUSL** / **Business Source License**, **Commons Clause**
  add-ons, **Elastic License** — not OSI-permissive; treat as
  proprietary.
- Any "source-available but commercial-use restricted" license.

If a candidate library's `LICENSE` file lists multiple licenses
(dual-licensing), the project may be used **only** if at least one of the
listed licenses is in the Allowed or Flag-for-review set. For example,
`Apache-2.0 OR MIT` is fine; `MIT OR GPL-3.0` is fine (pick MIT);
`GPL-2.0 OR Commercial` is **not** fine.

### Companion CLI tools recommended in skills

The same license rules apply to any CLI tool a `SKILL.md` instructs an
agent to install or invoke (e.g. rebase helpers, formatters, linters).
A skill must not recommend a GPL/AGPL/LGPL CLI as a dependency. We also
rule out **cloud-gated / SaaS-only CLIs** (where the binary is closed-
source and requires a vendor account for non-trivial use) regardless of
their license tag — they don't give us the freedom-to-fork that the
permissive ecosystem does.

### Process for adding a dependency

When a PR introduces a new third-party library or recommends a new
companion CLI:

1. Cite the **LICENSE file URL** from the canonical upstream repository
   in the PR description (not the project website).
2. State the license verbatim (e.g. "Apache-2.0", not "open source").
3. If it's in the Flag-for-review set, explain why no Allowed-list
   alternative is sufficient.
4. For Go modules, run `go mod tidy` and confirm `go.sum` doesn't drag
   in transitive banned licenses (a `go-licenses` or similar audit pass
   is encouraged when the dependency tree is non-trivial).

### Verification

License claims must be verified against the actual `LICENSE` file in
the upstream repo. README badges and marketing pages are not
authoritative — projects relicense and the badge often lags. When in
doubt, link the specific file/blob commit you read.

### Testing & TDD Standards

All code modifications and new feature development under `src/` **MUST** follow a Test-Driven Development (TDD) workflow and adhere to standard testing practices for the language (e.g., standard Go testing patterns).

1. **Test First**: Start with the minimal tests needed for the feature. Include positive test cases, negative test cases, and edge cases (e.g., empty inputs). Add mocking if external dependencies are involved.
2. **Implement**: Add the new features to satisfy the tests.
3. **Validate**: Run the tests to validate the features.
4. **Iterate**: Debug what went wrong and iterate by adding more test cases until the desired result is achieved.

**Coverage Goal**: Aim for a minimum test coverage standard of **>60%** for all packages. When summarizing work, always include basic stats: added/removed lines, added/removed test cases, overall test coverage, and confidence level that the changes work.
