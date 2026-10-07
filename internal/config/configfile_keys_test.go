// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/nhtera/sonde/internal/value"
)

// TestParseFileEveryOption sets each option the config file accepts and
// checks the typed value it gives.
func TestParseFileEveryOption(t *testing.T) {
	for _, tt := range everyOptionCases {
		t.Run(tt.content, func(t *testing.T) {
			got, err := ParseFile(tt.content)
			if err != nil {
				t.Fatal(err)
			}
			got.Keys = nil
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseFile(%q) =\n%+v\nwant\n%+v", tt.content, got, tt.want)
			}
		})
	}
}

func ms(n int) *time.Duration { d := time.Duration(n) * time.Millisecond; return &d }

// everyOptionCases set each option the config file accepts, and give the
// typed value it parses to.
var everyOptionCases = []struct {
	content string
	want    FileOptions
}{
	{"--color\n", FileOptions{Color: ptr(true)}},
	{"--compressed\n", FileOptions{Compressed: true}},
	{"--connect-timeout=500ms\n", FileOptions{ConnectTimeout: ms(500)}},
	{"--connect-timeout 2\n", FileOptions{ConnectTimeout: ms(2000)}},
	{"--continue-on-error\n", FileOptions{ContinueOnError: true}},
	{"--delay=1s\n", FileOptions{Delay: ms(1000)}},
	{"--delay=250\n", FileOptions{Delay: ms(250)}},
	{"--error-format=long\n", FileOptions{ErrorFormat: "long"}},
	{"--fail-with-body\n", FileOptions{FailWithBody: true}},
	{"--header=header1:value1\n", FileOptions{Headers: []string{"header1:value1"}}},
	{"--http1.0\n", FileOptions{HTTPVersion: "1.0"}},
	{"--http1.1\n", FileOptions{HTTPVersion: "1.1"}},
	{"--http2\n", FileOptions{HTTPVersion: "2"}},
	{"--http3\n", FileOptions{HTTPVersion: "3"}},
	{"--insecure\n", FileOptions{Insecure: true}},
	{"--ipv6\n", FileOptions{IPv6: true}},
	{"--jobs=4\n", FileOptions{Jobs: ptr(4)}},
	{"--limit-rate=2000000\n", FileOptions{LimitRate: ptr(int64(2000000))}},
	{"--location\n", FileOptions{Location: true}},
	{"--location-trusted\n", FileOptions{Location: true, LocationTrusted: true}},
	{"--max-filesize=255\n", FileOptions{MaxFilesize: ptr(int64(255))}},
	{"--max-redirs=-1\n", FileOptions{MaxRedirs: ptr(-1)}},
	{"--max-time=30s\n", FileOptions{MaxTime: ms(30000)}},
	{"--no-assert\n", FileOptions{NoAssert: true}},
	{"--no-color\n", FileOptions{Color: ptr(false)}},
	{"--no-cookie-store\n", FileOptions{NoCookieStore: true}},
	{"--no-header=user-agent\n", FileOptions{NoHeaders: []string{"user-agent"}}},
	{"--no-jsonpath-coercion\n", FileOptions{NoJSONPathCoercion: true}},
	{"--no-output\n", FileOptions{NoOutput: true}},
	{"--no-pretty\n", FileOptions{Pretty: ptr(false)}},
	{"--no-progress-bar\n", FileOptions{NoProgressBar: true}},
	{"--no-proxy=127.0.0.1\n", FileOptions{NoProxy: ptr("127.0.0.1")}},
	{"--pretty\n", FileOptions{Pretty: ptr(true)}},
	{"--proxy=127.0.0.1:3128\n", FileOptions{Proxy: ptr("127.0.0.1:3128")}},
	{"--proxy-header=From-Proxy:Hello\n", FileOptions{ProxyHeaders: []string{"From-Proxy:Hello"}}},
	{"--retry=10\n", FileOptions{Retry: ptr(10)}},
	{"--retry-interval=100ms\n", FileOptions{RetryInterval: ms(100)}},
	{"--secret=password=secret\n", FileOptions{Secrets: map[string]string{"password": "secret"}}},
	{"--test\n", FileOptions{Test: true}},
	{"--user=bob@email.com:secret\n", FileOptions{User: ptr("bob@email.com:secret")}},
	{"--user-agent=\"Mozilla/5.0 A\"", FileOptions{UserAgent: ptr("Mozilla/5.0 A")}},
	{"--user-agent=\n", FileOptions{UserAgent: ptr("")}},
	{"--variable=hobby=tennis\n", FileOptions{Variables: []Assignment{{Name: "hobby", Value: value.String("tennis")}}}},
	{"--verbose\n", FileOptions{Verbosity: "verbose"}},
	{"--very-verbose\n", FileOptions{Verbosity: "debug"}},
	{"--verbosity=brief\n", FileOptions{Verbosity: "brief"}},
	// The last of two exclusive options wins.
	{"--color\n--no-color\n", FileOptions{Color: ptr(false)}},
	{"--http2\n--http1.1\n", FileOptions{HTTPVersion: "1.1"}},
}

// TestParseFileCoversEveryOption keeps everyOptionCases in step with the
// option table, which has the reference's 43 options.
func TestParseFileCoversEveryOption(t *testing.T) {
	if got := len(configOptions); got != 43 {
		t.Errorf("%d options, want the reference's 43", got)
	}
	for name := range configOptions {
		covered := false
		for _, tt := range everyOptionCases {
			opts, err := ParseFile(tt.content)
			covered = covered || (err == nil && opts.Sets(name))
		}
		if !covered {
			t.Errorf("--%s has no case in everyOptionCases", name)
		}
	}
}

