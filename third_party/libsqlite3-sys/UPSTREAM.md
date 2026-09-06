# Threadline libsqlite3-sys source override

This directory is a temporary, audited source override for
`libsqlite3-sys` 0.38.2. Its Rust wrapper and build files come from the exact
`libsqlite3-sys` 0.38.2 crate published on crates.io. Only the bundled
`sqlcipher/` amalgamation was regenerated from SQLCipher Community 4.18.0.

The override is required because the published crate bundles SQLCipher 4.14.0,
while SQLCipher 4.18.0 fixes a Windows recursive logging crash when enhanced
memory security is enabled. Remove this directory and the root Cargo patch once
a reviewed `libsqlite3-sys` release bundles SQLCipher 4.18.0 or newer and passes
Threadline's complete platform matrix.

## Upstream source

- Project: <https://github.com/sqlcipher/sqlcipher>
- Tag: `v4.18.0`
- Tag object: `dca3c1ee114fe6bf5d996fc71f3c5380f43cc82c`
- Commit: `63697beb0fafcb61faa7a3e6fd267036548ab11b`
- Source archive:
  `https://github.com/sqlcipher/sqlcipher/archive/refs/tags/v4.18.0.tar.gz`
- Source archive SHA-256:
  `1df02d1b346fa27feaf2da2cb2c0d8209e788248e461ec288718aa5d3e9643e5`
- Release note:
  <https://www.zetetic.net/blog/2026/08/18/sqlcipher-4.18.0-release/>

The amalgamation was generated from that archive with:

```text
make -f Makefile.linux-generic sqlite3.c
```

Generated file SHA-256 values:

```text
964c72bd1d3e031862588202e2bf6342d36ec68a2cae4f4276d9cd79e6571acb  sqlcipher/sqlite3.c
8a9d1bff44d75174ca6dea3ea9bac50a6104d86facb566647b8bb839375b7b3a  sqlcipher/sqlite3.h
ac9645e5c9ff0cf176efdd6e75cb5e98f46295d38e02db5c4d208826a39ab4be  sqlcipher/sqlite3ext.h
```

SQLCipher's BSD-3-Clause license is retained in `sqlcipher/LICENSE`.
