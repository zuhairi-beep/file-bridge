package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// isFileBridgeAt reports whether something answering on ip:port is File Bridge.
func isFileBridgeAt(ip string, port int) bool {
	if port <= 0 || port > 65535 {
		return false
	}
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	url := fmt.Sprintf("http://%s:%d/api/ping", ip, port)
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var body struct {
		App string `json:"app"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return false
	}
	return body.App == "file-bridge"
}
