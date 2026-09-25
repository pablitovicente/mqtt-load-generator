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

// buildTLSConfig builds the *tls.Config to use for the connection, or nil for a plain
// connection. With a full set of mTLS files it loads the client certificate and key, and
// verifies the broker's certificate against the CA file. With only --mqtts it returns a TLS
// config with no client certificate.
//
// Unlike v1, a CA file that contains no usable certificates is an error instead of silently
// connecting with an empty pool.
func buildTLSConfig(options Options) (*tls.Config, error) {
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

	caFileContents, err := os.ReadFile(options.TLSCAFile)
	if err != nil {
		return nil, fmt.Errorf("reading CA file: %w", err)
	}

	caCertificatePool := x509.NewCertPool()
	if !caCertificatePool.AppendCertsFromPEM(caFileContents) {
		return nil, fmt.Errorf("CA file %q has no usable certificates", options.TLSCAFile)
	}

	return &tls.Config{
		Certificates:       []tls.Certificate{certificate},
		RootCAs:            caCertificatePool,
		InsecureSkipVerify: options.Insecure,
	}, nil
}
