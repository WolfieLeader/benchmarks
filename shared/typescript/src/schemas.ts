// database/types: the domain user schemas (zod-at-boundaries), their inferred TS
// types, and the row-mapping helpers every repository shares. The schema is the
// single source of truth; the TS types are derived via z.infer.

import { z } from "zod";

const zFavoriteNumber = z.number().int().min(0).max(100);
const zOptionalFavoriteNumber = z.preprocess(
  (value) => (value === null ? undefined : value),
  zFavoriteNumber.optional()
);

// oxc isolated-declarations cannot infer the type of a `z.object(...)` call
// expression, so each exported schema carries its explicit ZodObject generic
// (the same type tsc emitted for the single-file build). The z.infer types below
// resolve structurally from these annotations — no circular reference.
export const zUser: z.ZodObject<
  {
    id: z.ZodString;
    name: z.ZodString;
    email: z.ZodEmail;
    favoriteNumber: z.ZodPreprocess<z.ZodOptional<z.ZodNumber>>;
  },
  z.core.$strip
> = z.object({
  id: z.string(),
  name: z.string(),
  email: z.email(),
  favoriteNumber: zOptionalFavoriteNumber
});

export type User = z.infer<typeof zUser>;

export const zCreateUser: z.ZodObject<
  {
    name: z.ZodString;
    email: z.ZodEmail;
    favoriteNumber: z.ZodPreprocess<z.ZodOptional<z.ZodNumber>>;
  },
  z.core.$strip
> = z.object({
  name: z.string().min(1),
  email: z.email(),
  favoriteNumber: zOptionalFavoriteNumber
});

export type CreateUser = z.infer<typeof zCreateUser>;

export const zUpdateUser: z.ZodObject<
  {
    name: z.ZodOptional<z.ZodString>;
    email: z.ZodOptional<z.ZodEmail>;
    favoriteNumber: z.ZodPreprocess<z.ZodOptional<z.ZodNumber>>;
  },
  z.core.$strip
> = z.object({
  name: z.string().min(1).optional(),
  email: z.email().optional(),
  favoriteNumber: zOptionalFavoriteNumber
});

export type UpdateUser = z.infer<typeof zUpdateUser>;

type UserRow = { id: string; name: string; email: string; favoriteNumber?: number | null };

export function normalizeUser(row: UserRow): User {
  const user: User = { id: row.id, name: row.name, email: row.email };
  if (row.favoriteNumber != null) user.favoriteNumber = row.favoriteNumber;
  return user;
}

export function buildUser(id: string, data: CreateUser): User {
  const user: User = { id, name: data.name, email: data.email };
  if (data.favoriteNumber !== undefined) user.favoriteNumber = data.favoriteNumber;
  return user;
}
