// database/id: the injectable id generator. The default is `uuid`'s v7; Bun
// entries inject `randomUUIDv7` via setIdGenerator before initializeDatabases()
// (PLAN §3). Every repository that mints ids calls generateId() so the choice is
// made once, at startup, from the entrypoint.

import { v7 as uuidv7 } from "uuid";

let idGenerator: () => string = uuidv7;

export function generateId(): string {
  return idGenerator();
}

export function setIdGenerator(fn: () => string): void {
  idGenerator = fn;
}
