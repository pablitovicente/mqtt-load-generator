package main

import (
	"flag"

	MQTTClient "github.com/pablitovicente/mqtt-load-generator/pkg/MQTTClient"
)

func main() {
	// Argument parsing
	messageCount := flag.Int("c", 1000, "Number of messages to send")
	messageSize := flag.Int("s", 100, "Size in bytes of the message payload")
	interval := flag.Int("i", 1, "Milliseconds to wait between messages")
	targetTopic := flag.String("t", "/load", "Target MQTT topic to publish messages to")
	username := flag.String("u", "", "MQTT username")
	password := flag.String("P", "", "MQTT password")
	host := flag.String("h", "localhost", "MQTT host")
	port := flag.Int("p", 1883, "MQTT port")
	numberOfClients := flag.Int("n", 1, "Number of concurrent MQTT clients")
	flag.Parse()

	mqttClients := make([]MQTTClient.Client, 0)

	for i := 1; i <= *numberOfClients; i++ {
		// Configure the required number of clients
		mqttClient := MQTTClient.Client{
			ID: i,
			Config: MQTTClient.Config{
				MessageCount: messageCount,
				MessageSize:  messageSize,
				Interval:     interval,
				TargetTopic:  targetTopic,
				Username:     username,
				Password:     password,
				Host:         host,
				Port:         port,
			},
		}
		// Connect
		mqttClient.Connect()
		// Keep track of all clients. TODO: implement Go channels to make sure all connections are established before continuing.
		mqttClients = append(mqttClients, mqttClient)
	}

	// Now start publishing
	for _, c := range mqttClients {
		go c.Start()
	}
	// Dirty hack to block forever. TODO: implement signal Go channels to do this in an idiomatic way.
	select {}
}
