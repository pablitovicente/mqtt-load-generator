package main

import (
	"crypto/rand"
	"flag"
	"fmt"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

var messagePubHandler mqtt.MessageHandler = func(client mqtt.Client, msg mqtt.Message) {
	fmt.Printf("Received message: %s from topic: %s\n", msg.Payload(), msg.Topic())
}

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	fmt.Println("Connected")
}

var connectLostHandler mqtt.ConnectionLostHandler = func(client mqtt.Client, err error) {
	fmt.Printf("Connect lost: %v", err)
}

func publish(client mqtt.Client, messageCount *int, messageSize *int, targetTopic *string, interval *int) {
	payload := make([]byte, *messageSize)
	rand.Read(payload)

	for i := 0; i < *messageCount; i++ {
		token := client.Publish(*targetTopic, 1, false, payload)
		token.Wait()
		time.Sleep(time.Duration(*interval) * time.Millisecond)
	}
}

func main() {
	// Argument parsing
	messageCount := flag.Int("c", 1000, "Number of messages to send")
	messageSize := flag.Int("s", 100, "Size in bytes of the message payload")
	interval := flag.Int("i", 1, "Milliseconds to wait between messages")
	targetTopic := flag.String("topic", "/load", "Target MQTT topic to publish messages to")
	username := flag.String("u", "", "MQTT username")
	password := flag.String("password", "", "MQTT password")
	clientId := flag.String("id", "mqtt_load_generator", "MQTT clientID")
	flag.Parse()


	var broker = "localhost"
	var port = 1883
	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", broker, port))
	opts.SetClientID(*clientId)
	opts.SetUsername(*username)
	opts.SetPassword(*password)
	opts.SetDefaultPublishHandler(messagePubHandler)
	opts.OnConnect = connectHandler
	opts.OnConnectionLost = connectLostHandler
	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	publish(client, messageCount, messageSize, targetTopic, interval)
}
