package mqttload

import (
	"sync"
	"time"

	"github.com/pablitovicente/mqtt-load-generator/internal/broker"
)

// fakeToken is a broker.Token whose result a test sets directly.
type fakeToken struct {
	// completed false makes WaitTimeout return false, as if the broker never answered.
	completed bool
	err       error
}

func (token *fakeToken) WaitTimeout(_ time.Duration) bool {
	return token.completed
}

func (token *fakeToken) Error() error {
	return token.err
}

// fakeSubscription records one Subscribe call.
type fakeSubscription struct {
	topic    string
	qos      byte
	callback func(topic string, payload []byte)
}

// fakeClient is a Subscriber that records what was subscribed and lets a test deliver
// messages to it, instead of talking to a real broker.
type fakeClient struct {
	mutex sync.Mutex

	subscriptions []fakeSubscription
	disconnected  bool

	// subscribeToken, if set, is returned by Subscribe instead of a token that succeeds.
	subscribeToken broker.Token
}

func (client *fakeClient) Subscribe(topic string, qos byte, callback func(topic string, payload []byte)) broker.Token {
	client.mutex.Lock()
	defer client.mutex.Unlock()

	client.subscriptions = append(client.subscriptions, fakeSubscription{topic: topic, qos: qos, callback: callback})

	if client.subscribeToken != nil {
		return client.subscribeToken
	}
	return &fakeToken{completed: true}
}

func (client *fakeClient) Disconnect(_ uint) {
	client.mutex.Lock()
	defer client.mutex.Unlock()

	client.disconnected = true
}

// deliver calls the callback of every subscription on topic, as a broker would when a
// matching message arrives.
func (client *fakeClient) deliver(topic string, payload []byte) {
	client.mutex.Lock()
	var callbacks []func(string, []byte)
	for _, subscription := range client.subscriptions {
		if subscription.topic == topic {
			callbacks = append(callbacks, subscription.callback)
		}
	}
	client.mutex.Unlock()

	for _, callback := range callbacks {
		callback(topic, payload)
	}
}

// subscribeCalls returns a copy of the Subscribe calls made so far.
func (client *fakeClient) subscribeCalls() []fakeSubscription {
	client.mutex.Lock()
	defer client.mutex.Unlock()

	return append([]fakeSubscription(nil), client.subscriptions...)
}

// wasDisconnected reports whether Disconnect was called.
func (client *fakeClient) wasDisconnected() bool {
	client.mutex.Lock()
	defer client.mutex.Unlock()

	return client.disconnected
}
