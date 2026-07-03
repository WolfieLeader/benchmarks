// Docker image builds (Phase 0B): `just images <entry|all>`. Root-context builds
// per PLAN §2.4 (0C): the build context is the repo root and the Dockerfile is
// selected with `-f servers/<entry>/Dockerfile`, so each image can COPY both its
// own folder and (from 0D) the shared/<lang> layer. A root `.dockerignore` keeps
// the context small.
//
//   node scripts/images.mts <entry|all>     (entry = server name or image tag)

import { join } from "node:path";
import { type Job, pickTargets, repoRoot, report, runJobs, SERVERS, type Server, targetArg } from "./lib.mts";

const buildable: Server[] = SERVERS.filter((s) => s.image);

const targets = pickTargets(targetArg(), buildable, "images");
const jobs: Job[] = targets.map((s) => ({
  name: s.image ?? s.name,
  steps: [{ label: "build", cmd: `docker build -t ${s.image} -f ${join(s.dir, "Dockerfile")} .`, cwd: repoRoot }]
}));
// Docker builds are heavy; cap parallelism lower than the code-check scripts.
const results = await runJobs(jobs, 3);
report("images", results);
