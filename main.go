package main

import (
	"crypto/rand"
	"flag"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	MQTTClient "github.com/pablitovicente/mqtt-load-generator/pkg/MQTTClient"
)

func publish(client mqtt.Client, messageCount *int, messageSize *int, targetTopic *string, interval *int) {
	payload := make([]byte, *messageSize)
	rand.Read(payload)
	// bar := progressbar.Default(int64(*messageCount))

	for i := 0; i < *messageCount; i++ {
		// bar.Add(1)
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
	numberOfClients := flag.Int("n", 1, "Number of concurrent MQTT clients")
	flag.Parse()


	for i := 1; i <= *numberOfClients; i++ {
		mqttClient := MQTTClient.Client{
			ID: i,
			Config: MQTTClient.Config{
				MessageCount: messageCount,
				MessageSize: messageSize,
				Interval: interval,
				TargetTopic: targetTopic,
				Username: username,
				Password: password,
				Host: host,
				Port: port,
			},
		}
	
		mqttConnection := mqttClient.New()
	
		go publish(mqttConnection, messageCount, messageSize, targetTopic, interval)
	}
	select {}
}
