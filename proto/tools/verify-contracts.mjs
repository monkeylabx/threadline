import { runAggregate } from "./contract-checks/aggregate.mjs";
import { checkBundleBuilderRefusal } from "./contract-checks/bundle-refusal.mjs";
import { checkGoldenEvidence } from "./contract-checks/golden-evidence.mjs";
import { checkProtoSources } from "./contract-checks/proto-sources.mjs";
import { checkToolchainPolicy } from "./contract-checks/toolchain-policy.mjs";
import { checkRegisteredVerifiers, installTestPath, registeredVerifiers } from "./contract-checks/verifier-registry.mjs";

// Canonical aggregate contract command (`toolchain.lock.json` commands.verify).
// New verifiers join through contract-checks/verifier-registry.mjs; new policy
// checks are their own module listed in `checks` below.

const plan = {
  checks: [
    checkToolchainPolicy,
    checkBundleBuilderRefusal,
    checkProtoSources,
    checkGoldenEvidence,
    checkRegisteredVerifiers,
  ],
  verifiers: registeredVerifiers,
  installTestPath,
  successLines: [
    "Threadline contract structure and representative Golden Frames are valid.",
    "Threadline T015 message/sync behavior fixtures are valid.",
    "Threadline P03-05B authorization behavior fixtures are valid.",
    "Threadline P03-06B Capability Grant signature fixtures are valid.",
    "Threadline P03-07A Audit/Retention metadata fixtures are valid.",
    "Threadline T019 crypto/recovery behavior fixtures are valid.",
    "Threadline agentd local interface fixtures are valid.",
    "Threadline T011 Crypto semantic fixture manifest is valid.",
    "Threadline codegen repository failure-injection tests are valid.",
  ],
};

const result = runAggregate(plan);
for (const line of result.stderr) console.error(line);
for (const line of result.stdout) console.log(line);
process.exit(result.exitCode);
