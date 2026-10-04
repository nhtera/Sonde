// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"crypto/ed25519"
	"encoding/base64"
	"testing"
)

func TestTestKeyOnlyInDevBuilds(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	old := testKey
	t.Cleanup(func() { testKey = old })
	testKey = base64.StdEncoding.EncodeToString(pub)

	for _, v := range []string{"0.2.0-dev", "0.2.0-dev.3"} {
		keys, err := pinnedKeys(v)
		if err != nil || len(keys) != 1 || !keys["test"].Equal(pub) {
			t.Errorf("%s: keys %v, %v; want the test key alone", v, keys, err)
		}
	}
	for _, v := range []string{"0.2.0", "0.2.0-rc.1", "0.2.0-devx", "dev"} {
		if keys, _ := pinnedKeys(v); keys["test"] != nil {
			t.Errorf("%s: a release build took the test key", v)
		}
	}
}
