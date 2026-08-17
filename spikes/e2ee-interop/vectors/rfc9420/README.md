# RFC 9420 test vectors

Upstream Known Answer Test vectors from the IETF MLS working group, used by
`../../rust/tests/rfc9420_kat.rs`.

- Source: <https://github.com/mlswg/mls-implementations>, `test-vectors/`, `main`
  branch as fetched on 2026-08-18.
- Filter: entries are kept only where `cipher_suite == 1`
  (`MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519`), the single cipher suite that
  `tl-mls-1` fixes. Nothing else is edited; the remaining objects are byte-identical
  to upstream, re-serialized as pretty-printed JSON.

| file | upstream file | upstream SHA-256 | entries kept |
| --- | --- | --- | --- |
| `crypto-basics.cs1.json` | `crypto-basics.json` | `17cfcf89af9f51d0f2aa7af77f6f9ec99376a039214b6d42a6f11646b83e8c29` | 1 of 7 |
| `key-schedule.cs1.json` | `key-schedule.json` | `05aa9a68bd2538ace72d8c53375984cc728ef62220ebf314df675708546d97a7` | 1 of 7 |
| `psk_secret.cs1.json` | `psk_secret.json` | `2b534969dba0b65a04b7d790496af5c0ccdb472b3fc4ca25c8c82df3e8523784` | 11 of 77 |

These are public IETF test vectors. They contain no Threadline key material and
no message content.

## Why these three

They are the vectors that can be verified through a public crypto-provider API
without reaching into library internals, which is what makes them a test of the
provider rather than a test of OpenMLS against itself. The harness re-implements
`RefHash`, `ExpandWithLabel`, `DeriveSecret`, `DeriveTreeSecret`, `SignWithLabel`,
`EncryptWithLabel`, the RFC 9420 section 8 key schedule, the `GroupContext`
encoding and the PSK chain directly from the RFC.

Vectors still not covered, and the reason:

- `transcript-hashes`, `welcome`, `messages`, `tree-*`, `treekem`,
  `passive-client-*`: these need to inject private key material into group state
  or to drive library internals, which OpenMLS only exposes under its
  `test-utils` feature. Covering them means either running them inside the
  upstream test suite or building the encrypted `StorageProvider` that
  `client-crypto` owns in P05. Tracked as remaining P00-08 work in
  `docs/spikes/e2ee-library-selection.md`.
