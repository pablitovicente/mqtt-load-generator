package main

import (
	"flag"
	"fmt"
	"math/rand"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	MQTTClient "github.com/pablitovicente/mqtt-load-generator/pkg/MQTTClient"
)

func main() {
	// Argument parsing
	targetTopic := flag.String("t", "/load", "Target MQTT topic to publish messages to")
	username := flag.String("u", "", "MQTT username")
	password := flag.String("P", "", "MQTT password")
	host := flag.String("h", "localhost", "MQTT host")
	port := flag.Int("p", 1883, "MQTT port")
	qos := flag.Int("q", 1, "MQTT QoS used by all clients")

	flag.Parse()

	if *qos < 0 || *qos > 2 {
		panic("QoS should be any of [0, 1, 2]")
	}

	// General Client Config
	mqttClientConfig := MQTTClient.Config{
		TargetTopic: targetTopic,
		Username:    username,
		Password:    password,
		Host:        host,
		Port:        port,
		QoS:         qos,
	}

	rand.Seed(time.Now().UnixNano())
	updates := make(chan int)

	mqttClient := MQTTClient.Client{
		ID:      rand.Intn(100000),
		Config:  mqttClientConfig,
		Updates: updates,
	}

	mqttClient.Connect()

	mqttClient.Connection.Subscribe(*targetTopic, byte(*qos), func(c mqtt.Client, m mqtt.Message) {
		fmt.Println(m.Payload())
	})

	select {}
}
