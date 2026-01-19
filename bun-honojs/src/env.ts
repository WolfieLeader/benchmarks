import { type } from "arktype";

const tEnv = type({
  HOST: "string.url | string.ip | 'localhost' = '0.0.0.0'",
  PORT: "string.numeric.parse = '3005'",
}).narrow((data, ctx) => {
  data.HOST = data.HOST === "localhost" ? "0.0.0.0" : data.HOST;
  return data.PORT < 1 || data.PORT > 65535 ? ctx.reject("PORT must be between 1 and 65535") : true;
});

export const env = tEnv.assert(process.env);
