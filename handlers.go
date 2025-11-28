package main

import (
	"context"
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
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func monitorWSHandler(w http.ResponseWriter, r *http.Request, esClient *ESClient, hub *Hub) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Monitor WebSocket upgrade error:", err)
		return
	}

	clientIP := r.RemoteAddr
	log.Printf("Monitor client connected: %s", clientIP)

	hub.register <- Subscription{Conn: conn, Filter: "metrics"}

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		defer func() {
			hub.unregister <- conn
			log.Printf("Monitor client disconnected: %s", clientIP)
		}()

		for range ticker.C {
			cpuPercent, err := cpu.Percent(0, false)
			if err != nil || len(cpuPercent) == 0 {
				log.Printf("Failed to get CPU metrics: %v", err)
				continue
			}

			cpuInfo, err := cpu.Info()
			if err != nil || len(cpuInfo) == 0 {
				log.Printf("Failed to get CPU info: %v", err)
				continue
			}
			coreCount, _ := cpu.Counts(true)

			memStat, err := mem.VirtualMemory()
			if err != nil {
				log.Printf("Failed to get memory info: %v", err)
				continue
			}

			hostInfo, err := host.Info()
			if err != nil {
				log.Printf("Failed to get host info: %v", err)
				continue
			}

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

			hub.broadcast <- BroadcastMessage{
				Type:    "metrics",
				Data:    metrics,
				EventID: time.Now().Format("20060102150405"),
			}

			if esClient != nil && time.Now().Second()%5 == 0 {
				go func(m SystemMetrics) {
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
					defer cancel()

					done := make(chan error, 1)
					go func() {
						done <- esClient.IndexMetrics(m)
					}()

					select {
					case err := <-done:
						if err != nil {
							log.Printf("Failed to index metrics: %v", err)
						}
					case <-ctx.Done():
						log.Println("Elasticsearch metrics indexing timeout")
					}
				}(metrics)
			}
		}
	}()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Monitor client unexpected close: %s - %v", clientIP, err)
			}
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
	log.Printf("Tracking client connected: %s", clientIP)

	conn.SetReadDeadline(time.Now().Add(60 * time.Second))

	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()

	done := make(chan struct{})
	defer close(done)

	go func() {
		for {
			select {
			case <-pingTicker.C:
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			case <-done:
				return
			}
		}
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Tracking client unexpected close: %s - %v", clientIP, err)
			} else {
				log.Printf("Tracking client disconnected: %s", clientIP)
			}
			break
		}

		conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		var session CodingSession
		if err := json.Unmarshal(message, &session); err != nil {
			log.Printf("JSON parse error from %s: %v", clientIP, err)

			errResp := map[string]interface{}{
				"status": "error",
				"error":  "Invalid JSON format",
			}
			conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
			errJSON, _ := json.Marshal(errResp)
			conn.WriteMessage(websocket.TextMessage, errJSON)
			continue
		}

		if session.DurationSeconds <= 0 {
			log.Printf("Invalid duration from %s: %d", clientIP, session.DurationSeconds)
			continue
		}

		if esClient != nil {
			go func(s CodingSession) {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()

				done := make(chan error, 1)
				go func() {
					done <- esClient.IndexSession(s)
				}()

				select {
				case err := <-done:
					if err != nil {
						log.Printf("Failed to index session from %s: %v", clientIP, err)
					} else {
						log.Printf("Session indexed: %s | %s | %s | %ds",
							s.Editor, s.Project, s.Language, s.DurationSeconds)
					}
				case <-ctx.Done():
					log.Printf("Elasticsearch session indexing timeout for %s", clientIP)
				}
			}(session)
		}

		weekSeconds := hub.AddSessionRecord(clientIP, session.DurationSeconds)

		hub.broadcast <- BroadcastMessage{
			Type:    "session",
			Data:    session,
			EventID: time.Now().Format("20060102150405"),
		}

		summary := WeeklySummary{
			Client:      clientIP,
			WeekSeconds: weekSeconds,
		}
		hub.broadcast <- BroadcastMessage{
			Type:    "weekly_summary",
			Data:    summary,
			EventID: time.Now().Format("20060102150405"),
		}

		ack := map[string]interface{}{
			"status":       "received",
			"timestamp":    time.Now().Format(time.RFC3339),
			"week_seconds": weekSeconds,
		}

		conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		ackJSON, _ := json.Marshal(ack)
		if err := conn.WriteMessage(websocket.TextMessage, ackJSON); err != nil {
			log.Printf("Failed to send ack to %s: %v", clientIP, err)
			break
		}
	}
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

	filter := "session,weekly_summary"

	log.Printf("External client %s subscribed (session + weekly_summary only)", clientIP)

	hub.register <- Subscription{Conn: conn, Filter: filter}
	defer func() {
		hub.unregister <- conn
		log.Printf("External client disconnected: %s", clientIP)
	}()

	conn.SetReadDeadline(time.Now().Add(90 * time.Second))

	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		return nil
	})

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	done := make(chan struct{})
	defer close(done)

	go func() {
		for {
			select {
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			case <-done:
				return
			}
		}
	}()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("External client unexpected close: %s - %v", clientIP, err)
			}
			return
		}
		conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	}
}
