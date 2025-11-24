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

func main() {
        http.HandleFunc("/ws", wsHandler)

        log.Println("WebSocket running at ws://localhost:8081/ws")
        if err := http.ListenAndServe(":8081", nil); err != nil {
                log.Fatal(err)
        }
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
        conn, err := upgrader.Upgrade(w, r, nil)
        if err != nil {
                log.Println("Upgrade error:", err)
                return
        }
        defer conn.Close()

        for {
                // CPU USAGE
                cpuPercent, err := cpu.Percent(0, false)
                if err != nil || len(cpuPercent) == 0 {
                        continue
                }

                // CPU INFO
                cpuInfo, _ := cpu.Info() // includes model name, etc.
                coreCount, _ := cpu.Counts(true)

                // MEMORY INFO
                memStat, _ := mem.VirtualMemory()

                // SYSTEM / OS INFO
                hostInfo, _ := host.Info()

                data := map[string]interface{}{
                        // CPU
                        "cpu":        cpuPercent[0],
                        "cpuModel":   cpuInfo[0].ModelName,
                        "cores":      coreCount,

                        // MEMORY
                        "memory":     memStat.UsedPercent,
                        "totalMem":   memStat.Total / 1024 / 1024 / 1024, // GB
                        "usedMem":    memStat.Used / 1024 / 1024 / 1024,  // GB

                        // OS INFO
                        "os":         hostInfo.OS,         // "windows", "linux", "darwin"
                        "platform":   hostInfo.Platform,   // e.g. "Windows 10 Pro"
                        "kernel":     hostInfo.KernelVersion,
                        "arch":       hostInfo.KernelArch, // amd64 / arm64

                        // UPTIME
                        "uptime":     hostInfo.Uptime,     // seconds
                }

                jsonBytes, _ := json.Marshal(data)

                if err := conn.WriteMessage(websocket.TextMessage, jsonBytes); err != nil {
                        log.Println("Write error:", err)
                        return
                }

                time.Sleep(time.Second)
        }
}