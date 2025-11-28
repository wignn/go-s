package main

import (
	"time"

	"github.com/gorilla/websocket"
)

type CodingSession struct {
	DurationSeconds int64   `json:"duration_seconds"`
	Editor          string  `json:"editor"`
	Project         string  `json:"project"`
	Language        string  `json:"language"`
	FilePath        *string `json:"file_path,omitempty"`
	Timestamp       string  `json:"timestamp"`
	LinesOfCode     *int    `json:"lines_of_code,omitempty"`
}
type SystemMetrics struct {
	CPU       float64 `json:"cpu"`
	CPUModel  string  `json:"cpu_model"`
	Cores     int     `json:"cores"`
	Memory    float64 `json:"memory"`
	TotalMem  uint64  `json:"total_mem"`
	UsedMem   uint64  `json:"used_mem"`
	OS        string  `json:"os"`
	Platform  string  `json:"platform"`
	Kernel    string  `json:"kernel"`
	Arch      string  `json:"arch"`
	Uptime    uint64  `json:"uptime"`
	Timestamp string  `json:"timestamp"`
}

type BroadcastMessage struct {
	Type    string      `json:"type"`
	Data    interface{} `json:"data"`
	EventID string      `json:"event_id"`
}

// internal record for tracking session durations per timestamp
type SessionRecord struct {
	Timestamp time.Time `json:"timestamp"`
	Duration  int64     `json:"duration"`
}

// WeeklySummary sent to monitor/external clients
type WeeklySummary struct {
	Client      string `json:"client"`
	WeekSeconds int64  `json:"week_seconds"`
}

// Subscription represents a client subscribing to hub broadcasts with an optional filter
type Subscription struct {
	Conn   *websocket.Conn
	Filter string // empty = all, otherwise "metrics" or "session" or "weekly_summary"
}
