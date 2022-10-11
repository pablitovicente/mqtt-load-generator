package MQTTClient

import (
	"crypto/rand"
	"fmt"
	"os"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type Config struct {
	MessageCount *int
	MessageSize  *int
	Interval     *int
	TargetTopic  *string
	Username     *string
	Password     *string
	Host         *string
	Port         *int
}

type Client struct {
	ID         int
	Config     Config
	Connection mqtt.Client
}

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	fmt.Println("Connected")
}

func (c *Client) Connect() {
	fmt.Printf("Client id is %d \n", c.ID)
	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", *c.Config.Host, *c.Config.Port))
	opts.SetClientID(fmt.Sprintf("mqtt-load-generator-%d", c.ID))
	opts.SetUsername(*c.Config.Username)
	opts.SetPassword(*c.Config.Password)
	opts.OnConnect = connectHandler
	opts.OnConnectionLost = func(client mqtt.Client, err error) {
		fmt.Printf("Connection lost for client %d message: %v", c.ID, err)
	}
	mqttClient := mqtt.NewClient(opts)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		fmt.Println("Error establishing MQTT connection:", token.Error().Error())
		os.Exit(1)
	}

	c.Connection = mqttClient
}

func (c Client) Start() {
	payload := make([]byte, *c.Config.MessageSize)
	rand.Read(payload)

	for i := 0; i < *c.Config.MessageCount; i++ {
		token := c.Connection.Publish(*c.Config.TargetTopic, 1, false, payload)
		token.Wait()
		time.Sleep(time.Duration(*c.Config.Interval) * time.Millisecond)
	}
	fmt.Println("Done publishing for client:", c.ID)
	c.Connection.Disconnect(1)
}
