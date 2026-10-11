# Sure — research-backed agent handoff

Prepared on 11 October 2026 for implementing a Go, code-first feature-contract verifier.

## Start here

Give the coding agent **INITIAL_AGENT_BRIEF.md** and the owner's local Go CLI template. **BOOTSTRAP_PROMPT.txt** is a short entry prompt. **RESEARCH_NOTES.md** lists the primary references and exactly which owner-repo files were inspected.

## Contents

- INITIAL_AGENT_BRIEF.md — product boundaries, Go adaptation, commands, evidence, Jev, output, trust, corpus, agent loop, and milestones.
- CASE_MATRIX.md — 40 proposed independent evaluation cases and scoring rules.
- examples/ — proposed config/contract, a protocol-shaped Jev request, and a synthetic agent result.
- schemas/run-result.schema.json — draft versioned output contract; the sample result was schema-validated when preparing this handoff.
- RESEARCH_NOTES.md — source links and review limitations.
- BOOTSTRAP_PROMPT.txt — prompt to start implementation.

## Status

This package is **not** the Sure CLI, a running Next.js project, or a measured benchmark. The requested Go/Next.js repositories remain implementation work for the initial agent. No Jev credentials were used; sample numbers and source locations are illustrative. No GitHub repositories, commits, or releases were written during this research. Owner template source is not redistributed here.
