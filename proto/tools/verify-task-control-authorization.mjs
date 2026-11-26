import { loadFixtures, verify } from "./task-control-authorization/verify.mjs";

const result = verify(loadFixtures());
console.log(`Synthetic Task authorization reference: ${result.scenarios} scenarios, ${result.roleCells} role cells, ${result.guards} denial guards. Not production authorization evidence.`);
