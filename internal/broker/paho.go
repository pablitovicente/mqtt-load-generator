package broker

// This is the only file in the program that imports paho.mqtt.golang.

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// Client is a connected MQTT client, backed by paho.mqtt.golang.
type Client struct {
	pahoClient mqtt.Client
}

// Dial connects to the broker described by options, waiting for the connection to complete or
// for ctx to be cancelled, whichever comes first. There is no OnConnect callback: Dial waits
// on the connect token itself.
//
// Auto-reconnect stays on, same as v1. logger reports connection loss and reconnect attempts.
func Dial(ctx context.Context, options Options, clientID string, logger *slog.Logger) (*Client, error) {
	tlsConfig, err := buildTLSConfig(options)
	if err != nil {
		return nil, err
	}

	clientOptions := mqtt.NewClientOptions()
	clientOptions.AddBroker(brokerURL(options))
	clientOptions.SetClientID(clientID)
	clientOptions.SetUsername(options.Username)
	clientOptions.SetPassword(options.Password)
	clientOptions.SetCleanSession(options.CleanSession)
	clientOptions.SetOrderMatters(false)
	clientOptions.SetKeepAlive(time.Duration(options.KeepAliveSeconds) * time.Second)

	if tlsConfig != nil {
		clientOptions.SetTLSConfig(tlsConfig)
	}

	clientOptions.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		logger.Warn("mqtt connection lost", "clientID", clientID, "error", err)
	})
	clientOptions.SetReconnectingHandler(func(_ mqtt.Client, _ *mqtt.ClientOptions) {
		logger.Warn("mqtt reconnecting", "clientID", clientID)
	})

	pahoClient := mqtt.NewClient(clientOptions)

	connectToken := pahoClient.Connect()
	select {
	case <-connectToken.Done():
		if err := connectToken.Error(); err != nil {
			return nil, fmt.Errorf("connecting to %s: %w", brokerURL(options), err)
		}
	case <-ctx.Done():
		// The client never finished connecting before ctx was cancelled, so it isn't usable.
		// Disconnect it now instead of leaving it retrying in the background.
		pahoClient.Disconnect(0)
		return nil, ctx.Err()
	}

	return &Client{pahoClient: pahoClient}, nil
}

// Publish sends payload to topic at the given QoS. The returned Token completes once the
// broker has acknowledged the publish (QoS 1 and 2), or once it is written (QoS 0).
func (client *Client) Publish(topic string, qos byte, retained bool, payload []byte) Token {
	return client.pahoClient.Publish(topic, qos, retained, payload)
}

// Subscribe asks the broker to deliver messages published to topic. callback is called once
// per received message, on paho's own goroutines, until Disconnect is called.
func (client *Client) Subscribe(topic string, qos byte, callback func(topic string, payload []byte)) Token {
	return client.pahoClient.Subscribe(topic, qos, func(_ mqtt.Client, message mqtt.Message) {
		callback(message.Topic(), message.Payload())
	})
}

// Disconnect closes the connection. Packets already queued to be sent get up to
// maxWaitForQueuedSends to go out first. It does not wait for acknowledgements of publishes
// already sent; callers that care wait for those tokens before disconnecting.
func (client *Client) Disconnect(maxWaitForQueuedSends time.Duration) {
	// paho calls this wait "quiesce" and takes it in milliseconds.
	client.pahoClient.Disconnect(uint(maxWaitForQueuedSends.Milliseconds()))
}
