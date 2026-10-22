# MCP test fixtures

`mcp-list-real.txt` is a **byte capture** of real `claude mcp list` output
(Claude Code, 2026-10-20). Server names and targets are replaced with
placeholders (`server-NN`, `*.example.invalid`) and URLs inside error details
are scrubbed; **every status field is byte-for-byte as the CLI printed it**.
Never retype it: the shipped parser matched U+2713 because its fixtures were
typed by hand with the wrong glyph, and the tests stayed green while the count
was 0 in production.

Codepoint census of the status fields: 13× U+2714 `✔` (connected), 5× U+2718
`✘` (failed), 0× U+2713. Needs-authentication lines use ASCII `!`; "Not
configured" lines have no sigil. Totals: 66 servers — 13 connected, 5 failed,
26 needs authentication, 22 not configured.

To refresh it, capture `claude mcp list > raw.txt` and replace only the name
and target on each line; `TestParseStatus_RealCLI` (`-tags integration`)
catches format drift against the installed CLI.
