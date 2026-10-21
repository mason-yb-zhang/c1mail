package mail

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
)

// loadRootCAs 系统池 + 本地 cacert.pem（设备 /etc/ssl/certs 为空）。
func loadRootCAs(skip bool) *tls.Config {
	cfg := &tls.Config{InsecureSkipVerify: skip} //nolint:gosec
	if skip {
		return cfg
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	for _, p := range caCandidates() {
		data, err := os.ReadFile(p)
		if err != nil || len(data) < 100 {
			continue
		}
		pool.AppendCertsFromPEM(data)
		break
	}
	cfg.RootCAs = pool
	return cfg
}

func caCandidates() []string {
	var out []string
	if v := os.Getenv("C1MAIL_CA"); v != "" {
		out = append(out, v)
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		out = append(out,
			filepath.Join(dir, "assets", "cacert.pem"),
			filepath.Join(dir, "..", "assets", "cacert.pem"),
		)
	}
	out = append(out,
		"/storage/c1/local-apps/apps/mail/assets/cacert.pem",
		"/storage/c1/local-apps/apps/radio/assets/cacert.pem",
		"assets/cacert.pem",
	)
	return out
}
