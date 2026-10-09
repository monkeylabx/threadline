import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { fromBinary, toBinary } from "@bufbuild/protobuf";
import { SubmitRunInputRequestSchema, LocalAgentService } from "@threadline/proto/threadline/local_agent/v1/agent_service_pb";

test("exported local-agent SDK consumes the contract wire fixture", () => {
  const fixture = JSON.parse(readFileSync(new URL("../../../test/fixtures/proto/agentd-local/wire.json", import.meta.url)));
  const frame = fixture.frames.find((value) => value.type === "SubmitRunInputRequest");
  const wire = Buffer.from(frame.hex, "hex");
  const message = fromBinary(SubmitRunInputRequestSchema, wire);
  assert.equal(message.runId, frame.json.runId);
  assert.equal(message.inputId, frame.json.inputId);
  assert.equal(message.text, frame.json.text);
  assert.deepEqual(Buffer.from(toBinary(SubmitRunInputRequestSchema, message)), wire);
  assert.equal(LocalAgentService.typeName, "threadline.local_agent.v1.LocalAgentService");
});
