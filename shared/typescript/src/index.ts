// @bench/shared — infrastructure shared across every TypeScript server
// (PLAN §3/§4): DB repositories, zod schemas, env parsing, consts/errors, and
// the injectable adapters (uuid generator, Redis repository) that let Bun entries
// keep their native edge while sharing everything else. Routing/handlers/app
// structure stay per-framework and idiomatic — they are NOT here.
//
// This file is the PUBLIC BARREL only: the package's source is split into
// coherent modules (consts / env / schemas / id / db-*) and this index re-exports
// their public surface. It is built with `tsdown` (rolldown) using oxc
// isolated-declarations for the .d.ts (`dts: { oxc: true }`): the modular src is
// bundled into ONE self-contained dist/index.js + dist/index.d.ts, with the DB
// drivers / zod kept external. Every runtime (Node/Bun/Deno) consumes that single
// artifact identically — the same cross-runtime guarantee the old single-file
// build gave, now without hand-collapsing the source into one file. tsc is the
// typecheck gate only (`tsc --noEmit`); it emits nothing.
//
// oxc runs isolated-declarations, not the TS compiler API (which the TS 7.0.1-rc
// does not expose), so every export carries an explicit type annotation. Plain
// `dts: true` (the TS-compiler-backed mode) FAILS on the RC and must not be used
// until TS 7.1 restores the programmatic API (PLAN §4, typescript.md rule 40).

export {
  DEFAULT_LIMIT,
  type ErrorResponse,
  EXPECTED_FORM_CONTENT_TYPE,
  EXPECTED_MULTIPART_CONTENT_TYPE,
  FILE_NOT_FOUND,
  FILE_NOT_TEXT,
  FILE_SIZE_EXCEEDS,
  INTERNAL_ERROR,
  INVALID_FORM_DATA,
  INVALID_JSON_BODY,
  INVALID_MULTIPART,
  makeError,
  MAX_FILE_BYTES,
  MAX_REQUEST_BYTES,
  NOT_FOUND,
  NULL_BYTE,
  ONLY_TEXT_PLAIN,
  SNIFF_LEN
} from "./consts.ts";
export {
  disconnectDatabases,
  initializeDatabases,
  resolveRepository,
  setRedisRepositoryFactory
} from "./db-registry.ts";
export { type CassandraConfig, type DatabaseType, databaseTypes, type UserRepository } from "./db-types.ts";
export { env } from "./env.ts";
export { generateId, setIdGenerator } from "./id.ts";
export {
  buildUser,
  type CreateUser,
  normalizeUser,
  type UpdateUser,
  type User,
  zCreateUser,
  zUpdateUser,
  zUser
} from "./schemas.ts";
