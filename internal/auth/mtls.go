package auth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"

	"criticalsys.net/service-gateway/internal/config"
)

type clientCertKey struct{}

type ClientCertInfo struct {
	CommonName   string
	Organization string
}

type MTLSMiddleware struct {
	config config.MTLSConfig
}

func NewMTLSMiddleware(cfg config.MTLSConfig) *MTLSMiddleware {
	return &MTLSMiddleware{config: cfg}
}

func (m *MTLSMiddleware) LoadTLSConfig() (*tls.Config, error) {
	if !m.config.Enabled {
		return nil, nil
	}

	caCert, err := os.ReadFile(m.config.ClientCAPath)
	if err != nil {
		return nil, fmt.Errorf("read client CA: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse client CA certificate")
	}

	clientAuth := tls.VerifyClientCertIfGiven
	if m.config.RequireClient {
		clientAuth = tls.RequireAndVerifyClientCert
	}

	return &tls.Config{
		ClientCAs:  caCertPool,
		ClientAuth: clientAuth,
		MinVersion: tls.VersionTLS12,
	}, nil
}

func (m *MTLSMiddleware) Validate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Del("X-Client-CN")
		r.Header.Del("X-Client-Org")

		if !m.config.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		if r.TLS == nil {
			http.Error(w, "TLS required", http.StatusBadRequest)
			return
		}

		if m.config.RequireClient && len(r.TLS.PeerCertificates) == 0 {
			http.Error(w, "Client certificate required", http.StatusUnauthorized)
			return
		}

		if len(r.TLS.PeerCertificates) > 0 {
			cert := r.TLS.PeerCertificates[0]

			if len(r.TLS.VerifiedChains) == 0 {
				http.Error(w, "Client certificate not verified", http.StatusUnauthorized)
				return
			}

			info := &ClientCertInfo{
				CommonName: cert.Subject.CommonName,
			}
			if len(cert.Subject.Organization) > 0 {
				info.Organization = cert.Subject.Organization[0]
			}

			ctx := context.WithValue(r.Context(), clientCertKey{}, info)
			r = r.WithContext(ctx)
		}

		next.ServeHTTP(w, r)
	})
}

func GetClientCert(ctx context.Context) *ClientCertInfo {
	if info, ok := ctx.Value(clientCertKey{}).(*ClientCertInfo); ok {
		return info
	}
	return nil
}

func GetClientCN(r *http.Request) string {
	if info := GetClientCert(r.Context()); info != nil {
		return info.CommonName
	}
	return ""
}

func GetClientOrg(r *http.Request) string {
	if info := GetClientCert(r.Context()); info != nil {
		return info.Organization
	}
	return ""
}
