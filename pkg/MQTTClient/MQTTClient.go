package MQTTClient

import (
	"fmt"
	"os"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type Config struct {
	MessageCount 	*int
	MessageSize		*int
	Interval			*int
	TargetTopic		*string
	Username 			*string
	Password 			*string
	Host					*string
	Port 					*int
}

type Client struct {
	ID          int
	Config			Config
}

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	fmt.Println("Connected")
}

func (c Client) New () mqtt.Client {
	fmt.Printf("Client id is %d \n", c.ID)
	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", *c.Config.Host, *c.Config.Port))
	opts.SetClientID(fmt.Sprintf("mqtt-load-generator-%d", c.ID))
	opts.SetUsername(*c.Config.Username)
	opts.SetPassword(*c.Config.Password)
	opts.OnConnect = connectHandler
	opts.OnConnectionLost = func (client mqtt.Client, err error) {
		fmt.Printf("Connection lost for client %d message: %v", c.ID, err)
	}
	mqttClient := mqtt.NewClient(opts)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		fmt.Println("Error establishing MQTT connection:", token.Error().Error())
		os.Exit(1)
	}

	return mqttClient
}