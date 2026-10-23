# Threadline Desktop

Tauri 2 host plus a Vite/React renderer. The renderer talks to the host's
window-scoped commands; the host owns the Device's encrypted `locald` store, the
Core client and every secret. Without a configured session the renderer stays on
fixtures, so the shell is usable while Core is offline.

## Everyday commands

```sh
pnpm --filter @threadline/desktop dev     # renderer only, fixtures, http://localhost:1420
pnpm --filter @threadline/desktop test    # vitest (jsdom)
pnpm --filter @threadline/desktop lint    # tsc --noEmit
cargo test -p threadline-desktop-host --lib
```

## Two instances through the dev stack (Slice A)

Everything below is for the private dev stack only. Tokens, keys and the seed
are synthetic; nothing here is a production identity.

1. **Start Postgres and apply migrations**

   ```sh
   make -C deploy/compose up
   ```

2. **Seed one tenant, two Members and one Channel** (idempotent):

   ```sh
   docker compose -p threadline-dev exec -T postgres \
     psql -U threadline_postgres_dev -d threadline < deploy/compose/slice-a-seed.sql
   ```

3. **Run Core with the dev session verifier** (NOT PRODUCTION). Without the
   Outbox policy file `SendEvent` fails closed with `NOT_DURABLE`; the dev
   policy is the frozen v1 default document.

   ```sh
   export THREADLINE_CORE_LISTEN_ADDRESS=127.0.0.1:8080
   export THREADLINE_CORE_POSTGRES_DSN='postgres://threadline_postgres_dev:threadline-postgres-dev-only@127.0.0.1:5432/threadline?sslmode=disable'
   export THREADLINE_OUTBOX_POLICY_FILE="$PWD/deploy/compose/outbox-policy.json"
   export THREADLINE_DEV_SESSIONS_ENABLED=1
   export THREADLINE_DEV_SESSION_FIXTURES="$PWD/deploy/compose/dev-sessions.json"
   (cd services && go run ./core)
   ```

4. **Launch two Desktop instances**, one per Device. Each needs the full set of
   `THREADLINE_DESKTOP_*` variables; a partial set is rejected by name and the
   host falls back to fixtures.

   | Variable | Instance A | Instance B |
   | --- | --- | --- |
   | `THREADLINE_DESKTOP_CORE_URL` | `http://127.0.0.1:8080` | same |
   | `THREADLINE_DESKTOP_DEV_TOKEN` | `dev-session-lin-jia-laptop-0000000001` | `dev-session-meng-qing-laptop-000000001` |
   | `THREADLINE_DESKTOP_TENANT_ID` | `tenant-dev` | same |
   | `THREADLINE_DESKTOP_ACTOR_ID` | `actor-lin-jia` | `actor-meng-qing` |
   | `THREADLINE_DESKTOP_DEVICE_ID` | `device-lin-jia-laptop` | `device-meng-qing-laptop` |
   | `THREADLINE_DESKTOP_DATA_DIR` | `/tmp/threadline-dev/a` | `/tmp/threadline-dev/b` |
   | `THREADLINE_DESKTOP_DB_KEY` | 64 hex chars, not all zero | a different 64 hex chars |

   ```sh
   # terminal A
   THREADLINE_DESKTOP_CORE_URL=http://127.0.0.1:8080 \
   THREADLINE_DESKTOP_DEV_TOKEN=dev-session-lin-jia-laptop-0000000001 \
   THREADLINE_DESKTOP_TENANT_ID=tenant-dev THREADLINE_DESKTOP_ACTOR_ID=actor-lin-jia \
   THREADLINE_DESKTOP_DEVICE_ID=device-lin-jia-laptop THREADLINE_DESKTOP_DATA_DIR=/tmp/threadline-dev/a \
   THREADLINE_DESKTOP_DB_KEY=$(printf 'a1%.0s' $(seq 32)) \
   pnpm --filter @threadline/desktop tauri dev

   # terminal B: the Meng Qing values above, THREADLINE_DESKTOP_DATA_DIR=/tmp/threadline-dev/b and another key
   ```

   The Vite dev server is shared by both windows on port 1420; start terminal B
   with `pnpm --filter @threadline/desktop tauri dev -- --no-dev-server` if the
   port is already taken, or build once and run `tauri build --no-bundle`
   binaries instead.

5. **Enable realtime**: start Worker and Realtime using their service READMEs,
   then set `THREADLINE_DESKTOP_REALTIME_URL=http://127.0.0.1:8082` for both
   Desktop instances. Core commands and Realtime use separate endpoints.
   Without this optional variable, the existing4s polling remains available.

6. **Exercise the loop**: send from A; B wakes through StreamEvents, then syncs
   canonical history into its own encrypted local store. The bounded wait releases
   the store lock; reconnect always uses its persisted applied cursor. Failed or
   idle notifications fall back to Core reconciliation with4s retry backoff.
   Stop Core: sends stay `发送中…` and the offline banner shows. Start Core:
   pending items commit once, in order, with no duplicates. The scripted version
   of this check is A13 (#260).

## What crosses the IPC boundary

Only display DTOs: channel ids and names, message text, author ids, states and
timestamps. The dev token, the database key and raw envelope bytes never leave
the host process.

## Process-boundary acceptance peer

`cargo build -p threadline-desktop-host --example realtime_peer --locked` builds
an opt-in stdin/stdout peer using the exact Desktop Session and encrypted store.
It needs the same explicit Device variables (including Realtime URL). Commands:
`send <synthetic text>`, `sync`, `wait`, `state`, `check <synthetic text>`.
Receipts contain only counts, online/wake flags and applied sequence, no credentials
or message text. This validates host behavior; native window rendering is a
separate UI check. Stop the process before reopening the same Device store.

## V34 messaging surface

The running message page follows `docs/prototype/index.html`: compact toolbar, one resizable/collapsible sidebar, channel search/history and a bottom composer. Work, attention, conversation creation and attachment controls stay unavailable until their backend capabilities are connected. Authorized channel/message content comes from the configured Repository; the UI does not populate prototype conversations or invent member counts.

Browser layout regression (opt-in integration): run `corepack pnpm --filter @threadline/desktop dev --host 127.0.0.1`, open `http://127.0.0.1:1420/test/layout.html`, choose each of `empty`, `short`, `long`, `offline` and click “运行布局验收”. All must report PASS. The legacy-grid checkbox must turn the empty scenario FAIL, proving the original defect is detected. Long history checks both contained overflow and actual independent scrolling. This fixture has no IPC/native credentials, is absent from the production Vite entry and is not part of headless CI; it requires a real browser, not jsdom.
