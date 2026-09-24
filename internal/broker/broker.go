// Package broker connects to an MQTT broker. It is the only package that knows about
// paho.mqtt.golang: Dial returns our own *Client, and the code that uses a client declares the
// small interface it needs (see mqttload.Subscriber), so it can be tested with a fake.
package broker

import "time"

// Token is the result of an operation that completes asynchronously, such as a publish or a
// subscribe. It is a small subset of paho's own Token interface: just enough for callers to
// wait for the result and check whether it succeeded.
//
// Unlike Client, this is an interface: Client's methods return it, and Go only lets a type
// satisfy an interface when the method signatures match exactly. If Publish returned a
// concrete token type, the interfaces declared by callers would have to name that type too,
// and a fake client could never return a fake token.
type Token interface {
	// WaitTimeout blocks until the operation completes or the timeout passes. It returns
	// true if the operation completed in time.
	WaitTimeout(timeout time.Duration) bool

	// Error returns the error from the operation, once it has completed. It is only
	// meaningful after WaitTimeout has returned true.
	Error() error
}

// Options holds the settings needed to open an MQTT connection: broker address, credentials,
// TLS and session settings. broker must not import the cli package (cli imports broker), so
// cli converts its own Connection into an Options value before calling Dial.
type Options struct {
	Host     string
	Port     int
	Username string
	Password string

	// CleanSession and KeepAliveSeconds match paho's own settings of the same name.
	CleanSession     bool
	KeepAliveSeconds int64

	// MQTTS turns on plain TLS (encryption only, no client certificate) when none of the
	// three TLS file fields below are set. It has no effect when they are set: a full
	// mTLS setup always implies TLS.
	MQTTS bool

	// Insecure disables server certificate verification (InsecureSkipVerify). It applies to
	// both the MQTTS-only and the mTLS case.
	Insecure bool

	// TLSCertFile, TLSKeyFile and TLSCAFile together turn on mutual TLS. All three must be
	// set for mTLS to be used; the caller (cli's Connection.Validate) is responsible for
	// rejecting a partial set before it gets here.
	TLSCertFile string
	TLSKeyFile  string
	TLSCAFile   string
}
