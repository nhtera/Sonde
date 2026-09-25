// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package redact

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// benchSource returns a ~64 KiB string of filler text with secrets
// scattered through it, for benchmarking Redact.
func benchSource(secrets []string) string {
	var b strings.Builder
	rng := rand.New(rand.NewSource(1)) //nolint:gosec // G404: deterministic bench-data generator, not security-sensitive
	const filler = "the quick brown fox jumps over the lazy dog "
	for b.Len() < 64<<10 {
		b.WriteString(filler)
		if rng.Intn(50) == 0 {
			b.WriteString(secrets[rng.Intn(len(secrets))])
			b.WriteByte(' ')
		}
	}
	return b.String()
}

func BenchmarkRedact(b *testing.B) {
	r := New()
	secrets := make([]string, 20)
	for i := range secrets {
		secrets[i] = fmt.Sprintf("secret-value-%02d-xyz", i)
		r.Add("s", secrets[i])
	}
	src := benchSource(secrets)

	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	for b.Loop() {
		_ = r.Redact(src)
	}
}
