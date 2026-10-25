import { createCollector } from "./support.mjs";
import { runPropagatingVerifier, runRegisteredVerifiers } from "./verifier-runner.mjs";

// Orchestrates one aggregate contract run without touching the process:
// policy checks collect every failure first, then the registered verifiers
// run; any failure stops before the propagating install test. The caller maps
// the returned exit code and streams onto the real process.

function failureReport(failures) {
  return { exitCode: 1, stdout: [], stderr: [failures.map((failure) => `- ${failure}`).join("\n")] };
}

export function runAggregate(plan, runner = {}) {
  const { assert, failures } = createCollector();
  for (const check of plan.checks) check(assert);
  failures.push(...runRegisteredVerifiers(plan.verifiers, runner));
  if (failures.length > 0) return failureReport(failures);

  const installTests = runPropagatingVerifier(plan.installTestPath, runner);
  if (!installTests.ok) return { exitCode: installTests.status, stdout: [], stderr: [installTests.output] };
  return { exitCode: 0, stdout: [...plan.successLines], stderr: [] };
}
