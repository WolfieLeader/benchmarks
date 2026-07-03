// Docker image builds (Phase 0B): `just images <entry|all>`. Mirrors the old
// justfile `images` recipe (`docker build -t <image> <server-dir>`). Root-context
// builds per PLAN §2.4 arrive with the restructure (0C).
//
//   node scripts/images.mts <entry|all>     (entry = server name or image tag)

import { type Job, pickTargets, report, runJobs, SERVERS, type Server, targetArg } from "./lib.mts";

const buildable: Server[] = SERVERS.filter((s) => s.image);

const targets = pickTargets(targetArg(), buildable, "images");
const jobs: Job[] = targets.map((s) => ({
  name: s.image ?? s.name,
  steps: [{ label: "build", cmd: `docker build -t ${s.image} .`, cwd: s.dir }]
}));
// Docker builds are heavy; cap parallelism lower than the code-check scripts.
const results = await runJobs(jobs, 3);
report("images", results);
