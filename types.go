package main

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
