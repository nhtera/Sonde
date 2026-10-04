# Update signing keys

The public keys the app pins: `<id>.pub`, base64 of an ed25519 key, made
offline with `update-manifest genkey` (docs/release.md › Update signing
keys). `k1` is the active key, `k2` the offline standby. Private keys
(`*.key`) never belong here.
