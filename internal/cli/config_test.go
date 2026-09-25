package cli

import "testing"

// validConnection returns a Connection that passes Validate(), for tests to mutate one field
// at a time.
func validConnection() Connection {
	return Connection{
		Host:             "localhost",
		Port:             1883,
		QoS:              1,
		CleanSession:     true,
		KeepAliveTimeout: 5,
		LogLevel:         "info",
	}
}

func TestConnectionValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Connection)
		wantErr bool
	}{
		{"valid baseline", func(connection *Connection) {}, false},
		{"qos below range", func(connection *Connection) { connection.QoS = -1 }, true},
		{"qos above range", func(connection *Connection) { connection.QoS = 3 }, true},
		{"qos 0 is valid", func(connection *Connection) { connection.QoS = 0 }, false},
		{"qos 2 is valid", func(connection *Connection) { connection.QoS = 2 }, false},
		{"port zero", func(connection *Connection) { connection.Port = 0 }, true},
		{"port above range", func(connection *Connection) { connection.Port = 65536 }, true},
		{"port at top of range is valid", func(connection *Connection) { connection.Port = 65535 }, false},
		{"keepAliveTimeout negative", func(connection *Connection) { connection.KeepAliveTimeout = -1 }, true},
		{"keepAliveTimeout zero is valid", func(connection *Connection) { connection.KeepAliveTimeout = 0 }, false},
		{"log level unknown", func(connection *Connection) { connection.LogLevel = "verbose" }, true},
		{"log level debug is valid", func(connection *Connection) { connection.LogLevel = "debug" }, false},
		{"log level warn is valid", func(connection *Connection) { connection.LogLevel = "warn" }, false},
		{"log level error is valid", func(connection *Connection) { connection.LogLevel = "error" }, false},
		{"tls: only cert set", func(connection *Connection) { connection.TLS.Cert = "cert.pem" }, true},
		{"tls: only ca set", func(connection *Connection) { connection.TLS.CA = "ca.pem" }, true},
		{"tls: only key set", func(connection *Connection) { connection.TLS.Key = "key.pem" }, true},
		{"tls: two of three set", func(connection *Connection) {
			connection.TLS.Cert = "cert.pem"
			connection.TLS.Key = "key.pem"
		}, true},
		{"tls: all three set is valid", func(connection *Connection) {
			connection.TLS.CA = "ca.pem"
			connection.TLS.Cert = "cert.pem"
			connection.TLS.Key = "key.pem"
		}, false},
		{"insecure without tls", func(connection *Connection) { connection.Insecure = true }, true},
		{"insecure with mqtts is valid", func(connection *Connection) {
			connection.Insecure = true
			connection.MQTTS = true
		}, false},
		{"insecure with tls files is valid", func(connection *Connection) {
			connection.Insecure = true
			connection.TLS.CA = "ca.pem"
			connection.TLS.Cert = "cert.pem"
			connection.TLS.Key = "key.pem"
		}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			connection := validConnection()
			tt.modify(&connection)

			err := connection.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// validPublish returns a Publish that passes Validate(), for tests to mutate one field at a
// time.
func validPublish() Publish {
	return Publish{
		Count:              1,
		Size:               100,
		Interval:           1,
		Schedule:           "flat",
		Clients:            1,
		InFlight:           1,
		AckTimeout:         1,
		ConnectConcurrency: 1,
	}
}

func TestPublishValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Publish)
		wantErr bool
	}{
		{"valid baseline", func(publish *Publish) {}, false},
		{"count zero", func(publish *Publish) { publish.Count = 0 }, true},
		{"size negative", func(publish *Publish) { publish.Size = -1 }, true},
		{"size zero is valid", func(publish *Publish) { publish.Size = 0 }, false},
		{"interval negative", func(publish *Publish) { publish.Interval = -1 }, true},
		{"interval zero is valid", func(publish *Publish) { publish.Interval = 0 }, false},
		{"schedule unknown", func(publish *Publish) { publish.Schedule = "sometimes" }, true},
		{"schedule normal is valid", func(publish *Publish) { publish.Schedule = "normal" }, false},
		{"schedule random is valid", func(publish *Publish) { publish.Schedule = "random" }, false},
		{"clients zero", func(publish *Publish) { publish.Clients = 0 }, true},
		{"inflight zero", func(publish *Publish) { publish.InFlight = 0 }, true},
		{"inflight above range", func(publish *Publish) { publish.InFlight = 65536 }, true},
		{"inflight at top of range is valid", func(publish *Publish) { publish.InFlight = 65535 }, false},
		{"ack-timeout zero", func(publish *Publish) { publish.AckTimeout = 0 }, true},
		{"ack-timeout negative", func(publish *Publish) { publish.AckTimeout = -1 }, true},
		{"connect-concurrency zero", func(publish *Publish) { publish.ConnectConcurrency = 0 }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			publish := validPublish()
			tt.modify(&publish)

			err := publish.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPublishValidateWithConnection(t *testing.T) {
	tests := []struct {
		name     string
		clientID string
		clients  int
		wantErr  bool
	}{
		{"no clientID, one client", "", 1, false},
		{"no clientID, many clients", "", 5, false},
		{"clientID with one client", "custom-id", 1, false},
		{"clientID with many clients", "custom-id", 5, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			connection := validConnection()
			connection.ClientID = tt.clientID

			publish := validPublish()
			publish.Clients = tt.clients

			err := publish.ValidateWithConnection(&connection)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWithConnection() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSubscribeValidate(t *testing.T) {
	tests := []struct {
		name       string
		resetAfter float64
		wantErr    bool
	}{
		{"positive value is valid", 30, false},
		{"zero", 0, true},
		{"negative", -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subscribe := Subscribe{ResetAfter: tt.resetAfter}

			err := subscribe.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
