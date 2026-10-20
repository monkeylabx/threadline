# R2 pinned Goose source audit

Issue: #258. Reviewed: 2026-10-20. Scope: source and release metadata; runtime results belong to the probe receipt. No real credentials or worker execution were used for this audit.

## Artifact identity

The official `v1.52.0` tag resolves to commit `302b60806639ea9f0ae8f053f49f8bf0e88b26f4`. The release API identifies `goose-aarch64-apple-darwin.tar.gz` as 88,128,790 bytes with SHA-256 `7674b0124aab685c71f8782fb7e65bac100c736ce3de0c9d3bf46ba07910e412`. This pins the archive; the extracted executable needs its own digest. The `.tar.bz2` asset has a different digest and must not be checked against the gzip pin. [Tag API](https://api.github.com/repos/aaif-goose/goose/git/ref/tags/v1.52.0), [release API](https://api.github.com/repos/aaif-goose/goose/releases/tags/v1.52.0).

## Configuration boundary

An absolute `GOOSE_PATH_ROOT` relocates config, data, state and user Agent/plugin directories. It does not disable `/etc/goose/config.yaml`: default config loads system configuration, additional files from `GOOSE_ADDITIONAL_CONFIG_FILES`, then user configuration, merging extension maps. The probe therefore needs a cleared child environment, isolated empty workspace, and OS restrictions on ambient configuration. Set `GOOSE_DISABLE_KEYRING=1` to select isolated file secrets. [Paths](https://github.com/aaif-goose/goose/blob/302b60806639ea9f0ae8f053f49f8bf0e88b26f4/crates/goose/src/config/paths.rs), [configuration loading](https://github.com/aaif-goose/goose/blob/302b60806639ea9f0ae8f053f49f8bf0e88b26f4/crates/goose/src/config/base.rs).

ACP's default builtin selection includes developer unless its configured enabled state is false. Explicit `--with-builtin` selections bypass that check. Session recipes, `_meta.enabledExtensions`, configured extensions, plugin MCP discovery and client MCP declarations are additional loading paths; they are not permanently removed by an empty tool list. The bounded probe must send no recipe or extension metadata, `mcpServers: []`, and use a fresh workspace/root. [Selection and loading](https://github.com/aaif-goose/goose/blob/302b60806639ea9f0ae8f053f49f8bf0e88b26f4/crates/goose/src/acp/server.rs#L557), [new-session metadata](https://github.com/aaif-goose/goose/blob/302b60806639ea9f0ae8f053f49f8bf0e88b26f4/crates/goose/src/acp/server/new_session.rs), [plugin discovery](https://github.com/aaif-goose/goose/blob/302b60806639ea9f0ae8f053f49f8bf0e88b26f4/crates/goose/src/plugins/discovery.rs).

The eight default-enabled platform extensions are analyze, apps, extensionmanager, scheduler, summon, developer, tom and skills. Each disabled entry must deserialize as an extension, including `type: platform`, `name`, and `enabled: false`; malformed entries are skipped. [Platform defaults](https://github.com/aaif-goose/goose/blob/302b60806639ea9f0ae8f053f49f8bf0e88b26f4/crates/goose/src/agents/platform_extensions/mod.rs), [extension configuration parser](https://github.com/aaif-goose/goose/blob/302b60806639ea9f0ae8f053f49f8bf0e88b26f4/crates/goose/src/config/extensions.rs).

Minimal restricted `config/config.yaml` under the isolated root:

```yaml
GOOSE_MODE: approve
GOOSE_DISABLE_KEYRING: true
GOOSE_DISABLE_SESSION_NAMING: true
extensions:
  analyze: {type: platform, name: analyze, enabled: false}
  apps: {type: platform, name: apps, enabled: false}
  extensionmanager: {type: platform, name: extensionmanager, enabled: false}
  scheduler: {type: platform, name: scheduler, enabled: false}
  summon: {type: platform, name: summon, enabled: false}
  developer: {type: platform, name: developer, enabled: false}
  tom: {type: platform, name: tom, enabled: false}
  skills: {type: platform, name: skills, enabled: false}
```

## Synthetic provider and protocol

The OpenAI adapter accepts `OPENAI_HOST`, `OPENAI_BASE_PATH`, `OPENAI_BASE_URL` and `OPENAI_API_KEY`. Provider/model resolution reads `GOOSE_PROVIDER` and `GOOSE_MODEL`; values for the probe must come from its routing policy file. A synthetic model with chat-completions routing avoids the model-based Responses API path. A fixture may need to answer model inventory requests as well as streaming POSTs. [Provider configuration and routing](https://github.com/aaif-goose/goose/blob/302b60806639ea9f0ae8f053f49f8bf0e88b26f4/crates/goose-providers/src/openai.rs), [default model resolution](https://github.com/aaif-goose/goose/blob/302b60806639ea9f0ae8f053f49f8bf0e88b26f4/crates/goose/src/acp/server.rs#L619).

Clearing the child environment does not remove macOS system proxy settings: the HTTP dependency falls back to system proxy discovery when proxy environment variables are absent or empty. Set `NO_PROXY=127.0.0.1,::1,localhost` and the identical `no_proxy` explicitly for this loopback-only synthetic probe. In this pinned matcher, literal IP destinations use only the IP rules, so the domain wildcard `*` alone does not bypass their interception. The sandbox must still restrict direct connections. [Pinned HTTP dependency proxy matcher](https://github.com/hyperium/hyper-util/blob/v0.1.20/src/client/proxy/matcher.rs).

Upstream fixtures use SSE `data: <JSON>` records separated by blank lines: assistant role/content deltas, a finish-reason chunk, optional usage-only chunk, and `data: [DONE]`. Tool calls carry `delta.tool_calls` with index, id, type `function`, function name and JSON-encoded argument fragments; they end with `finish_reason: tool_calls`. The synthetic fixture should return text after the rejected tool response to bound the loop. [Text fixture](https://github.com/aaif-goose/goose/blob/302b60806639ea9f0ae8f053f49f8bf0e88b26f4/crates/goose/tests/acp_test_data/openai_basic.txt), [shell fixture](https://github.com/aaif-goose/goose/blob/302b60806639ea9f0ae8f053f49f8bf0e88b26f4/crates/goose/tests/acp_test_data/openai_shell_tool_call.txt).

Developer exposes the unprefixed tool `shell`; its argument object requires `command` and optionally `timeout_secs`. A forced tool absent from the model-visible registry becomes an invalid request before execution. The rejection contains `was not advertised for this model turn`. Verify both the request's tools and absence of the synthetic command's effect. [Shell schema and execution](https://github.com/aaif-goose/goose/blob/302b60806639ea9f0ae8f053f49f8bf0e88b26f4/crates/goose/src/agents/platform_extensions/developer/shell.rs#L179), [rejection](https://github.com/aaif-goose/goose/blob/302b60806639ea9f0ae8f053f49f8bf0e88b26f4/crates/goose/src/agents/reply_parts.rs#L599).

## ACP compatibility and cleanup limits

After responding to `session/new`, Goose unconditionally sends standard `session/update` notifications for usage and available commands; only custom Goose notifications depend on client metadata. Threadline's current R1 text-only prompt reader rejects these setup updates. A bounded probe observer can record them, but that does not establish compatibility with the production adapter. [Dispatch order](https://github.com/aaif-goose/goose/blob/302b60806639ea9f0ae8f053f49f8bf0e88b26f4/crates/goose/src/acp/server/dispatch.rs#L42), [setup notifications](https://github.com/aaif-goose/goose/blob/302b60806639ea9f0ae8f053f49f8bf0e88b26f4/crates/goose/src/acp/response_builder.rs#L483).

`session/cancel` cancels the active-run token. The developer shell responds by killing and waiting for its direct child; that path does not enumerate or seal descendants. These source paths do not prove background-child cleanup. Real probe observations must distinguish ACP cancellation, direct-child reaping, known-child exit, and the stronger `SealedAndEmpty` contract. [ACP cancellation](https://github.com/aaif-goose/goose/blob/302b60806639ea9f0ae8f053f49f8bf0e88b26f4/crates/goose/src/acp/server.rs#L2411), [shell cancellation](https://github.com/aaif-goose/goose/blob/302b60806639ea9f0ae8f053f49f8bf0e88b26f4/crates/goose/src/agents/platform_extensions/developer/shell.rs#L557), [Threadline containment contract](worker-containment-profile.md).

No source claim here admits a real worker to production, authorizes user files, or establishes protection against malicious runtimes, detached descendants, dynamic extensions, crash recovery or other operating systems.
