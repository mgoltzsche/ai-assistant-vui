package tlsutils

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

func GenerateSelfSignedTLSCertificate() (string, string, func(), error) {
	certPEM, keyPEM, err := generateSelfSignedTLSCertificate()
	if err != nil {
		return "", "", func() {}, err
	}

	dir, err := os.MkdirTemp("", "ai-assistant-vui-tls-")
	if err != nil {
		return "", "", func() {}, err
	}

	cleanup := func() {
		_ = os.RemoveAll(dir)
	}

	certFile := filepath.Join(dir, "cert.tls")
	keyFile := filepath.Join(dir, "key.tls")

	err = os.WriteFile(certFile, certPEM, 0o600)
	if err != nil {
		cleanup()
		return "", "", func() {}, err
	}

	err = os.WriteFile(keyFile, keyPEM, 0o600)
	if err != nil {
		cleanup()
		return "", "", func() {}, err
	}

	return certFile, keyFile, cleanup, nil
}

func generateSelfSignedTLSCertificate() ([]byte, []byte, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate ECDSA key: %v", err)
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(30 * 24 * time.Hour)

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("generate serial number: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   "localhost",
			Organization: []string{"Fake Org"},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	privBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal ECDSA private key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privBytes})

	return certPEM, keyPEM, nil
}
