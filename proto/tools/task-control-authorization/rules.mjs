import { principalRules } from "./principal.mjs";
import { factRules } from "./facts.mjs";
import { parentRules } from "./parent.mjs";
import { taskRules } from "./task.mjs";
import { requestRules } from "./request.mjs";
import { approvalRules } from "./approval.mjs";
import { runtimeRules } from "./runtime.mjs";
import { useInitialRules, useRules } from "./use.mjs";
import { reconciliationRules } from "./reconciliation.mjs";

export const rules = [...principalRules, ...factRules, ...parentRules, ...taskRules, ...requestRules, ...useInitialRules, ...approvalRules, ...runtimeRules, ...useRules, ...reconciliationRules];
