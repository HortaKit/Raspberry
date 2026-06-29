package main

import (
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"log"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"go.bug.st/serial"
)

const (
	DeviceID = "device-01"

	MQTTBroker = "ssl://f59ab996ae224d71a7a3187c60360d8d.s1.eu.hivemq.cloud:8883"
	MQTTUser   = "joaosvc"
	MQTTPass   = "qweqweROOT123"

	SerialPort = "/dev/serial0"
)

var port serial.Port

var handleCommand mqtt.MessageHandler = func(client mqtt.Client, msg mqtt.Message) {
	command := string(msg.Payload())

	if port != nil {
		switch command {
		case "CMD:REQ_HIST":
			SendHistory(client, DeviceID)
		case "CMD:PMP_1":
			port.Write([]byte("CMD:PUMP_ON\n"))
			fmt.Printf("Bomba ligada via MQTT\n")
		case "CMD:PMP_0":
			port.Write([]byte("CMD:PUMP_OFF\n"))
			fmt.Printf("Bomba desligada via MQTT\n")
		}
	}
}

func main() {
	opts := mqtt.NewClientOptions().AddBroker(MQTTBroker)
	opts.SetClientID(DeviceID)

	opts.SetUsername(MQTTUser)
	opts.SetPassword(MQTTPass)

	tlsConfig := &tls.Config{
		InsecureSkipVerify: false,
	}

	opts.SetTLSConfig(tlsConfig)

	opts.SetWill(fmt.Sprintf("dispositivos/%s/status", DeviceID), "OFFLINE", 1, true)

	opts.SetDefaultPublishHandler(handleCommand)

	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(5 * time.Second)

	opts.OnConnect = func(c mqtt.Client) {
		fmt.Printf("Sistema conectado ao broker, ID: %s\n", DeviceID)
		c.Subscribe(fmt.Sprintf("dispositivos/%s/comando", DeviceID), 1, nil)

		telemetriaTopic := fmt.Sprintf("dispositivos/%s/status", DeviceID)
		c.Publish(telemetriaTopic, 1, true, []byte("ONLINE_INIT"))
	}

	mqttClient := mqtt.NewClient(opts)

	go func() {
		for {
			if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
				log.Printf("Erro inicial MQTT: %v. Tentando novamente em 5s...", token.Error())
				time.Sleep(5 * time.Second)
			} else {
				break
			}
		}
	}()

	mode := &serial.Mode{
		BaudRate: 115200,
	}

	var err error
	port, err = serial.Open(SerialPort, mode)
	if err != nil {
		log.Fatalf("Erro ao abrir porta serial %s: %v", SerialPort, err)
	}
	defer port.Close()

	ReadHistory()
	StartHistoryScheduler(1 * time.Minute)
	StartMqttHistoryScheduler(mqttClient, 2*time.Minute)

	fmt.Printf("Sistema iniciado com sucesso!\n")

	const expectedBytes = 4
	buf := make([]byte, expectedBytes)

	executeRead := func() {
		port.ResetInputBuffer()

		_, err := port.Write([]byte("A"))
		if err != nil {
			log.Printf("Erro ao solicitar dados para STM: %v\n", err)
			return
		}

		bytesRead := 0
		timeout := time.After(500 * time.Millisecond)
		readFailed := false

		for bytesRead < expectedBytes {
			select {
			case <-timeout:
				log.Printf("Timeout ao ler dados da STM: %d/%d\n", bytesRead, expectedBytes)
				readFailed = true
			default:
				n, err := port.Read(buf[bytesRead:])
				if err != nil {
					log.Printf("Erro ao ler UART: %v\n", err)
					readFailed = true
					break
				}
				if n > 0 {
					bytesRead += n
				}
			}

			if readFailed {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}

		if !readFailed && bytesRead == expectedBytes {
			umidade := binary.LittleEndian.Uint16(buf[0:2])
			rele := binary.LittleEndian.Uint16(buf[2:4])

			if umidade > 3600 {
				log.Printf("Leitura invalida de umidade: %d\n", umidade)
				return
			}

			fmt.Printf("Umidade: %d, Relé: %d\n", umidade, rele)

			mqttFormat := fmt.Sprintf("D:%d,R:%d", umidade, rele)

			SaveHistory(umidade, uint8(rele))
			mqttClient.Publish(fmt.Sprintf("dispositivos/%s/telemetria", DeviceID), 1, false, []byte(mqttFormat))
		}
	}

	SendHistory(mqttClient, DeviceID)

	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		executeRead()
	}
}