func TestParseFileKeys(t *testing.T) {
	opts, err := ParseFile("--header a:b\n--verbose\n--header c:d\n--http1.1\n")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"header", "verbose", "http1.1"}; !slices.Equal(opts.Keys, want) {
		t.Errorf("Keys = %v, want %v", opts.Keys, want)
	}
}

// TestParseFileErrorPositions ports the upstream parser's error cases:
// each message, and the line:col it is reported at.
func TestParseFileErrorPositions(t *testing.T) {
	tests := []struct{ content, want string }{
		{"verbose\n", "1:1: Expecting an option starting with --"},
		{"--xxx", "1:1: Unknown option <--xxx>"},
		{"--verbosexxx", "1:1: Unknown option <--verbosexxx>"},
		{"# the first 2 lines are ignored\n\n  --unknown\n", "3:3: Unknown option <--unknown>"},
		{"--header\n", "1:9: Expecting a value using space or '=' separator"},
		{"# comment\n--header\n\n", "2:9: Expecting a value using space or '=' separator"},
		{"--verbose=1\n", "1:10: Not expecting a value for this option"},
		{"# comment\n--verbose=1\n", "2:10: Not expecting a value for this option"},
		{"--header=\n", "1:1: Option --header requires a value"},
		{"--no-header=\n", "1:1: Option --no-header requires a value"},
		{"--no-proxy=\n", "1:1: Option --no-proxy requires a value"},
		{"--proxy=\n", "1:1: Option --proxy requires a value"},
		{"--proxy-header=\n", "1:1: Option --proxy-header requires a value"},
		{"--user=\n", "1:1: Option --user requires a value"},
		{"--max-redirs=a\n", "1:14: Option --max-redirs requires an integer value"},
		{"--max-redirs=-2\n", "1:14: Option --max-redirs requires an integer value >= -1"},
		{"--delay=abc\n", "1:9: Option --delay has an invalid duration"},
		{"--connect-timeout=x\n", "1:19: Option --connect-timeout has an invalid duration"},
		{"--max-time=abc\n", "1:12: Option --max-time has an invalid duration"},
		{"--retry-interval=abc\n", "1:18: Option --retry-interval has an invalid duration"},
		{"--limit-rate=abc\n", "1:14: Option --limit-rate requires an integer value"},
		{"--max-filesize=abc\n", "1:16: Option --max-filesize requires an integer value"},
		{"--retry=abc\n", "1:9: Option --retry requires an integer value"},
		{"--retry=-2\n", "1:9: Option --retry requires an integer value >= 1"},
		{"--jobs=abc\n", "1:8: Option --jobs requires an integer value"},
		{"--jobs=0\n", "1:8: Option --jobs requires an integer value >= 1"},
		{"--jobs=-1\n", "1:8: Option --jobs requires an integer value"},
		{"--error-format=foo\n", "1:16: Option --error-format requires one of the following values: short, long"},
		{"--verbosity=invalid\n", "1:13: Option --verbosity requires one of the following values: brief, verbose, debug"},
		{"--user-agent=\"Mozilla/5.0 A\" --verbose\n", "1:30: characters after the closing quote"},
		{"--user-agent=\"1\n", "2:1: Missing closing quote"},
		{"--secret=a=1\n--secret=a=2\n", "2:10: secret 'a' can't be reassigned"},
		{"--variable=hobby\n", "1:12: Missing value for variable hobby!"},
		// The text of a secret line is never echoed back.
		{"--secret=tok3n-value\n", "1:10: invalid secret assignment: missing '='"},
		// A combining mark takes no column.
		{"--user-agent=\"cafe\u0301\" x\n", "1:21: characters after the closing quote"},
	}
	for _, tt := range tests {
		t.Run(tt.content, func(t *testing.T) {
			_, err := ParseFile(tt.content)
			if err == nil || err.Error() != tt.want {
				t.Errorf("ParseFile(%q) error = %v, want %q", tt.content, err, tt.want)
			}
		})
	}
}

func TestParseFileValues(t *testing.T) {
	tests := []struct{ content, want string }{
		// Quoted values may span lines and are trimmed inside the quotes;
		// a blank or a comment may follow the closing quote.
		{"--user-agent=\"Hello\nBob!\"\n", "Hello\nBob!"},
		{"--user-agent=\" Hello \"   \n", "Hello"},
		{"--user-agent=\"Hello\" # a comment\n", "Hello"},
		{"--user-agent   spaced value  \n", "spaced value"},
		{"--user-agent=--test:1\n", "--test:1"},
	}
	for _, tt := range tests {
		opts, err := ParseFile(tt.content)
		if err != nil {
			t.Fatalf("ParseFile(%q): %v", tt.content, err)
		}
		if opts.UserAgent == nil || *opts.UserAgent != tt.want {
			t.Errorf("ParseFile(%q) user-agent = %v, want %q", tt.content, opts.UserAgent, tt.want)
		}
	}
	// A leading BOM and CRLF line ends are accepted.
	opts, err := ParseFile("\uFEFF--insecure\r\n--user-agent \"x\"\r\n--header a:b\r\n--compressed\r\n")
	if err != nil || !opts.Insecure || !opts.Compressed || *opts.UserAgent != "x" || opts.Headers[0] != "a:b" {
		t.Errorf("BOM and CRLF = %+v, %v", opts, err)
	}
	// A lone CR is still a stray character.
	if _, err := ParseFile("--insecure\r--compressed\n"); err == nil || err.Error() != "1:11: Not expecting a value for this option" {
		t.Errorf("lone CR: %v", err)
	}
	// A flag may be followed by a comment, and the option after it is
	// still read.
	opts, err = ParseFile("--insecure # trust any certificate\n--compressed\n")
	if err != nil || !opts.Insecure || !opts.Compressed {
		t.Errorf("flag with a comment = %+v, %v", opts, err)
	}
}
