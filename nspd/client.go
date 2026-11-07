package nspd

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"net"
	"net/http"
	"net/url"
	"time"

	"gis-backend/config"
)

// The public map presents a certificate signed by the Russian trusted root.
// That root is not in the container's default store, so the client adds it.
//
//go:embed russian_trusted_root_ca.pem
var russianRootPEM []byte

func httpClient() *http.Client {
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}
	roots.AppendCertsFromPEM(russianRootPEM)
	serverName := "nspd.gov.ru"
	if parsed, err := url.Parse(config.NSPDBaseURL()); err == nil && parsed.Hostname() != "" {
		serverName = parsed.Hostname()
	}
	dialer := &net.Dialer{Timeout: 8 * time.Second}
	pinned := config.NSPDDialAddr()
	return &http.Client{
		Timeout: 12 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    roots,
				ServerName: serverName,
			},
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				if pinned != "" {
					_, port, splitErr := net.SplitHostPort(addr)
					if splitErr != nil || port == "" {
						port = "443"
					}
					addr = net.JoinHostPort(pinned, port)
				}
				return dialer.DialContext(ctx, network, addr)
			},
		},
	}
}
