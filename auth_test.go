//go:build !js

package mygo

import (
	"bytes"
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

func TestAuthHandler(t *testing.T) {
	w, fw := testWindow(t, WindowOptions{})
	ask := func(req platform.AuthRequest) platform.AuthResponse {
		return onMainValue(func() platform.AuthResponse { return fw.H.AuthRequested(req) })
	}
	challenge := platform.AuthRequest{Host: "router.lan", Port: 80, Realm: "admin", Scheme: "basic"}
	// Without a handler the web view decides, as before.
	if got := ask(challenge); got != (platform.AuthResponse{Default: true}) {
		t.Errorf("default answer = %+v", got)
	}
	var got AuthRequest
	w.Page().SetAuthHandler(func(req AuthRequest) AuthResponse {
		got = req
		if req.Host == "router.lan" && !req.IsProxy {
			return AuthResponse{Username: "admin", Password: "hunter2"}
		}
		return AuthResponse{}
	})
	if r := ask(challenge); r != (platform.AuthResponse{Username: "admin", Password: "hunter2"}) {
		t.Errorf("answer = %+v", r)
	}
	if got != (AuthRequest{Host: "router.lan", Port: 80, Realm: "admin", Scheme: "basic"}) {
		t.Errorf("request = %+v", got)
	}
	// An empty response cancels: no credentials, and not the default.
	proxy := platform.AuthRequest{Host: "proxy.lan", Port: 3128, Scheme: "digest", IsProxy: true}
	if r := ask(proxy); r != (platform.AuthResponse{}) {
		t.Errorf("canceled answer = %+v", r)
	}
	if !got.IsProxy || got.Scheme != "digest" || got.Port != 3128 {
		t.Errorf("proxy request = %+v", got)
	}
	w.Page().SetAuthHandler(nil)
	if r := ask(challenge); !r.Default {
		t.Error("SetAuthHandler(nil) did not restore the default")
	}
}

func TestCertificateErrorHandler(t *testing.T) {
	w, fw := testWindow(t, WindowOptions{})
	handles := func() bool { return onMainValue(fw.H.HandlesCertificateErrors) }
	ask := func(e platform.CertificateError) bool {
		return onMainValue(func() bool { return fw.H.CertificateError(e) })
	}
	der := []byte{0x30, 0x82, 0x01, 0x0a}
	e := platform.CertificateError{URL: "https://dev.example:8443/app", Host: "dev.example", Error: "the certificate's issuer is not known", Certificate: der}
	// By default certificate errors fail, and backends need not check.
	if handles() || ask(e) {
		t.Error("a certificate error is allowed without a handler")
	}
	var got CertificateError
	w.Page().SetCertificateErrorHandler(func(e CertificateError) bool {
		got = e
		return e.Host == "dev.example" && bytes.Equal(e.Certificate, der)
	})
	if !handles() || !ask(e) {
		t.Error("the handler's yes was not applied")
	}
	if got.URL != e.URL || got.Host != e.Host || got.Error != e.Error || !bytes.Equal(got.Certificate, der) {
		t.Errorf("error = %+v", got)
	}
	other := e
	other.Certificate = []byte{0x30, 0x00}
	if ask(other) {
		t.Error("the handler's no was not applied")
	}
	w.Page().SetCertificateErrorHandler(nil)
	if handles() || ask(e) {
		t.Error("SetCertificateErrorHandler(nil) did not restore the default")
	}
}

func TestClientCertificateHandler(t *testing.T) {
	w, fw := testWindow(t, WindowOptions{})
	ask := func(req platform.ClientCertificateRequest) platform.ClientCertificateResponse {
		return onMainValue(func() platform.ClientCertificateResponse { return fw.H.ClientCertificateRequested(req) })
	}
	mine := platform.ClientCertificate{
		DER: []byte{0x30, 0x82, 0x01, 0x0a}, Subject: "CN=me", Issuer: "CN=mygo e2e CA",
		ValidFrom: 1760000000, ValidTo: 1790000000, Native: 0x1234,
	}
	other := mine
	other.DER, other.Native = []byte{0x30, 0x00}, 0x5678
	challenge := platform.ClientCertificateRequest{Host: "intranet.example.com", Port: 443, Certificates: []platform.ClientCertificate{mine, other}}
	// Without a handler the web view decides, as before.
	if got := ask(challenge); got != (platform.ClientCertificateResponse{Default: true}) {
		t.Errorf("default answer = %+v", got)
	}
	var got ClientCertificateRequest
	w.Page().SetClientCertificateHandler(func(req ClientCertificateRequest) ClientCertificateResponse {
		got = req
		if len(req.Certificates) == 2 {
			return ClientCertificateResponse{Certificate: req.Certificates[1]}
		}
		return ClientCertificateResponse{Cancel: true}
	})
	if r := ask(challenge); r.Cancel || r.Default || r.Certificate == nil || !bytes.Equal(r.Certificate.DER, other.DER) || r.Certificate.Native != 0x5678 {
		t.Errorf("answer = %+v", r)
	}
	if got.Host != "intranet.example.com" || got.Port != 443 || got.IsProxy ||
		len(got.Certificates) != 2 || got.Certificates[0].Subject != "CN=me" || got.Certificates[0].Issuer != "CN=mygo e2e CA" ||
		got.Certificates[0].ValidFrom != 1760000000 || got.Certificates[0].ValidTo != 1790000000 ||
		!bytes.Equal(got.Certificates[0].Certificate, mine.DER) {
		t.Errorf("request = %+v", got)
	}
	// A certificate the request did not offer is as if none was chosen.
	proxy := platform.ClientCertificateRequest{Host: "proxy.lan", Port: 3128, IsProxy: true}
	if r := ask(proxy); !r.Cancel {
		t.Errorf("canceled answer = %+v", r)
	}
	unknown := ClientCertificateResponse{Certificate: ClientCertificate{Certificate: []byte{0xff}}}
	w.Page().SetClientCertificateHandler(func(ClientCertificateRequest) ClientCertificateResponse { return unknown })
	if r := ask(challenge); r.Cancel || r.Certificate != nil {
		t.Errorf("an unknown certificate was answered = %+v", r)
	}
	w.Page().SetClientCertificateHandler(nil)
	if r := ask(challenge); !r.Default {
		t.Error("SetClientCertificateHandler(nil) did not restore the default")
	}
}

// The page of a web view in native UI has handlers of its own.
func TestWebViewAuthHandlers(t *testing.T) {
	show := true
	_, v, _, fv := webViewWindow(t, &show)
	v.Page().SetAuthHandler(func(AuthRequest) AuthResponse { return AuthResponse{Username: "u", Password: "p"} })
	v.Page().SetCertificateErrorHandler(func(CertificateError) bool { return true })
	if r := onMainValue(func() platform.AuthResponse { return fv.H.AuthRequested(platform.AuthRequest{Host: "h"}) }); r.Username != "u" {
		t.Errorf("web view answer = %+v", r)
	}
	if !onMainValue(func() bool { return fv.H.CertificateError(platform.CertificateError{Host: "h"}) }) {
		t.Error("the web view's certificate error handler was not asked")
	}
}
