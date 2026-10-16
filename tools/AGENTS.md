# tools/ — pinned build modules for repo tooling

Go modules that exist only to **build a tool at a pinned version** for a Makefile target. They
are not part of the product, are not installed by `install.sh`, and are not scanned or linted
as `sdk/` modules (the `sdk/*` loops skip them).

| Module | Builds | Used by | Why pinned here |
| :-- | :-- | :-- | :-- |
| [`security/`](./security/go.mod) | `gosec`, `govulncheck` | `make security` (CI: Security Scan) | a tagged `gosec` vendors an `x/tools` that cannot read the `.go-version` toolchain's export data; requiring `x/tools` explicitly keeps the scan on the same Go as the code |

**Bumping:** edit the `require` lines in the module's `go.mod`, run `go mod tidy` inside it, run the
Makefile target, commit `go.mod` + `go.sum` together. The policy that those tools enforce lives in
the Makefile target's comment block and in
[`docs/mbo/designs/security-scan-green.md`](../docs/mbo/designs/security-scan-green.md).
