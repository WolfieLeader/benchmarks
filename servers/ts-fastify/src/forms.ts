import type { FastifyRequest } from "fastify";

export type FormFields = Record<string, string>;

export async function collectFormFields(request: FastifyRequest): Promise<FormFields> {
  if (request.isMultipart()) {
    const fields: FormFields = {};
    const parts = request.parts();
    for await (const part of parts) {
      if (part.type === "file") {
        part.file.resume();
        continue;
      }
      fields[part.fieldname] = part.value as string;
    }
    return fields;
  }

  const body = (request.body ?? {}) as Record<string, unknown>;
  const fields: FormFields = {};
  for (const [key, value] of Object.entries(body)) {
    if (typeof value === "string") {
      fields[key] = value;
    }
  }
  return fields;
}
