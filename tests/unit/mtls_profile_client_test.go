package unit

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/pkg/jira"
)

func TestProfileCarriesMTLSCertificateReferences(t *testing.T) {
	cfg := config.Config{
		DefaultProfile: "dc",
		Profiles: []config.Profile{{
			Name:        "dc",
			BaseURL:     "https://jira.example.com",
			AuthType:    config.AuthTypeMTLS,
			MTLSCertRef: "/secure/client.crt",
			MTLSKeyRef:  "/secure/client.key",
		}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	p := cfg.Profile("dc")
	if p.MTLSCertRef == "" || p.MTLSKeyRef == "" {
		t.Fatalf("mTLS refs were not preserved: %+v", p)
	}
	if redacted := p.Redacted(); redacted == "" || containsAny(redacted, "PRIVATE KEY", "client.key") {
		t.Fatalf("profile redaction leaked mTLS private key reference: %q", redacted)
	}
}

func TestMTLSHTTPClientLoadsCertificatePair(t *testing.T) {
	certPath, keyPath := writeSelfSignedCertPair(t)
	client, err := jira.MTLSHTTPClient(certPath, keyPath, 30*time.Second)
	if err != nil {
		t.Fatalf("MTLSHTTPClient() error = %v", err)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T, want *http.Transport", client.Transport)
	}
	if transport.TLSClientConfig == nil || len(transport.TLSClientConfig.Certificates) != 1 {
		t.Fatalf("TLS client config missing certificate: %+v", transport.TLSClientConfig)
	}
}

func writeSelfSignedCertPair(t *testing.T) (string, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "jira-cli-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate() error = %v", err)
	}
	dir := t.TempDir()
	certPath := filepath.Join(dir, "client.crt")
	keyPath := filepath.Join(dir, "client.key")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatalf("WriteFile(cert) error = %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("WriteFile(key) error = %v", err)
	}
	return certPath, keyPath
}

func containsAny(s string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(s, value) {
			return true
		}
	}
	return false
}
