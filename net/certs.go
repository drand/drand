package net

import (
	"crypto/x509"
	"fmt"
	"io/ioutil"

	"github.com/drand/drand/log"
)

// CertManager is used to managed certificates. It is most commonly used for
// testing with self signed certificate. By default, it returns the bundled set
// of certificates coming with the OS (Go's implementation).
type CertManager struct {
	pool *x509.CertPool
}

// NewCertManager returns a cert manager filled with the trusted certificates of
// the running system
func NewCertManager() *CertManager {
	pool, err := x509.SystemCertPool()
	if err != nil {
		panic(err)
	}
	return &CertManager{pool}
}

// Pool returns the pool of trusted certificates
func (p *CertManager) Pool() *x509.CertPool {
	return p.pool
}

// Add tries to add the certificate at the given path to the pool and returns an
// error otherwise
func (p *CertManager) Add(certPath string) error {
	b, err := ioutil.ReadFile(certPath)
	if err != nil {
		return err
	}
	if !p.pool.AppendCertsFromPEM(b) {
		return fmt.Errorf("peer cert: failed to append certificate %s", certPath)
	}
	log.DefaultLogger().Debug("cert_manager", "add", "server cert path", certPath)
	return nil
}
