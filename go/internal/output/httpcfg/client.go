package httpcfg

import (
	"crypto/tls"
	"net/http"
	"strings"
	"time"
)

func NewClient(verifySSL bool, sslProtocol string, timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: NewTLSConfig(verifySSL, sslProtocol),
		},
		Timeout: timeout,
	}
}

func NewTLSConfig(verifySSL bool, sslProtocol string) *tls.Config {
	cfg := &tls.Config{InsecureSkipVerify: !verifySSL}
	switch normalizeTLSProfile(sslProtocol) {
	case "tls10":
		cfg.MinVersion = tls.VersionTLS10
		cfg.MaxVersion = tls.VersionTLS10
	case "tls11":
		cfg.MinVersion = tls.VersionTLS11
		cfg.MaxVersion = tls.VersionTLS11
	case "tls12":
		cfg.MinVersion = tls.VersionTLS12
		cfg.MaxVersion = tls.VersionTLS12
	case "tls13":
		cfg.MinVersion = tls.VersionTLS13
		cfg.MaxVersion = tls.VersionTLS13
	}
	return cfg
}

func normalizeTLSProfile(value string) string {
	normalized := strings.NewReplacer(".", "", "_", "", "-", "", " ", "").Replace(strings.ToLower(strings.TrimSpace(value)))
	switch normalized {
	case "", "default":
		return "default"
	case "tls10", "tls1", "tlsv10", "tlsversion10":
		return "tls10"
	case "tls11", "tlsv11", "tlsversion11":
		return "tls11"
	case "tls12", "tlsv12", "tlsversion12":
		return "tls12"
	case "tls13", "tlsv13", "tlsversion13":
		return "tls13"
	default:
		return "default"
	}
}
