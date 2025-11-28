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

func monitorWSHandler(w http.ResponseWriter, r *http.Request, esClient *ESClient, hub *Hub) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Monitor WebSocket upgrade error:", err)
		return
	}

	clientIP := r.RemoteAddr
	log.Printf("Monitor client connected: %s", clientIP)

	hub.register <- Subscription{Conn: conn, Filter: ""}

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		defer func() {
			hub.unregister <- conn
		}()

		for range ticker.C {
			// CPU USAGE
			cpuPercent, err := cpu.Percent(0, false)
			if err != nil || len(cpuPercent) == 0 {
				continue
			}

			// CPU INFO
			cpuInfo, _ := cpu.Info()
			coreCount, _ := cpu.Counts(true)

			// MEMORY INFO
			memStat, _ := mem.VirtualMemory()

			// SYSTEM / OS INFO
			hostInfo, _ := host.Info()

			// Build metrics
			metrics := SystemMetrics{
				CPU:       cpuPercent[0],
				CPUModel:  cpuInfo[0].ModelName,
				Cores:     coreCount,
				Memory:    memStat.UsedPercent,
				TotalMem:  memStat.Total / 1024 / 1024 / 1024, // GB
				UsedMem:   memStat.Used / 1024 / 1024 / 1024,  // GB
				OS:        hostInfo.OS,
				Platform:  hostInfo.Platform,
				Kernel:    hostInfo.KernelVersion,
				Arch:      hostInfo.KernelArch,
				Uptime:    hostInfo.Uptime,
				Timestamp: time.Now().Format(time.RFC3339),
			}

			// Broadcast metrics to all clients
			hub.broadcast <- BroadcastMessage{
				Type:    "metrics",
				Data:    metrics,
				EventID: time.Now().Format("20060102150405"),
			}

			// Index to Elasticsearch (every 5 seconds)
			if time.Now().Second()%5 == 0 {
				go func() {
					if err := esClient.IndexMetrics(metrics); err != nil {
						log.Printf("Failed to index metrics: %v", err)
					}
				}()
			}
		}
	}()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Monitor client disconnected: %s", clientIP)
			hub.unregister <- conn
			break
		}
	}
}

func trackingWSHandler(w http.ResponseWriter, r *http.Request, esClient *ESClient, hub *Hub) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Tracking WebSocket upgrade error:", err)
		return
	}
	defer conn.Close()

	clientIP := r.RemoteAddr
	log.Printf(" Tracking client connected: %s", clientIP)

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
			log.Printf(" Failed to index session from %s: %v", clientIP, err)
		} else {
			log.Printf(" Session indexed: %s | %s | %s | %ds",
				session.Editor, session.Project, session.Language, session.DurationSeconds)
		}

		weekSeconds := hub.AddSessionRecord(clientIP, session.DurationSeconds)

		hub.broadcast <- BroadcastMessage{
			Type:    "session",
			Data:    session,
			EventID: time.Now().Format("20060102150405"),
		}

		summary := WeeklySummary{Client: clientIP, WeekSeconds: weekSeconds}
		hub.broadcast <- BroadcastMessage{Type: "weekly_summary", Data: summary, EventID: time.Now().Format("20060102150405")}

		ack := map[string]interface{}{
			"status":    "received",
			"timestamp": time.Now().Format(time.RFC3339),
		}
		ackJSON, _ := json.Marshal(ack)
		if err := conn.WriteMessage(websocket.TextMessage, ackJSON); err != nil {
			log.Printf(" Failed to send ack to %s: %v", clientIP, err)
			break
		}
	}

	log.Printf("Tracking client disconnected: %s", clientIP)
}

func externalWSHandler(w http.ResponseWriter, r *http.Request, hub *Hub) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("External WebSocket upgrade error:", err)
		return
	}
	defer conn.Close()

	clientIP := r.RemoteAddr
	log.Printf("External client connected: %s", clientIP)

	filter := r.URL.Query().Get("type")
	hub.register <- Subscription{Conn: conn, Filter: filter}
	defer func() { hub.unregister <- conn }()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			log.Printf("External client disconnected: %s", clientIP)
			break
		}
	}
}
