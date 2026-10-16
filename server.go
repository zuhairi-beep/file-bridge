package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"
)

// FileServer manages the HTTP server for file uploads
type FileServer struct {
	httpServers []*http.Server
	listeners   []net.Listener
	port        int
	aliasPorts  []int
	lanIP       string
	running     bool
	portWarning string
	app         *App
}

// NewFileServer creates a new FileServer instance
func NewFileServer(app *App) *FileServer {
	return &FileServer{app: app}
}

// Start starts the HTTP server on an available port
func (fs *FileServer) Start() error {
	if fs.running {
		return fmt.Errorf("server is already running")
	}

	// Find LAN IP
	ip, err := getLANIP()
	if err != nil {
		return fmt.Errorf("failed to get LAN IP: %w", err)
	}
	fs.lanIP = ip

	ensureShortcutToken(fs.app.config)

	preferred := fs.app.config.Port
	if preferred == 0 {
		preferred = defaultPort
	}
	fs.portWarning = ""

	nonceBytes := make([]byte, 8)
	rand.Read(nonceBytes)
	nonce := hex.EncodeToString(nonceBytes)
	mux := fs.buildMux(nonce)

	tryPorts := []int{preferred}
	if preferred != defaultPort {
		tryPorts = append(tryPorts, defaultPort)
	}

	var bound []int
	for _, p := range tryPorts {
		if err := fs.bindPort(mux, p, nonce); err != nil {
			if isFileBridgeAt(fs.lanIP, p) {
				return fmt.Errorf("another File Bridge is already listening on port %d — close the other window or end its process in Task Manager", p)
			}
			if p == preferred {
				if fs.app.config.AllowEphemeralPort {
					continue
				}
				return fmt.Errorf("port %d is in use by another program: %v", p, err)
			}
			log.Printf("Optional alias port %d not available: %v", p, err)
			continue
		}
		bound = append(bound, p)
	}

	if len(bound) == 0 && fs.app.config.AllowEphemeralPort {
		if err := fs.bindPort(mux, 0, nonce); err != nil {
			return fmt.Errorf("failed to start upload server: %w", err)
		}
		bound = append(bound, fs.port)
		fs.portWarning = fmt.Sprintf("Test mode: using ephemeral port %d instead of %d.", fs.port, preferred)
	}

	if len(bound) == 0 {
		return fmt.Errorf("could not bind upload server on port %d", preferred)
	}

	fs.port = preferred
	if containsInt(bound, defaultPort) {
		fs.port = defaultPort
	} else if !containsInt(bound, preferred) {
		fs.port = bound[0]
	}
	for _, p := range bound {
		if p != fs.port {
			fs.aliasPorts = append(fs.aliasPorts, p)
		}
	}
	if len(fs.aliasPorts) > 0 {
		fs.portWarning = fmt.Sprintf("Also listening on port(s) %v for older Shortcut URLs.", fs.aliasPorts)
	}

	fs.running = true
	log.Printf("Upload URL: http://%s:%d/upload", fs.lanIP, fs.port)
	return nil
}

func containsInt(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// Stop gracefully stops the HTTP server
func (fs *FileServer) Stop() error {
	if !fs.running {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var err error
	for _, srv := range fs.httpServers {
		if e := srv.Shutdown(ctx); e != nil {
			err = e
		}
	}
	for _, ln := range fs.listeners {
		ln.Close()
	}
	fs.httpServers = nil
	fs.listeners = nil
	fs.running = false
	log.Println("HTTP server stopped")
	return err
}

// GetUploadURL returns the upload URL with language parameter
func (fs *FileServer) GetUploadURL() string {
	if !fs.running {
		return ""
	}
	lang := fs.app.GetLang()
	return fmt.Sprintf("http://%s:%d/upload?lang=%s", fs.lanIP, fs.port, lang)
}

func (fs *FileServer) buildMux(nonce string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/upload", fs.handleUploadPage)
	mux.HandleFunc("/upload/sha256.js", fs.handleUploadSha256JS)
	mux.HandleFunc("/api/upload", fs.handleFileUpload)
	mux.HandleFunc("/api/shortcut/upload", fs.handleShortcutUpload)
	mux.HandleFunc("/api/shortcut/info", fs.handleShortcutInfo)
	mux.HandleFunc("/api/shortcut/session/create", fs.handleShortcutSessionCreate)
	mux.HandleFunc("/api/shortcut/session/register", fs.handleShortcutSessionRegister)
	mux.HandleFunc("/api/shortcut/session/check", fs.handleShortcutSessionCheck)
	mux.HandleFunc("/api/shortcut/session/progress", fs.handleShortcutSessionProgress)
	mux.HandleFunc("/api/shortcut/session/fail", fs.handleShortcutSessionFail)
	mux.HandleFunc("/shortcut/session", fs.handleShortcutSessionPage)
	mux.HandleFunc("/api/ping", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"app": "file-bridge", "nonce": nonce})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			fs.handleUploadPage(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, notFoundPage)
	})
	return mux
}

