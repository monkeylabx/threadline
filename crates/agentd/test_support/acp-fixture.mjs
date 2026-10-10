import { createInterface } from 'node:readline';

// Synthetic pipe peer only: no tools, models, files, network, or descendants.
const lines = createInterface({ input: process.stdin });
lines.once('line', line => {
  const request = JSON.parse(line);
  if (request.method !== 'initialize' || request.params.protocolVersion !== 1) {
    process.exitCode = 43;
  } else if (process.argv[2] === 'partial') {
    process.stdout.end('{"jsonrpc":');
    process.exitCode = 42;
  } else {
    process.stderr.write('synthetic diagnostic, separate from protocol\n');
    process.stdout.write(JSON.stringify({ jsonrpc: '2.0', id: request.id,
      result: { protocolVersion: 1, agentCapabilities: {}, authMethods: [] } }) + '\n');
    process.stdout.end(JSON.stringify({ jsonrpc: '2.0', method: '_fixture/notice', params: {} }) + '\n');
  }
  lines.close();
  process.stdin.destroy();
});
