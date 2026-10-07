// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/jcmturner/gokrb5/v8/client"
	"github.com/jcmturner/gokrb5/v8/config"
	"github.com/jcmturner/gokrb5/v8/credentials"
	"github.com/jcmturner/gokrb5/v8/spnego"
)

// ErrNoTicket reports that Negotiate has no Kerberos credentials to use.
var ErrNoTicket = errors.New("negotiate: no Kerberos ticket (run kinit, or set KRB5CCNAME)")

// Negotiator makes Negotiate (SPNEGO) tokens from the Kerberos credential
// cache (KRB5CCNAME, else the default file cache) and configuration
// (KRB5_CONFIG, else /etc/krb5.conf); service tickets are cached.
type Negotiator struct {
	mu sync.Mutex
	cl *client.Client
}

// NewNegotiator loads the configuration and the credential cache.
func NewNegotiator() (*Negotiator, error) {
	cfgPath := os.Getenv("KRB5_CONFIG")
	if cfgPath == "" {
		cfgPath = "/etc/krb5.conf"
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("negotiate: Kerberos configuration %s: %w", cfgPath, err)
	}
	ccPath, err := ccachePath()
	if err != nil {
		return nil, err
	}
	cc, err := credentials.LoadCCache(ccPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoTicket, err)
	}
	cl, err := client.NewFromCCache(cc, cfg, client.DisablePAFXFAST(true))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoTicket, err)
	}
	return &Negotiator{cl: cl}, nil
}

// Token returns the Authorization value of a request to host; the service
// principal is HTTP/host.
func (n *Negotiator) Token(host string) (string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	req, err := http.NewRequest(http.MethodGet, "http://"+host+"/", nil)
	if err != nil {
		return "", err
	}
	if err := spnego.SetSPNEGOHeader(n.cl, req, "HTTP/"+host); err != nil {
		return "", fmt.Errorf("negotiate: %w", err)
	}
	return req.Header.Get("Authorization"), nil
}

// Close ends the Kerberos client.
func (n *Negotiator) Close() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.cl.Destroy()
}

// ccachePath is the file credential cache: KRB5CCNAME (FILE: caches
// only), else /tmp/krb5cc_<uid>.
func ccachePath() (string, error) {
	if name := os.Getenv("KRB5CCNAME"); name != "" {
		if kind, path, ok := strings.Cut(name, ":"); ok && len(kind) > 1 {
			if !strings.EqualFold(kind, "FILE") {
				return "", fmt.Errorf("negotiate: credential cache %s: only FILE caches are supported", name)
			}
			return path, nil
		}
		return name, nil
	}
	if runtime.GOOS == "windows" {
		return "", ErrNoTicket
	}
	u, err := user.Current()
	if err != nil {
		return "", ErrNoTicket
	}
	return filepath.Join(os.TempDir(), "krb5cc_"+u.Uid), nil
}
