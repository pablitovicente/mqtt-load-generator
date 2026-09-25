package cli

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/pablitovicente/mqtt-load-generator/internal/broker"
	"github.com/pablitovicente/mqtt-load-generator/internal/mqttload"
)

// connectedClient is what a connected MQTT client can do. sub and dump only subscribe, but pub also
// publishes, so connectFunc returns an interface wide enough for all three; each command's own
// code only calls the methods it actually needs.
type connectedClient interface {
	mqttload.Subscriber
	mqttload.Publisher
}

// connectFunc opens a connection to the broker. The real one is connectToBroker; tests pass a
// fake so they don't need a broker.
type connectFunc func(ctx context.Context, options broker.Options, clientID string, logger *slog.Logger) (connectedClient, error)

// connectToBroker is the real connectFunc. It checks the error before returning, because
// returning a nil *broker.Client as a connectedClient would give the caller a non-nil interface
// holding a nil pointer.
func connectToBroker(ctx context.Context, options broker.Options, clientID string, logger *slog.Logger) (connectedClient, error) {
	client, err := broker.Dial(ctx, options, clientID, logger)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// toBrokerOptions converts our own Connection into broker.Options, the shape broker.Dial takes.
// broker must not import cli (that would be a cycle), so this conversion lives here instead.
func (connection *Connection) toBrokerOptions() broker.Options {
	return broker.Options{
		Host:             connection.Host,
		Port:             connection.Port,
		Username:         connection.Username,
		Password:         connection.Password,
		CleanSession:     connection.CleanSession,
		KeepAliveSeconds: connection.KeepAliveTimeout,
		MQTTS:            connection.MQTTS,
		Insecure:         connection.Insecure,
		TLSCertFile:      connection.TLS.Cert,
		TLSKeyFile:       connection.TLS.Key,
		TLSCAFile:        connection.TLS.CA,
	}
}

// effectiveClientID returns --clientID if the user set one, otherwise a generated ID in the
// same shape v1 used: "mqtt-load-generator-<uuid>". Connection.Validate already rejects a
// custom client ID together with more than one client, so this is safe to call as-is.
func (connection *Connection) effectiveClientID() string {
	if connection.ClientID != "" {
		return connection.ClientID
	}
	return "mqtt-load-generator-" + uuid.NewString()
}
