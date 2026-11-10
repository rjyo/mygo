package platform

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"
)

func TestCertificateIssuer(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		issuer   pkix.Name
		from, to time.Time
		want     string
	}{
		{"common name", pkix.Name{CommonName: "mygo test client CA", Organization: []string{"MyGo"}}, time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC), time.Date(2027, 11, 10, 8, 0, 0, 0, time.UTC), "mygo test client CA"},
		{"organization", pkix.Name{Organization: []string{"MyGo"}, Country: []string{"US"}}, time.Date(1999, 1, 2, 3, 4, 5, 0, time.UTC), time.Date(2049, 12, 31, 23, 59, 59, 0, time.UTC), "MyGo"},
		// Years from 2050 are GeneralizedTime.
		{"generalized time", pkix.Name{CommonName: "长期 CA"}, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC), "长期 CA"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmpl := &x509.Certificate{
				SerialNumber: big.NewInt(1234567),
				Subject:      pkix.Name{CommonName: "mygo test client"},
				Issuer:       tc.issuer,
				NotBefore:    tc.from,
				NotAfter:     tc.to,
			}
			parent := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: tc.issuer, NotBefore: tc.from, NotAfter: tc.to}
			der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, &key.PublicKey, key)
			if err != nil {
				t.Fatal(err)
			}
			issuer, from, to, ok := CertificateIssuer(der)
			if !ok || issuer != tc.want || from != tc.from.Unix() || to != tc.to.Unix() {
				t.Errorf("CertificateIssuer = %q, %d, %d, %v; want %q, %d, %d, true", issuer, from, to, ok, tc.want, tc.from.Unix(), tc.to.Unix())
			}
			for n := range der {
				if _, _, _, ok := CertificateIssuer(der[:n]); ok {
					t.Fatalf("CertificateIssuer of %d of %d bytes is ok", n, len(der))
				}
			}
		})
	}
	if _, _, _, ok := CertificateIssuer(nil); ok {
		t.Error("CertificateIssuer(nil) is ok")
	}
}
