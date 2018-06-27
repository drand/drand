package net

import (
	"crypto/x509"
	"fmt"
	"io/ioutil"

	"github.com/nikkolasg/slog"
)

// CertManager is used to managed certificates. It is most commonly used for
// testing with self signed certificate. By default, it returns the bundled set
// of certificates coming with the OS (Go's implementation).
type CertManager struct {
	pool *x509.CertPool
}

func NewCertManager() *CertManager {
	pool, err := x509.SystemCertPool()
	if err != nil {
		panic(err)
	}
	return &CertManager{pool}
}

func (p *CertManager) Pool() *x509.CertPool {
	return p.pool
}

func (p *CertManager) Add(certPath string) error {
	b, err := ioutil.ReadFile(certPath)
	if err != nil {
		return err
	}
	if !p.pool.AppendCertsFromPEM(b) {
		return fmt.Errorf("peer cert: failed to append certificate %s", certPath)
	}
	slog.Info("peer cert: storing server certificate ", certPath)
	return nil
}