func (fs *FileServer) bindPort(mux *http.ServeMux, port int, nonce string) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	actual := ln.Addr().(*net.TCPAddr).Port
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error on :%d: %v", actual, err)
		}
	}()
	if err := selfCheck(fmt.Sprintf("http://%s:%d/api/ping", fs.lanIP, actual), nonce); err != nil {
		srv.Close()
		ln.Close()
		return err
	}
	fs.httpServers = append(fs.httpServers, srv)
	fs.listeners = append(fs.listeners, ln)
	if fs.port == 0 {
		fs.port = actual
	}
	log.Printf("HTTP server listening on 0.0.0.0:%d (verified via %s)", actual, fs.lanIP)
	return nil
}

func selfCheck(url, nonce string) error {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("self-check failed: %w", err)
	}
	defer resp.Body.Close()
	var body struct{ Nonce string }
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil || body.Nonce != nonce {
		return fmt.Errorf("another program answered on this port (HTTP %d)", resp.StatusCode)
	}
	return nil
}

// GetPortWarning returns a message if the preferred port was taken
func (fs *FileServer) GetPortWarning() string { return fs.portWarning }

// GetShortcutURL returns the endpoint used by the iOS Shortcut
func (fs *FileServer) GetShortcutURL() string {
	if !fs.running {
		return ""
	}
	return fmt.Sprintf("http://%s:%d/api/shortcut/upload", fs.lanIP, fs.port)
}

// GetPort returns the server port
func (fs *FileServer) GetPort() int {
	return fs.port
}

// GetLANIP returns the LAN IP address
func (fs *FileServer) GetLANIP() string {
	return fs.lanIP
}

// IsRunning returns whether the server is running
func (fs *FileServer) IsRunning() bool {
	return fs.running
}

// getLANIP finds the best LAN IP address
func getLANIP() (string, error) {
	ips, err := getLANIPs()
	if err != nil {
		return "", err
	}
	return ips[0], nil
}

// defaultRouteIP returns the local IPv4 address the OS would use to reach the
// internet, i.e. the adapter with the default gateway. UDP "dial" sends no packets.
func defaultRouteIP() string {
	c, err := net.Dial("udp4", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).IP.String()
}

// getLANIPs returns candidate LAN IPv4 addresses, best first: the default
// gateway adapter, then Wi-Fi, then 192.168/10/172.16-31 ranges. Virtual,
// VPN, WSL, Hyper-V and Docker adapters are skipped.
func getLANIPs() ([]string, error) {
	routeIP := defaultRouteIP()
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	type ipCandidate struct {
		ip       string
		priority int
	}

	var candidates []ipCandidate

	for _, iface := range ifaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		// Skip virtual/VPN adapters a phone on Wi-Fi cannot reach
		if isVirtualAdapter(iface.Name) {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			// Only IPv4
			if ip == nil || ip.To4() == nil {
				continue
			}

			ipStr := ip.String()
			priority := 0

			// Prioritize private IP ranges
			if strings.HasPrefix(ipStr, "192.168.") {
				priority = 3
			} else if strings.HasPrefix(ipStr, "10.") {
				priority = 2
			} else if strings.HasPrefix(ipStr, "172.") {
				// Check 172.16.0.0 - 172.31.255.255
				parts := strings.Split(ipStr, ".")
				if len(parts) >= 2 {
					secondOctet := 0
					fmt.Sscanf(parts[1], "%d", &secondOctet)
					if secondOctet >= 16 && secondOctet <= 31 {
						priority = 1
					}
				}
			}

			if priority > 0 {
				// Prefer Wi-Fi interfaces
				nameLower := strings.ToLower(iface.Name)
				if strings.Contains(nameLower, "wi-fi") || strings.Contains(nameLower, "wifi") || strings.Contains(nameLower, "wlan") || strings.Contains(nameLower, "wireless") {
					priority += 10
				}
				candidates = append(candidates, ipCandidate{ip: ipStr, priority: priority})
			}
		}
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no LAN IP address found")
	}
	for i := range candidates {
		if candidates[i].ip == routeIP {
			candidates[i].priority += 100
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].priority > candidates[j].priority
	})
	ips := make([]string, len(candidates))
	for i, c := range candidates {
		ips[i] = c.ip
	}
	return ips, nil
}

// SetLANIP switches the address shown in the QR code (the server already
// listens on all interfaces, so no restart is needed).
func (fs *FileServer) SetLANIP(ip string) error {
	ips, _ := getLANIPs()
	for _, c := range ips {
		if c == ip {
			fs.lanIP = ip
			return nil
		}
	}
	return fmt.Errorf("unknown LAN IP %s", ip)
}

// isVirtualAdapter reports adapters created by WSL, Hyper-V, VMs, Docker and VPNs.
func isVirtualAdapter(name string) bool {
	n := strings.ToLower(name)
	for _, k := range []string{"vethernet", "wsl", "hyper-v", "virtualbox", "vmware", "vmnet", "docker", "br-", "veth", "tailscale", "zerotier", "wireguard", "openvpn", "tap", "tun", "utun", "loopback"} {
		if strings.Contains(n, k) {
			return true
		}
	}
	return false
}

const notFoundPage = `<!DOCTYPE html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>File Bridge</title></head>
<body style="font-family:-apple-system,sans-serif;background:#0f172a;color:#e2e8f0;text-align:center;padding:40px 20px">
<h1 style="color:#38bdf8">File Bridge</h1><p>This page does not exist, but File Bridge is running here.</p>
<p><a href="/" style="color:#38bdf8;font-size:1.2rem">Open the upload page</a></p></body></html>`
