// [DEBUG-242] Temporary, bounded synthetic fixture startup comparison; remove after diagnosis.
import { spawnSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
const expected = `v${readFileSync('.node-version', 'utf8').trim()}`;
const input = JSON.stringify({ jsonrpc:'2.0', id:7, method:'initialize', params:{protocolVersion:1,clientCapabilities:{}} })+'\n';
for (const [name, env] of [['empty',{}],['system-root',{SystemRoot:process.env.SystemRoot}],['inherited',process.env]]) {
  const child = spawnSync(process.execPath,[resolve('crates/agentd/test_support/acp-fixture.mjs'),'exchange',expected],{env,input,encoding:'utf8',timeout:10000,maxBuffer:8192});
  console.log(JSON.stringify({tag:'DEBUG-242',name,node:process.version,expected,status:child.status,signal:child.signal,error:child.error?.code,stdout:child.stdout,stderr:child.stderr}));
}
