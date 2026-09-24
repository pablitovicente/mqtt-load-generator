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

func (client *fakeClient) Disconnect(_ time.Duration) {
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

// pendingToken is a broker.Token that stays unresolved until the test calls release, so tests
// can control exactly when a publish "completes" and check the in-flight window's limit in the
// meantime. WaitTimeout honours its timeout, like a real token would when the broker never
// answers: never calling release simulates a publish that times out.
type pendingToken struct {
	done chan struct{}
	err  error
}

func newPendingToken() *pendingToken {
	return &pendingToken{done: make(chan struct{})}
}

// release makes the token complete, with err as its result.
func (token *pendingToken) release(err error) {
	token.err = err
	close(token.done)
}

func (token *pendingToken) WaitTimeout(timeout time.Duration) bool {
	select {
	case <-token.done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (token *pendingToken) Error() error {
	return token.err
}

// fakePublishCall records one Publish call made through a fakePublisher.
type fakePublishCall struct {
	topic   string
	qos     byte
	payload []byte
}

// fakePublisher is a Publisher that records what was published and returns tokens from a
// caller-supplied function, so tests can control exactly what each publish "does": succeed,
// fail, or hang past the ack timeout (see pendingToken). With no function set, every publish
// succeeds immediately.
type fakePublisher struct {
	mutex sync.Mutex

	calls        []fakePublishCall
	disconnected bool

	nextToken func(call fakePublishCall) broker.Token
}

func (client *fakePublisher) Publish(topic string, qos byte, _ bool, payload []byte) broker.Token {
	client.mutex.Lock()
	call := fakePublishCall{topic: topic, qos: qos, payload: append([]byte(nil), payload...)}
	client.calls = append(client.calls, call)
	nextToken := client.nextToken
	client.mutex.Unlock()

	if nextToken != nil {
		return nextToken(call)
	}
	return &fakeToken{completed: true}
}

func (client *fakePublisher) Disconnect(_ time.Duration) {
	client.mutex.Lock()
	defer client.mutex.Unlock()

	client.disconnected = true
}

// publishCalls returns a copy of the Publish calls made so far.
func (client *fakePublisher) publishCalls() []fakePublishCall {
	client.mutex.Lock()
	defer client.mutex.Unlock()

	return append([]fakePublishCall(nil), client.calls...)
}

// wasDisconnected reports whether Disconnect was called.
func (client *fakePublisher) wasDisconnected() bool {
	client.mutex.Lock()
	defer client.mutex.Unlock()

	return client.disconnected
}
