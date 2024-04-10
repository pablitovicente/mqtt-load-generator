package MQTTClient

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type Config struct {
	MessageCount  *int
	MessageSize   *int
	Interval      *int
	TargetTopic   *string
	Username      *string
	Password      *string
	Host          *string
	Schedule      *string
	Port          *int
	IdAsSubTopic  *bool
	QoS           *int
	TLSConfigured bool
	CA            *string
	Cert          *string
	Key           *string
	Insecure      *bool
	MQTTS         *bool
}

type Client struct {
	ID             string
	SubTopicId     int
	Config         Config
	Connection     mqtt.Client
	Updates        chan int
	ConnectionDone chan struct{}
}

func (c *Client) Connect() {
	opts := mqtt.NewClientOptions()
	opts.SetClientID(fmt.Sprintf("mqtt-load-generator-%s", c.ID))
	opts.SetUsername(*c.Config.Username)
	opts.SetPassword(*c.Config.Password)
	opts.CleanSession = true
	opts.SetOrderMatters(false)
	// TLS config if configured
	if c.Config.TLSConfigured {
		cer, err := tls.LoadX509KeyPair(*c.Config.Cert, *c.Config.Key)
		if err != nil {
			fmt.Println("Error reading certificate and/or key")
			panic(err)
		}

		caCertFile, err := ioutil.ReadFile(*c.Config.CA)
		if err != nil {
			fmt.Println("Error reading CA file")
			panic(err)
		}

		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCertFile)

		opts.SetTLSConfig(&tls.Config{
			Certificates:       []tls.Certificate{cer},
			ClientCAs:          caCertPool,
			RootCAs:            caCertPool,
			InsecureSkipVerify: *c.Config.Insecure,
		})

		opts.AddBroker(fmt.Sprintf("tls://%s:%d", *c.Config.Host, *c.Config.Port))
	} else {
		protocol := "tcp"
		if *c.Config.MQTTS {
			tlsConfig := &tls.Config{
				InsecureSkipVerify: *c.Config.Insecure,
			}
			opts.SetTLSConfig(tlsConfig)
			protocol = "tls"
		}

		opts.AddBroker(fmt.Sprintf("%s://%s:%d", protocol, *c.Config.Host, *c.Config.Port))
	}

	// We use a closure so we can have access to the scope if required
	opts.OnConnect = func(client mqtt.Client) {
		c.ConnectionDone <- struct{}{}
	}
	opts.OnConnectionLost = func(client mqtt.Client, err error) {
		optionsReader := client.OptionsReader()
		fmt.Printf("Connection lost for client '%s' message: %v\n", optionsReader.ClientID(), err.Error())
	}

	opts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
		// We just send a 1 for each received message
		c.Updates <- 1
	})

	mqttClient := mqtt.NewClient(opts)

	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		fmt.Println("Error establishing MQTT connection:", token.Error().Error())
		os.Exit(1)
	}

	c.Connection = mqttClient
}

func (c Client) Start(wg *sync.WaitGroup) {
	defer wg.Done()
	payload := make([]byte, *c.Config.MessageSize)
	rand.Read(payload)

	var topic string
	if *c.Config.IdAsSubTopic {
		topic = fmt.Sprintf("%s/%d", *c.Config.TargetTopic, c.SubTopicId)
	} else {
		topic = *c.Config.TargetTopic
	}

	for i := 0; i < *c.Config.MessageCount; i++ {
		token := c.Connection.Publish(topic, byte(*c.Config.QoS), false, payload)
		token.Wait()

		// If the interval is zero skip this logic
		interval := float64(*c.Config.Interval)
		if interval > 0 {
			// Default case is flat
			sleepTime := interval
			if *c.Config.Schedule == "normal" {
				sleepTime = interval + interval*rand.NormFloat64()/2

			} else if *c.Config.Schedule == "random" {
				sleepTime = interval * 2 * rand.Float64()
			}
			time.Sleep(time.Duration(sleepTime) * time.Millisecond)
		}
		c.Updates <- 1
	}
	c.Connection.Disconnect(1)
}

func (c Client) Subscribe(topic string) {
	token := c.Connection.Subscribe(topic, 1, nil)
	token.Wait()
	fmt.Printf("Subscribed to topic '%s'\n", topic)
}
