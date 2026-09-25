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

// usesCAOnly reports whether options carries a CA file with no client certificate: --ca given
// on its own, with --mqtts. cli's Connection.Validate only allows this combination together
// with --mqtts, or as part of the full mTLS set handled by usesTLSFiles.
func usesCAOnly(options Options) bool {
	return options.TLSCAFile != "" && options.TLSCertFile == "" && options.TLSKeyFile == ""
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
// connection. With a full set of mTLS files it loads the client certificate and key, and
// checks the broker's certificate against the CA file. With --ca on its own it checks the
// broker's certificate against that CA file too, but without presenting a client certificate.
// With only --mqtts it returns a TLS config with no client certificate, checked against the
// system's trusted CAs.
func buildTLSConfig(options Options) (*tls.Config, error) {
	if usesCAOnly(options) {
		pool, err := loadCACertificatePool(options.TLSCAFile)
		if err != nil {
			return nil, err
		}
		return &tls.Config{RootCAs: pool, InsecureSkipVerify: options.Insecure}, nil
	}

	if !usesTLSFiles(options) {
		if options.MQTTS {
			return &tls.Config{InsecureSkipVerify: options.Insecure}, nil
		}
		return nil, nil
	}

	certificate, err := tls.LoadX509KeyPair(options.TLSCertFile, options.TLSKeyFile)
	if err != nil {
		return nil, fmt.Errorf("loading TLS certificate and key: %w", err)
	}

	pool, err := loadCACertificatePool(options.TLSCAFile)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates:       []tls.Certificate{certificate},
		RootCAs:            pool,
		InsecureSkipVerify: options.Insecure,
	}, nil
}
