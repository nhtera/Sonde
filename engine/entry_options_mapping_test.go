// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"reflect"
	"testing"
)

// httpOptionFields maps each HTTPOptions field to the httpx.Options field
// baseHTTPOptions copies it to.
var httpOptionFields = map[string]string{
	"AWSSigV4": "AWSSigV4", "CACert": "CACert", "ClientCert": "ClientCert", "ClientKey": "ClientKey",
	"Compressed": "Compressed", "ConnectTimeout": "ConnectTimeout", "ConnectTo": "ConnectTo",
	"Digest": "Digest", "FollowLocation": "FollowLocation", "LocationTrusted": "LocationTrusted",
	"Headers": "Headers", "HTTPVersion": "HTTPVersion", "Insecure": "Insecure", "IPResolve": "IPResolve",
	"MaxFilesize": "MaxFilesize", "LimitRate": "MaxRecvSpeed", "MaxRedirects": "MaxRedirects",
	"Negotiate": "Negotiate", "Netrc": "Netrc", "NetrcFile": "NetrcFile", "NetrcOptional": "NetrcOptional",
	"NetrcAllowReroute": "NetrcAllowReroute", "NoHeaders": "NoHeaders", "NoProxy": "NoProxy", "NTLM": "NTLM",
	"PathAsIs": "PathAsIs", "PinnedPublicKey": "PinnedPublicKey", "Proxy": "Proxy",
	"ProxyHeaders": "ProxyHeaders", "Resolve": "Resolve", "Timeout": "Timeout", "UnixSocket": "UnixSocket",
	"User": "User", "UserAgent": "UserAgent",
}

// TestHTTPOptionsMapped fails when an HTTPOptions field is not copied to
// the transport options: every field is set, and the httpx field it maps
// to must be set too.
func TestHTTPOptionsMapped(t *testing.T) {
	var h HTTPOptions
	v := reflect.ValueOf(&h).Elem()
	for i := range v.NumField() {
		f := v.Field(i)
		switch f.Kind() {
		case reflect.String:
			f.SetString("x")
		case reflect.Bool:
			f.SetBool(true)
		case reflect.Int, reflect.Int64:
			f.SetInt(3)
		case reflect.Slice:
			f.Set(reflect.ValueOf([]string{"A: b"}))
		default:
			t.Fatalf("field %s: kind %s not handled by the test", v.Type().Field(i).Name, f.Kind())
		}
	}
	o := reflect.ValueOf((&Runner{opt: Options{HTTP: h}}).baseHTTPOptions())
	for i := range v.NumField() {
		name := v.Type().Field(i).Name
		target, ok := httpOptionFields[name]
		if !ok {
			t.Errorf("HTTPOptions.%s is not mapped (add it to baseHTTPOptions and httpOptionFields)", name)
			continue
		}
		if o.FieldByName(target).IsZero() {
			t.Errorf("HTTPOptions.%s does not reach httpx.Options.%s", name, target)
		}
	}
}
