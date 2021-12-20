/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package testca

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

type CA struct {
	KeyPath  string
	CertPath string

	t      testing.TB
	key    *rsa.PrivateKey
	cert   *x509.Certificate
	closeF func() error
}

func (c *CA) Close() error {
	return c.closeF()
}

const keyLength = 4096

func New(t testing.TB) *CA {
	key, err := rsa.GenerateKey(rand.Reader, keyLength)
	assert.NilError(t, err)

	cert := &x509.Certificate{
		SerialNumber: serialNumber(t),
		Subject: pkix.Name{
			Organization: []string{"nerdctl test organization"},
			CommonName:   fmt.Sprintf("nerdctl CA (%s)", t.Name()),
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	dir, err := os.MkdirTemp(t.TempDir(), "ca")
	assert.NilError(t, err)
	keyPath := filepath.Join(dir, "ca.key")
	certPath := filepath.Join(dir, "ca.cert")
	writePair(t, keyPath, certPath, cert, cert, key, key)

	return &CA{
		KeyPath:  keyPath,
		CertPath: certPath,
		t:        t,
		key:      key,
		cert:     cert,
		closeF: func() error {
			return os.RemoveAll(dir)
		},
	}
}

type Cert struct {
	KeyPath  string
	CertPath string
	closeF   func() error
}

func (c *Cert) Close() error {
	return c.closeF()
}

func (ca *CA) NewCert(host string) *Cert {
	t := ca.t

	key, err := rsa.GenerateKey(rand.Reader, keyLength)
	assert.NilError(t, err)

	cert := &x509.Certificate{
		SerialNumber: serialNumber(t),
		Subject: pkix.Name{
			Organization: []string{"nerdctl test organization"},
			CommonName:   fmt.Sprintf("nerdctl %s (%s)", host, t.Name()),
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageCRLSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    []string{host},
	}
	if ip := net.ParseIP(host); ip != nil {
		cert.IPAddresses = append(cert.IPAddresses, ip)
	}

	dir, err := os.MkdirTemp(t.TempDir(), "cert")
	assert.NilError(t, err)
	certPath := filepath.Join(dir, "a.cert")
	keyPath := filepath.Join(dir, "a.key")
	writePair(t, keyPath, certPath, cert, ca.cert, key, ca.key)

	return &Cert{
		CertPath: certPath,
		KeyPath:  keyPath,
		closeF: func() error {
			return os.RemoveAll(dir)
		},
	}
}

func writePair(t testing.TB, keyPath, certPath string, cert, caCert *x509.Certificate, key, caKey *rsa.PrivateKey) {
	keyF, err := os.Create(keyPath)
	assert.NilError(t, err)
	defer keyF.Close()
	assert.NilError(t, pem.Encode(keyF, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	assert.NilError(t, keyF.Close())

	certB, err := x509.CreateCertificate(rand.Reader, cert, caCert, &key.PublicKey, caKey)
	assert.NilError(t, err)
	certF, err := os.Create(certPath)
	assert.NilError(t, err)
	defer certF.Close()
	assert.NilError(t, pem.Encode(certF, &pem.Block{Type: "CERTIFICATE", Bytes: certB}))
	assert.NilError(t, certF.Close())
}

func serialNumber(t testing.TB) *big.Int {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 60)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	assert.NilError(t, err)
	return serialNumber
}
