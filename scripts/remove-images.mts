// Remove all built Docker images for the roster (`just remove-images`, best
// effort). Manifest-driven: image tags come from scripts/lib.mts's SERVERS
// (servers/*/bench.json `image`), so there is no hardcoded list to drift.
//
//   node scripts/remove-images.mts

import { c, SERVERS, spawnSync } from "./lib.mts";

const images = SERVERS.filter((s) => s.image).map((s) => s.image as string);
if (images.length === 0) {
  console.log(c.yellow("no images to remove"));
  process.exit(0);
}

console.log(c.cyan(`› removing images: ${images.join(", ")}`));
// Best effort: `docker rmi` exits non-zero for images that were never built,
// which mirrors the old `-docker rmi …` recipe — so never fail on that.
spawnSync("docker", ["rmi", ...images], { stdio: "inherit" });
process.exit(0);
