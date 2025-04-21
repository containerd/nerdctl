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

// Package testca provides helpers to create a self-signed CA certificate, and the ability to generate
// signed certificates from it.
// PLEASE NOTE THIS IS NOT A PRODUCTION SAFE NOR VERIFIED WAY TO MANAGE CERTIFICATES FOR SERVERS.
package testca

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"time"

	"github.com/containerd/nerdctl/mod/tigron/internal/assertive"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"
)

const (
	keyLength    = 4096
	caRoot       = "ca"
	certsRoot    = "certs"
	organization = "tigron volatile testing organization"
	lifetime     = 24 * time.Hour
	serialSize   = 60
)

// NewX509 creates a new, self-signed, signing certificate under data.Temp()/ca
// From that Cert as a CA, you can then generate signed certificates.
// Note that the common name of the cert will be set to the test name.
func NewX509(data test.Data, helpers test.Helpers) *Cert {
	template := &x509.Certificate{
		Subject: pkix.Name{
			Organization: []string{organization},
			CommonName:   helpers.T().Name(),
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(lifetime),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	return (&Cert{}).GenerateCustomX509(data, helpers, caRoot, template)
}

// Cert allows the consumer to retrieve the cert and key path, to be used by other processes, like servers for example.
type Cert struct {
	KeyPath  string
	CertPath string
	key      *rsa.PrivateKey
	cert     *x509.Certificate
}

// GenerateServerX509 produces a certificate usable by a server.
// additional can be used to provide additional ips to be added to the certificate.
func (ca *Cert) GenerateServerX509(data test.Data, helpers test.Helpers, host string, additional ...string) *Cert {
	template := &x509.Certificate{
		Subject: pkix.Name{
			Organization: []string{organization},
			CommonName:   host,
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(lifetime),
		KeyUsage:    x509.KeyUsageCRLSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    additional,
	}

	additional = append([]string{host}, additional...)
	for _, h := range additional {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		}
	}

	return ca.GenerateCustomX509(data, helpers, certsRoot, template)
}

// GenerateCustomX509 signs a random x509 certificate template.
// Note that if SerialNumber is specified, it must be safe to use on the filesystem as this will be used in the name
// of the certificate file.
func (ca *Cert) GenerateCustomX509(
	data test.Data,
	helpers test.Helpers,
	underDirectory string,
	template *x509.Certificate,
) *Cert {
	silentT := assertive.WithSilentSuccess(helpers.T())
	key, certPath, keyPath := createCert(silentT, data, underDirectory, template, ca.cert, ca.key)

	return &Cert{
		CertPath: certPath,
		KeyPath:  keyPath,
		key:      key,
		cert:     template,
	}
}

func createCert(
	testing tig.T,
	data test.Data,
	dir string,
	template, caCert *x509.Certificate,
	caKey *rsa.PrivateKey,
) (key *rsa.PrivateKey, certPath, keyPath string) {
	if caCert == nil {
		caCert = template
	}

	if caKey == nil {
		caKey = key
	}

	key, err := rsa.GenerateKey(rand.Reader, keyLength)
	assertive.ErrorIsNil(testing, err, "key generation should succeed")

	signedCert, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	assertive.ErrorIsNil(testing, err, "certificate creation should succeed")

	serial := template.SerialNumber
	if serial == nil {
		serial = serialNumber()
	}

	data.Temp().Dir(dir)
	certPath = data.Temp().Path(dir, serial.String()+".cert")
	keyPath = data.Temp().Path(dir, serial.String()+".key")

	data.Temp().SaveToWriter(func(writer io.Writer) error {
		return pem.Encode(writer, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	}, keyPath)

	data.Temp().SaveToWriter(func(writer io.Writer) error {
		return pem.Encode(writer, &pem.Block{Type: "CERTIFICATE", Bytes: signedCert})
	}, keyPath)

	return key, certPath, keyPath
}

func serialNumber() *big.Int {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), serialSize)

	serial, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		panic(err)
	}

	return serial
}
