// @bench/hono-app — the single Hono application, shared by the three runtime
// entries (ts-honojs on Node, ts-bun-honojs on Bun, ts-deno-honojs on Deno).
// PLAN §4: Hono is the one framework with first-party support on all three
// runtimes, so it ships as ONE app + three thin per-runtime entrypoints. The
// entrypoints own only the runtime edge (server binding, graceful shutdown, and
// on Bun the native adapter injections); routing/handlers live in this package
// and are identical across runtimes because they use only web-standard
// Request/Response.
//
// Why a separate package (not folded into @bench/shared): the §3 sharing
// boundary keeps routing/handlers/app structure OUT of @bench/shared (DB/schema/
// env infrastructure only). This package is the framework-level shared layer for
// Hono specifically — its sole extra dependency over @bench/shared is `hono`.
//
// This file is the PUBLIC BARREL only: the app is split into coherent modules
// (app / db-routes / params-routes) and this index re-exports the public surface
// (createApp). It is built with `tsdown` (rolldown) using oxc
// isolated-declarations for the .d.ts (`dts: { oxc: true }`): the modular src is
// bundled into ONE self-contained dist/index.js + dist/index.d.ts, with `hono`
// and `@bench/shared` kept external, so all three runtimes consume the identical
// artifact. oxc needs no TS compiler API, so it works under the TS 7.0.1-rc where
// `dts: true` fails (PLAN §4, typescript.md rule 40). tsc is typecheck-only.

export { createApp } from "./app.ts";
