# Research-evaluation glossary

Plain-language definitions for the jargon that shows up in research dossiers and the
rubric. Link to an entry on a term's **first use** in a doc, e.g.
`[bus factor](../references/glossary.md#bus-factor)` — a link, not extra prose, so the
output stays short but stays learnable. (Playground design docs link to their own
`docs/mbo/glossary.md` copy.)

### Bus factor
The number of people who would have to be "hit by a bus" (leave abruptly) before a
project stalls. **Bus factor = 1** means one maintainer holds effectively all the
knowledge and publishing rights — if they walk, the project (and your dependency on it)
is stranded. Low bus factor is a supply-chain and stability risk.
Learn more: <https://en.wikipedia.org/wiki/Bus_factor>

### Blast radius
How much breaks when one component fails or is compromised. A tool given broad
credentials or sitting in the request path has a large blast radius; an isolated
sidecar with least-privilege access has a small one.
Learn more: <https://cloud.google.com/architecture/framework/reliability/manage-failure-modes>

### Software supply-chain surface
Everything you implicitly trust by installing something: its package registry account,
transitive dependencies, install scripts, and build/publish pipeline. A larger surface
(many deps, single-maintainer publishing) means more ways a compromise reaches you.
Learn more: <https://en.wikipedia.org/wiki/Supply_chain_attack>

### Fail-open (vs fail-closed)
What a guard does when it can't decide. **Fail-open** = allow/continue on error (favours
availability, weakens safety); **fail-closed** = block on error (favours safety). A
security guardrail that "logs and continues" is fail-open.
Learn more: <https://en.wikipedia.org/wiki/Fail-safe>

### Prompt cache / cached reads
Anthropic can cache a stable prompt prefix so repeated tokens are billed at ~10% of the
normal input rate ("cached reads"). Anything that rewrites the prefix "busts" the cache
and you pay full price again — which is why naive token-reduction math can overstate
dollar savings.
Learn more: <https://docs.anthropic.com/en/docs/build-with-claude/prompt-caching>

### CCR (compress · cache · retrieve)
Headroom's pattern for *reversible* compression: shrink a payload, cache the original
locally, and give the model a tool to fetch the full detail on demand. Failure mode: the
model has to notice something's missing, so lossy compression can yield silent wrong
answers rather than errors.
Learn more: <https://headroom-docs.vercel.app/docs>

### Prompt injection
An attack where untrusted content (a web page, tool output, a file) carries instructions
the model then follows. Persisted memory/observation stores make it worse: injected text
can be re-injected into future sessions.
Learn more: <https://owasp.org/www-project-top-10-for-large-language-model-applications/>

### SSRF (server-side request forgery)
Tricking a server into making requests to addresses it shouldn't reach (internal
metadata endpoints, private IPs). Relevant when a proxy/agent allows arbitrary outbound
URLs (`allow_private_urls`).
Learn more: <https://owasp.org/www-community/attacks/Server_Side_Request_Forgery>

### MCP (Model Context Protocol)
An open protocol for exposing tools/data to LLM agents over a standard interface. MCP
servers run as subprocesses with whatever privileges you grant — an unsigned MCP is a
code-execution trust decision.
Learn more: <https://modelcontextprotocol.io>

### CalVer (calendar versioning)
Version numbers derived from the date (e.g. `v2026.8.3`) rather than semantic meaning.
Signals a fast, continuous release train — pin versions.
Learn more: <https://calver.org>

### CVE
A Common Vulnerabilities and Exposures identifier — a public ID for a specific known
security flaw (e.g. `CVE-2026-9277`). "An open CVE in a dependency" = a known,
catalogued vulnerability not yet remediated.
Learn more: <https://www.cve.org>

### SBOM (software bill of materials)
A machine-readable inventory of every component and dependency in a piece of software —
the manifest you'd audit to answer "am I affected by X?". Absence of an SBOM/signing
raises supply-chain uncertainty.
Learn more: <https://www.cisa.gov/sbom>
