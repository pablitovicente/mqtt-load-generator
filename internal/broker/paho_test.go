package broker

import (
	"context"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
)

// TestDial_PlainConnectionClosedRightAwayHintsAtTLS checks the error when a broker closes a
// plain connection straight away, which is what a TLS-only port does. paho reports a bare EOF;
// the error should point at --mqtts.
func TestDial_PlainConnectionClosedRightAwayHintsAtTLS(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	// Accept each connection and close it without a word, like a TLS port seeing plain MQTT.
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			_ = connection.Close()
		}
	}()

	port := listener.Addr().(*net.TCPAddr).Port
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	_, err = Dial(context.Background(), Options{Host: "127.0.0.1", Port: port, CleanSession: true, KeepAliveSeconds: 5}, "test-client", logger)
	if err == nil {
		t.Fatal("expected an error, got none")
	}
	if !strings.Contains(err.Error(), "--mqtts") {
		t.Errorf("expected the error to suggest --mqtts, got: %v", err)
	}
}
