package main

import (
	"crypto/rand"
	"flag"
	"fmt"
	"os"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/schollz/progressbar/v3"
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
	bar := progressbar.Default(int64(*messageCount))

	for i := 0; i < *messageCount; i++ {
		bar.Add(1)
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
	targetTopic := flag.String("t", "/load", "Target MQTT topic to publish messages to")
	username := flag.String("u", "", "MQTT username")
	password := flag.String("P", "", "MQTT password")
	host := flag.String("h", "localhost", "MQTT host")
	port := flag.Int("p", 1883, "MQTT port")
	clientId := flag.String("id", "mqtt_load_generator", "MQTT clientID")
	flag.Parse()

	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", *host, *port))
	opts.SetClientID(*clientId)
	opts.SetUsername(*username)
	opts.SetPassword(*password)
	opts.SetDefaultPublishHandler(messagePubHandler)
	opts.OnConnect = connectHandler
	opts.OnConnectionLost = connectLostHandler
	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		fmt.Println("Error establishing MQTT connection:", token.Error().Error())
		os.Exit(1)
	}

	publish(client, messageCount, messageSize, targetTopic, interval)
}
