package moxy

import (
	"net"
	"testing"
)

func TestGenerateDefaultCert(t *testing.T) {
	cert := generateDefaultCert()

	if len(cert.Certificate) == 0 {
		t.Fatal("expected certificate chain, got empty")
	}

	if cert.PrivateKey == nil {
		t.Fatal("expected private key, got nil")
	}

	if cert.Leaf == nil {
		t.Fatal("expected Leaf certificate, got nil")
	}
	// Check SANs include 127.0.0.1
	found := false
	for _, ip := range cert.Leaf.IPAddresses {
		if ip.Equal(net.ParseIP("127.0.0.1")) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected IP 127.0.0.1 in certificate SANs")
	}
}

func TestGenerateSelfSignedCert(t *testing.T) {
	commonName := "test.local"
	cert, leaf, err := generateSelfSignedCert(commonName)
	if err != nil {
		t.Fatalf("unexpected error generating certificate: %v", err)
	}

	if len(cert.Certificate) == 0 {
		t.Fatal("expected certificate chain, got empty")
	}

	if cert.PrivateKey == nil {
		t.Fatal("expected private key, got nil")
	}

	found := false
	for _, ip := range leaf.IPAddresses {
		if ip.Equal(net.ParseIP("127.0.0.1")) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected IP 127.0.0.1 in certificate SANs")
	}
}
