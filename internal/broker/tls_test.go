package broker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const certsDir = "../../snake-oil-certs"

func TestBrokerURL(t *testing.T) {
	tests := []struct {
		name    string
		options Options
		want    string
	}{
		{
			name:    "plain tcp",
			options: Options{Host: "broker.example.com", Port: 1883},
			want:    "tcp://broker.example.com:1883",
		},
		{
			name:    "mqtts",
			options: Options{Host: "broker.example.com", Port: 8883, MQTTS: true},
			want:    "tls://broker.example.com:8883",
		},
		{
			name: "mTLS from cert/key/ca files, mqtts not set",
			options: Options{
				Host:        "broker.example.com",
				Port:        8883,
				TLSCertFile: filepath.Join(certsDir, "client-cert.pem"),
				TLSKeyFile:  filepath.Join(certsDir, "client-key.pem"),
				TLSCAFile:   filepath.Join(certsDir, "ca-cert.pem"),
			},
			want: "tls://broker.example.com:8883",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := brokerURL(tt.options); got != tt.want {
				t.Errorf("brokerURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildTLSConfig(t *testing.T) {
	t.Run("no TLS options gives no TLS config", func(t *testing.T) {
		config, err := buildTLSConfig(Options{Host: "localhost", Port: 1883})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if config != nil {
			t.Errorf("expected nil TLS config, got %+v", config)
		}
	})

	t.Run("mqtts only gives a TLS config with no client certificate", func(t *testing.T) {
		config, err := buildTLSConfig(Options{Host: "localhost", Port: 8883, MQTTS: true, Insecure: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if config == nil {
			t.Fatal("expected a TLS config, got nil")
		}
		if len(config.Certificates) != 0 {
			t.Errorf("expected no client certificates, got %d", len(config.Certificates))
		}
		if !config.InsecureSkipVerify {
			t.Errorf("expected InsecureSkipVerify to carry over from options.Insecure")
		}
	})

	t.Run("mqtts with CA only sets RootCAs and no client certificate", func(t *testing.T) {
		options := Options{
			Host:      "localhost",
			Port:      8883,
			MQTTS:     true,
			TLSCAFile: filepath.Join(certsDir, "ca-cert.pem"),
		}

		config, err := buildTLSConfig(options)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if config == nil {
			t.Fatal("expected a TLS config, got nil")
		}
		if len(config.Certificates) != 0 {
			t.Errorf("expected no client certificates, got %d", len(config.Certificates))
		}
		if config.RootCAs == nil {
			t.Errorf("expected RootCAs to be set from the CA file")
		}
	})

	t.Run("CA only with a bad CA file is an error", func(t *testing.T) {
		options := Options{
			Host:      "localhost",
			Port:      8883,
			MQTTS:     true,
			TLSCAFile: filepath.Join(certsDir, "does-not-exist.pem"),
		}

		if _, err := buildTLSConfig(options); err == nil {
			t.Error("expected an error for a missing CA file, got none")
		}
	})

	t.Run("valid mTLS file set builds a usable config", func(t *testing.T) {
		options := Options{
			Host:        "localhost",
			Port:        8883,
			TLSCertFile: filepath.Join(certsDir, "client-cert.pem"),
			TLSKeyFile:  filepath.Join(certsDir, "client-key.pem"),
			TLSCAFile:   filepath.Join(certsDir, "ca-cert.pem"),
		}

		config, err := buildTLSConfig(options)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if config == nil {
			t.Fatal("expected a TLS config, got nil")
		}
		if len(config.Certificates) != 1 {
			t.Errorf("expected 1 client certificate, got %d", len(config.Certificates))
		}
		if config.RootCAs == nil {
			t.Errorf("expected RootCAs to be set from the CA file")
		}
	})

	t.Run("missing cert file is an error", func(t *testing.T) {
		options := Options{
			TLSCertFile: filepath.Join(certsDir, "does-not-exist.pem"),
			TLSKeyFile:  filepath.Join(certsDir, "client-key.pem"),
			TLSCAFile:   filepath.Join(certsDir, "ca-cert.pem"),
		}

		if _, err := buildTLSConfig(options); err == nil {
			t.Error("expected an error for a missing cert file, got none")
		}
	})

	t.Run("missing CA file is an error", func(t *testing.T) {
		options := Options{
			TLSCertFile: filepath.Join(certsDir, "client-cert.pem"),
			TLSKeyFile:  filepath.Join(certsDir, "client-key.pem"),
			TLSCAFile:   filepath.Join(certsDir, "does-not-exist.pem"),
		}

		if _, err := buildTLSConfig(options); err == nil {
			t.Error("expected an error for a missing CA file, got none")
		}
	})

	t.Run("CA file with no certificates is an error", func(t *testing.T) {
		// v1 silently connected with an empty pool in this case (AppendCertsFromPEM's
		// return value was never checked). Iteration 1 fixes that: it's a hard error.
		emptyCAFile := filepath.Join(t.TempDir(), "empty-ca.pem")
		if err := os.WriteFile(emptyCAFile, []byte("not a certificate\n"), 0o600); err != nil {
			t.Fatalf("could not write test fixture: %v", err)
		}

		options := Options{
			TLSCertFile: filepath.Join(certsDir, "client-cert.pem"),
			TLSKeyFile:  filepath.Join(certsDir, "client-key.pem"),
			TLSCAFile:   emptyCAFile,
		}

		_, err := buildTLSConfig(options)
		if err == nil {
			t.Fatal("expected an error for a CA file with no certificates, got none")
		}
		if !strings.Contains(err.Error(), "no usable certificates") {
			t.Errorf("expected error to mention no usable certificates, got: %v", err)
		}
	})

	t.Run("partial file set is treated the same as none set", func(t *testing.T) {
		// cli's Connection.Validate rejects a partial set before it reaches here; this just
		// documents that buildTLSConfig itself falls back to "no mTLS" rather than erroring.
		options := Options{
			TLSCertFile: filepath.Join(certsDir, "client-cert.pem"),
			TLSKeyFile:  filepath.Join(certsDir, "client-key.pem"),
		}

		config, err := buildTLSConfig(options)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if config != nil {
			t.Errorf("expected nil TLS config for a partial file set, got %+v", config)
		}
	})
}
