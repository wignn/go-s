package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// monitorWSHandler - Real-time system monitoring with Elasticsearch logging
func monitorWSHandler(w http.ResponseWriter, r *http.Request, esClient *ESClient, hub *Hub) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Monitor WebSocket upgrade error:", err)
		return
	}
	defer conn.Close()

	clientIP := r.RemoteAddr
	log.Printf("Monitor client connected: %s", clientIP)

	// register client to hub so it receives broadcasts
	hub.register <- conn
	defer func() { hub.unregister <- conn }()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		cpuPercent, err := cpu.Percent(0, false)
		if err != nil || len(cpuPercent) == 0 {
			continue
		}

		cpuInfo, _ := cpu.Info()
		coreCount, _ := cpu.Counts(true)
		memStat, _ := mem.VirtualMemory()
		hostInfo, _ := host.Info()

		metrics := SystemMetrics{
			CPU:       cpuPercent[0],
			CPUModel:  cpuInfo[0].ModelName,
			Cores:     coreCount,
			Memory:    memStat.UsedPercent,
			TotalMem:  memStat.Total / 1024 / 1024 / 1024,
			UsedMem:   memStat.Used / 1024 / 1024 / 1024,
			OS:        hostInfo.OS,
			Platform:  hostInfo.Platform,
			Kernel:    hostInfo.KernelVersion,
			Arch:      hostInfo.KernelArch,
			Uptime:    hostInfo.Uptime,
			Timestamp: time.Now().Format(time.RFC3339),
		}

		jsonBytes, _ := json.Marshal(metrics)
		if err := conn.WriteMessage(websocket.TextMessage, jsonBytes); err != nil {
			log.Printf("Monitor write error for %s: %v", clientIP, err)
			break
		}

		if time.Now().Second()%5 == 0 {
			// pass metrics into goroutine to avoid race on variable
			m := metrics
			go func(m SystemMetrics) {
				if err := esClient.IndexMetrics(m); err != nil {
					log.Printf("Failed to index metrics: %v", err)
				}
			}(m)
		}

		// also broadcast metrics to all monitor clients via hub
		hub.broadcast <- BroadcastMessage{Type: "metrics", Data: metrics, EventID: eventID()}
	}

	log.Printf("Monitor client disconnected: %s", clientIP)
}

// trackingWSHandler - Coding session tracking
func trackingWSHandler(w http.ResponseWriter, r *http.Request, esClient *ESClient, hub *Hub) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Tracking WebSocket upgrade error:", err)
		return
	}
	defer conn.Close()

	clientIP := r.RemoteAddr
	log.Printf("Tracking client connected: %s", clientIP)

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Tracking read error for %s: %v", clientIP, err)
			break
		}

		var session CodingSession
		if err := json.Unmarshal(message, &session); err != nil {
			log.Printf("JSON parse error from %s: %v", clientIP, err)
			continue
		}

		if err := esClient.IndexSession(session); err != nil {
			log.Printf("Failed to index session from %s: %v", clientIP, err)
		} else {
			log.Printf("Session indexed: %s | %s | %s | %ds",
				session.Editor, session.Project, session.Language, session.DurationSeconds)
		}

		// broadcast session to all monitor clients
		hub.broadcast <- BroadcastMessage{Type: "session", Data: session, EventID: eventID()}

		ack := map[string]interface{}{"status": "received", "timestamp": time.Now().Format(time.RFC3339)}
		ackJSON, _ := json.Marshal(ack)
		if err := conn.WriteMessage(websocket.TextMessage, ackJSON); err != nil {
			log.Printf("Failed to send ack to %s: %v", clientIP, err)
			break
		}
	}

	log.Printf("Tracking client disconnected: %s", clientIP)
}
