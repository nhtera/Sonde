// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/value"
)

// TestCaptureValueJSON checks the JSON form of every kind of captured
// value.
func TestCaptureValueJSON(t *testing.T) {
	tests := []struct {
		v    value.Value
		want string
	}{
		{value.Null{}, `null`},
		{value.Bool(true), `true`},
		{value.Int(-3), `-3`},
		{value.BigInt("123456789012345678901234567890"), `123456789012345678901234567890`},
		{value.Float(1.5), `1.5`},
		{value.String("a " + testSecret), `"a ***"`},
		{value.Bytes("hi"), `"aGk="`},
		{value.Date(time.Date(2026, 9, 27, 1, 2, 3, 0, time.UTC)), `"2026-09-27 01:02:03 UTC"`},
		{value.List{value.Int(1), value.String("x")}, `[1,"x"]`},
		{value.Object{{Key: "b", Value: value.Int(1)}, {Key: "a", Value: value.Null{}}}, `{"b":1,"a":null}`},
		{value.Nodeset(2), `{"size":2,"type":"nodeset"}`},
		{value.Unit{}, `{"type":"unit"}`},
		{value.HTTPResponse{Status: 301, Location: "/x", HasLocation: true}, `{"location":"/x","status":301}`},
		{value.HTTPResponse{Status: 200}, `{"location":"None","status":200}`},
	}
	for _, tt := range tests {
		b, err := json.Marshal(toValue(valueOf(tt.v), redactTestSecret))
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != tt.want {
			t.Errorf("%#v: got %s, want %s", tt.v, b, tt.want)
		}
	}
}

// TestCertificateJSON checks the JSON form of a server certificate.
func TestCertificateJSON(t *testing.T) {
	start := time.Date(2025, 12, 12, 5, 18, 48, 0, time.UTC)
	call := engine.Call{Response: &exchange.Response{Status: 200, Version: "HTTP/1.1", Certificate: &exchange.CertInfo{
		Subject: "CN = localhost", Issuer: "CN = myCA", StartDate: start, ExpireDate: start.AddDate(2, 0, 0),
		SerialNumber: "3e:23", SubjectAltName: "DNS:localhost", Value: "-----BEGIN CERTIFICATE-----\n",
	}}}
	c, err := toCall(call, redactNothing, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(c.Response.Certificate)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"expire_date":"2027-12-12 05:18:48 UTC","issuer":"CN = myCA","serial_number":"3e:23","start_date":"2025-12-12 05:18:48 UTC","subject":"CN = localhost","subject_alt_name":"DNS:localhost","value":"-----BEGIN CERTIFICATE-----\n"}`
	if string(b) != want {
		t.Errorf("got %s\nwant %s", b, want)
	}
	if c, _ := toCall(engine.Call{Response: &exchange.Response{Status: 200}}, redactNothing, nil); c.Response.Certificate != nil {
		t.Error("certificate for a plain HTTP response")
	}
}

func redactNothing(s string) string { return s }
