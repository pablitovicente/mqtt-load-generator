package MQTTClient

import (
	"sync"

	"github.com/google/uuid"
)

type Pool struct {
	MqttClients []*Client
	SetupDone   chan struct{}
}

func (p *Pool) New(numOfClients *int, clientConfig Config, updates chan int) {
	connectionDone := make(chan struct{})
	// Configure the required number of clients
	for c := 1; c <= *numOfClients; c++ {
		mqttClient := Client{
			ID:             uuid.NewString(),
			SubTopicId:     c,
			Config:         clientConfig,
			Updates:        updates,
			ConnectionDone: connectionDone,
		}
		// Connect
		mqttClient.Connect()
		// We wait until all clients connect
		<-connectionDone
		// Then we close the channel to be a good person
		p.MqttClients = append(p.MqttClients, &mqttClient)
	}

	close(connectionDone)
	close(p.SetupDone)
}

func (p *Pool) Start(wg *sync.WaitGroup) {
	for _, c := range p.MqttClients {
		wg.Add(1)
		go c.Start(wg)
	}
}
