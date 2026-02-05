import { z } from "zod";

const zFavoriteNumber = z.number().int().min(0).max(100);
const zOptionalFavoriteNumber = z.preprocess(
  (value) => (value === null ? undefined : value),
  zFavoriteNumber.optional()
);

export const zUser = z.object({
  id: z.string(),
  name: z.string(),
  email: z.email(),
  favoriteNumber: zOptionalFavoriteNumber
});

export type User = z.infer<typeof zUser>;

export const zCreateUser = z.object({
  name: z.string().min(1),
  email: z.email(),
  favoriteNumber: zOptionalFavoriteNumber
});

export type CreateUser = z.infer<typeof zCreateUser>;

export const zUpdateUser = z.object({
  name: z.string().min(1).optional(),
  email: z.email().optional(),
  favoriteNumber: zOptionalFavoriteNumber
});

export type UpdateUser = z.infer<typeof zUpdateUser>;

type UserRow = {
  id: string;
  name: string;
  email: string;
  favoriteNumber?: number | null;
};

export function normalizeUser(row: UserRow): User {
  const user: User = { id: row.id, name: row.name, email: row.email };
  if (row.favoriteNumber != null) user.favoriteNumber = row.favoriteNumber;
  return user;
}

export function buildUser(id: string, data: CreateUser): User {
  const user: User = { id, name: data.name, email: data.email };
  if (data.favoriteNumber !== undefined) {
    user.favoriteNumber = data.favoriteNumber;
  }
  return user;
}
