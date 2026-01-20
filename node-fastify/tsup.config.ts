import { defineConfig, type Options } from "tsup";

export default defineConfig((options: Options) => ({
  entryPoints: ["src/index.ts"],
  clean: true,
  format: ["esm"],
  tsconfig: "tsconfig.json",
  sourcemap: true,
  minify: true,
  ...options,
}));
