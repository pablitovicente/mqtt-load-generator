package broker

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// usesTLSFiles reports whether options carries a full set of mTLS files. All three files must
// be set together; cli's Connection.Validate rejects a partial set before it gets here.
func usesTLSFiles(options Options) bool {
	return options.TLSCertFile != "" && options.TLSKeyFile != "" && options.TLSCAFile != ""
}

// brokerURL builds the broker address paho connects to: tls:// when there is a full set of
// mTLS files, or when --mqtts was given, tcp:// otherwise. This matches v1's Connect().
func brokerURL(options Options) string {
	scheme := "tcp"
	if usesTLSFiles(options) || options.MQTTS {
		scheme = "tls"
	}

	return fmt.Sprintf("%s://%s:%d", scheme, options.Host, options.Port)
}

// loadCACertificatePool reads caFile and parses it into a certificate pool, used to check the
// broker's certificate instead of the system's trusted CAs.
//
// Unlike v1, a CA file that contains no usable certificates is an error instead of silently
// connecting with an empty pool.
func loadCACertificatePool(caFile string) (*x509.CertPool, error) {
	caFileContents, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("reading CA file: %w", err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caFileContents) {
		return nil, fmt.Errorf("CA file %q has no usable certificates", caFile)
	}

	return pool, nil
}

// buildTLSConfig builds the *tls.Config to use for the connection, or nil for a plain
// connection. TLS is on with --mqtts, or with all three of --ca, --cert and --key (as in 1.x).
// Then:
//   - --ca: check the broker's certificate against this CA file instead of the system's
//     trusted CAs.
//   - --cert and --key: present this client certificate (mutual TLS).
//   - --insecure: skip checking the broker's certificate.
func buildTLSConfig(options Options) (*tls.Config, error) {
	if !options.MQTTS && !usesTLSFiles(options) {
		return nil, nil
	}

	config := &tls.Config{InsecureSkipVerify: options.Insecure}

	if options.TLSCAFile != "" {
		pool, err := loadCACertificatePool(options.TLSCAFile)
		if err != nil {
			return nil, err
		}
		config.RootCAs = pool
	}

	if options.TLSCertFile != "" {
		certificate, err := tls.LoadX509KeyPair(options.TLSCertFile, options.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("loading TLS certificate and key: %w", err)
		}
		config.Certificates = []tls.Certificate{certificate}
	}

	return config, nil
}
