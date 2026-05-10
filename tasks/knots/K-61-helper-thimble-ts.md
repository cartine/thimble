# K-61 — Ship thimble-ts helper library

- Status: planned. Stays READY_FOR_PLANNING until Thimble v0.1.0+ ships.
- Knot id: see `kno ls -g ref:k-61`.
- Follow-up to: K-58 (`thimble exec`), K-59 (doc realignment).

## Goal

A tiny official TypeScript / Node.js client library that wraps the
`thimble exec` stdin protocol with a single function:
`const secrets = await thimble.readSecrets()`. The library does NOT
call `age` or speak to the store directly — it parses the dotenv body
`thimble exec` already pipes into the child process.

## Why a library at all

See [K-60](K-60-helper-thimble-py.md) — same rationale, different
language. The library standardizes parsing across languages so subtle
escape-handling bugs in hand-rolled user code don't bite.

## Acceptance

1. Repository at `github.com/cartine/thimble-ts` with LICENSE
   (Apache-2.0), README, tests (`vitest` or `node --test`), CI.
2. Single public API: `readSecrets(): Promise<Record<string, string>>`.
   Exported as both ESM and CommonJS.
3. Distribution: `npm install @thimble/secrets` works.
4. Parser handles the same dotenv subset as
   [`internal/dotenv/dotenv.go`](../../internal/dotenv/dotenv.go).
5. Tests cover: happy path, malformed line, empty stdin, large value
   (>64 KiB), trailing newlines.
6. README example shows the canonical pattern; links back to Thimble's
   main README "Consuming Secrets" section.
7. TypeScript types ship in the package; no separate `@types/...`.

## Constraints

- ZERO third-party runtime deps. Node stdlib only.
- API surface MUST stay tiny — one function.
- No mention of third-party vault products in the docs.
- v0.1.0 release waits until Thimble itself is at v0.1.0+.
