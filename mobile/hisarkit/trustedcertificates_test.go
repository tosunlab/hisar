package hisarkit

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
)

type testAuthority struct {
	certificate *x509.Certificate
	key         *ecdsa.PrivateKey
}

func createTestAuthority(t *testing.T, name string) testAuthority {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: name}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return testAuthority{certificate: certificate, key: key}
}

func (authority testAuthority) encodePEM() string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: authority.certificate.Raw}))
}

func (authority testAuthority) issueLoopbackCertificate(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "hisar.home"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, authority.certificate, &key.PublicKey, authority.key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// TestTrustedCertificatesReachTheServer runs a self-hosted server whose certificate a private, user-installed CA issued.
func TestTrustedCertificatesReachTheServer(t *testing.T) {
	privateAuthority, otherAuthority := createTestAuthority(t, "Home CA"), createTestAuthority(t, "Another CA")
	var requests atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		writer.WriteHeader(http.StatusUpgradeRequired)
		json.NewEncoder(writer).Encode(apitypes.UpgradeRequired{Error: "update required", MinimumVersion: "0.2.0"})
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{privateAuthority.issueLoopbackCertificate(t)}}
	server.StartTLS()
	t.Cleanup(server.Close)
	stateDirectory := t.TempDir()
	os.WriteFile(filepath.Join(stateDirectory, "hisar.json"), []byte(`{"server":"`+server.URL+`"}`), 0o600)
	hisar, err := New(stateDirectory, "Test phone", createMemorySecretKeyStore(), nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	hisar.SetAppVersion("0.1.0")
	isServerReached := func() bool {
		var status struct {
			UpdateRequired *struct{} `json:"updateRequired"`
		}
		if err := json.Unmarshal([]byte(hisar.CheckMinimumVersion()), &status); err != nil {
			t.Fatal(err)
		}
		return status.UpdateRequired != nil
	}

	if isServerReached() || requests.Load() != 0 {
		t.Fatal("reached the server before its CA was trusted")
	}
	if err := hisar.SetTrustedCertificates(otherAuthority.encodePEM()); err != nil {
		t.Fatal(err)
	}
	if isServerReached() || requests.Load() != 0 {
		t.Fatal("reached the server with only another CA trusted")
	}
	if err := hisar.SetTrustedCertificates(otherAuthority.encodePEM() + privateAuthority.encodePEM()); err != nil {
		t.Fatal(err)
	}
	if !isServerReached() || requests.Load() != 1 {
		t.Fatalf("the server under a trusted CA was not reached (%d requests)", requests.Load())
	}
	if err := hisar.SetTrustedCertificates("not a certificate"); err == nil {
		t.Error("text without a certificate was accepted")
	}
}
