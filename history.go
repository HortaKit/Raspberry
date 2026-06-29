package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	HistoryFile = "history.bin"
	/** 4 bytes timestamp + 2 bytes umidade + 1 byte bomba */
	RecordSize = 7
)

var (
	bufferRead  []RecordHistory
	bufferMutex sync.Mutex
)

type RecordHistory struct {
	Timestamp uint32
	Umidade   uint16
	Bomba     uint8
}

func SaveHistory(umidade uint16, bomba uint8) {
	reg := RecordHistory{
		Timestamp: uint32(time.Now().Unix()),
		Umidade:   umidade,
		Bomba:     bomba,
	}

	bufferMutex.Lock()
	bufferRead = append(bufferRead, reg)
	bufferMutex.Unlock()
}

func StartHistoryScheduler(value time.Duration) {
	go func() {
		ticker := time.NewTicker(value)
		defer ticker.Stop()

		for range ticker.C {
			WriteHistory()
		}
	}()
}

func StartMqttHistoryScheduler(client mqtt.Client, value time.Duration) {
	go func() {
		ticker := time.NewTicker(value)
		defer ticker.Stop()

		for range ticker.C {
			if client.IsConnected() {
				SendHistory(client, DeviceID)
			}
		}
	}()
}

func WriteHistory() {
	bufferMutex.Lock()

	if len(bufferRead) == 0 {
		bufferMutex.Unlock()
		return
	}

	bufferReadCopy := bufferRead
	bufferMutex.Unlock()

	f, err := os.OpenFile(HistoryFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Erro ao abrir arquivo: %v\n", err)
		return
	}
	defer f.Close()

	for _, reg := range bufferReadCopy {
		err = binary.Write(f, binary.LittleEndian, reg)
		if err != nil {
			fmt.Printf("Erro ao escrever: %v\n", err)
		}
	}
}

func ReadHistory() {
	bufferMutex.Lock()
	defer bufferMutex.Unlock()

	if _, err := os.Stat(HistoryFile); os.IsNotExist(err) {
		return
	}

	raw, err := os.ReadFile(HistoryFile)
	if err != nil {
		fmt.Printf("Erro ao ler o arquivo: %v\n", err)
		return
	}
	totalBytes := len(raw)
	if totalBytes == 0 {
		return
	}

	if totalBytes%RecordSize != 0 {
		fmt.Println("Arquivo de historico corrompido")
	}

	reader := bytes.NewReader(raw)
	validRecords := make([]RecordHistory, 0)

	for i := 0; i < totalBytes/RecordSize; i++ {
		var reg RecordHistory
		err := binary.Read(reader, binary.LittleEndian, &reg)

		if err != nil {
			fmt.Printf("Erro ao ler %d: %v\n", i, err)
			break
		}

		if reg.Umidade > 3600 {
			fmt.Printf("Removido do registro errado do historico: %d\n", reg.Umidade)
			continue
		}

		validRecords = append(validRecords, reg)
	}

	bufferRead = validRecords
}

func SendHistory(client mqtt.Client, deviceID string) {
	bufferMutex.Lock()

	if len(bufferRead) == 0 {
		bufferMutex.Unlock()
		return
	}

	raw := make([]RecordHistory, len(bufferRead))
	copy(raw, bufferRead)
	bufferMutex.Unlock()

	var buffer bytes.Buffer

	for _, reg := range raw {
		err := binary.Write(&buffer, binary.LittleEndian, reg)
		if err != nil {
			fmt.Printf("Erro ao escrever: %v\n", err)
			return
		}
	}

	token := client.Publish(fmt.Sprintf("dispositivos/%s/historico", deviceID), 1, true, buffer.Bytes())
	token.Wait()

	if token.Error() != nil {
		fmt.Printf("Erro ao enviar o historico: %v\n", token.Error())
	}
}
