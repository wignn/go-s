package main

import (
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Hub struct {
	// clients maps connection -> filter ("" == all, "metrics" == metrics only, etc)
	clients map[*websocket.Conn]string

	// broadcast channel for sending messages to clients
	broadcast chan BroadcastMessage

	// register accepts Subscription objects (Conn + Filter)
	register chan Subscription

	// unregister removes clients
	unregister chan *websocket.Conn

	// mutex for thread-safe access
	mutex sync.RWMutex

	// weekly session records keyed by client identifier (e.g., client IP)
	weeklyRecords map[string][]SessionRecord
}

func newHub() *Hub {
	return &Hub{
		clients:       make(map[*websocket.Conn]string),
		broadcast:     make(chan BroadcastMessage, 256),
		register:      make(chan Subscription),
		unregister:    make(chan *websocket.Conn),
		weeklyRecords: make(map[string][]SessionRecord),
	}
}

func (h *Hub) run() {
	log.Println("Hub started")

	for {
		select {
		case sub := <-h.register:
			h.mutex.Lock()
			h.clients[sub.Conn] = sub.Filter
			clientCount := len(h.clients)
			h.mutex.Unlock()
			log.Printf("Client registered with filter '%s'. Total clients: %d", sub.Filter, clientCount)

		case client := <-h.unregister:
			h.mutex.Lock()
			if filter, ok := h.clients[client]; ok {
				delete(h.clients, client)
				client.Close()
				clientCount := len(h.clients)
				log.Printf("Client unregistered (filter: %s). Total clients: %d", filter, clientCount)
			}
			h.mutex.Unlock()

		case message := <-h.broadcast:
			h.broadcastMessage(message)
		}
	}
}

func (h *Hub) broadcastMessage(message BroadcastMessage) {
	h.mutex.RLock()
	clientsCopy := make(map[*websocket.Conn]string, len(h.clients))
	for conn, filter := range h.clients {
		clientsCopy[conn] = filter
	}
	h.mutex.RUnlock()

	jsonData, err := json.Marshal(message)
	if err != nil {
		log.Printf("Failed to marshal broadcast message: %v", err)
		return
	}

	var failedClients []*websocket.Conn
	successCount := 0

	for client, filter := range clientsCopy {
		if filter != "" && !matchesFilter(message.Type, filter) {
			continue
		}

		client.SetWriteDeadline(time.Now().Add(2 * time.Second))

		err := client.WriteMessage(websocket.TextMessage, jsonData)
		if err != nil {
			log.Printf("Broadcast error to client (filter: %s): %v", filter, err)
			failedClients = append(failedClients, client)
		} else {
			successCount++
		}
	}

	if message.Type != "metrics" {
		log.Printf("Broadcast '%s' to %d clients", message.Type, successCount)
	}

	if len(failedClients) > 0 {
		h.mutex.Lock()
		for _, client := range failedClients {
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				client.Close()
			}
		}
		h.mutex.Unlock()
		log.Printf("Removed %d failed clients", len(failedClients))
	}
}


func matchesFilter(messageType, filter string) bool {
	if filter == "" {
		return true
	}

	filters := strings.Split(filter, ",")
	for _, f := range filters {
		if strings.TrimSpace(f) == messageType {
			return true
		}
	}

	return false
}


func (h *Hub) AddSessionRecord(clientKey string, duration int64) int64 {
	now := time.Now()
	sevenDaysAgo := now.Add(-7 * 24 * time.Hour)

	h.mutex.Lock()
	defer h.mutex.Unlock()

	recs := h.weeklyRecords[clientKey]

	recs = append(recs, SessionRecord{
		Timestamp: now,
		Duration:  duration,
	})

	var pruned []SessionRecord
	var total int64
	for _, r := range recs {
		if r.Timestamp.After(sevenDaysAgo) {
			pruned = append(pruned, r)
			total += r.Duration
		}
	}

	h.weeklyRecords[clientKey] = pruned

	return total
}

func (h *Hub) GetWeeklyTotal(clientKey string) int64 {
	now := time.Now()
	sevenDaysAgo := now.Add(-7 * 24 * time.Hour)

	h.mutex.RLock()
	defer h.mutex.RUnlock()

	var total int64
	for _, r := range h.weeklyRecords[clientKey] {
		if r.Timestamp.After(sevenDaysAgo) {
			total += r.Duration
		}
	}

	return total
}

func (h *Hub) GetAllWeeklyTotals() map[string]int64 {
	now := time.Now()
	sevenDaysAgo := now.Add(-7 * 24 * time.Hour)

	h.mutex.RLock()
	defer h.mutex.RUnlock()

	totals := make(map[string]int64)
	for clientKey, records := range h.weeklyRecords {
		var total int64
		for _, r := range records {
			if r.Timestamp.After(sevenDaysAgo) {
				total += r.Duration
			}
		}
		if total > 0 {
			totals[clientKey] = total
		}
	}

	return totals
}

func (h *Hub) GetClientInfo() map[string]string {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	info := make(map[string]string)
	for conn, filter := range h.clients {
		info[conn.RemoteAddr().String()] = filter
	}

	return info
}

func (h *Hub) GetClientCount() int {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	return len(h.clients)
}