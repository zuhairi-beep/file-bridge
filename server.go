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
	server  *http.Server
	port    int
	lanIP   string
	running bool
	// portWarning is set when the preferred port was taken
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

	// Preferred fixed port (stable URL for iOS Shortcuts). If it is busy, or
	// another program answers on it for our LAN IP, fall back to a random free
	// port. The QR code always shows the port we really serve on.
	port := fs.app.config.Port
	if port == 0 {
		port = defaultPort
	}
	fs.portWarning = ""
	if err := fs.listenAndVerify(port); err != nil {
		log.Printf("Port %d not usable: %v; using a random free port", port, err)
		fs.portWarning = fmt.Sprintf("Port %d is used by another program; using port %%d instead. Update your iOS Shortcut URL.", port)
		if err := fs.listenAndVerify(0); err != nil {
			return fmt.Errorf("failed to start upload server: %w", err)
		}
		fs.portWarning = fmt.Sprintf(fs.portWarning, fs.port)
	}

	fs.running = true
	log.Printf("Upload URL: http://%s:%d/upload", fs.lanIP, fs.port)
	return nil
}

// Stop gracefully stops the HTTP server
func (fs *FileServer) Stop() error {
	if !fs.running || fs.server == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := fs.server.Shutdown(ctx)
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

// listenAndVerify binds 0.0.0.0:port, starts serving on that same listener
// (no close-and-rebind race), then calls its own /api/ping through the LAN IP
// and checks a per-run nonce. On Windows a program bound to the specific LAN
// IP and port wins over our wildcard bind without any error, so the self-check
// is the only reliable way to know the phone will reach File Bridge.
func (fs *FileServer) listenAndVerify(port int) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	nonceBytes := make([]byte, 8)
	rand.Read(nonceBytes)
	nonce := hex.EncodeToString(nonceBytes)

	mux := http.NewServeMux()
	mux.HandleFunc("/upload", fs.handleUploadPage)
	mux.HandleFunc("/api/upload", fs.handleFileUpload)
	mux.HandleFunc("/api/shortcut/upload", fs.handleShortcutUpload)
	mux.HandleFunc("/api/shortcut/session/create", fs.handleShortcutSessionCreate)
	mux.HandleFunc("/api/shortcut/session/register", fs.handleShortcutSessionRegister)
	mux.HandleFunc("/api/shortcut/session/check", fs.handleShortcutSessionCheck)
	mux.HandleFunc("/api/shortcut/session/progress", fs.handleShortcutSessionProgress)
	mux.HandleFunc("/api/shortcut/session/fail", fs.handleShortcutSessionFail)
	mux.HandleFunc("/shortcut/session", fs.handleShortcutSessionPage)
	mux.HandleFunc("/api/ping", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"app": "file-bridge", "nonce": nonce})
	})
	// "/" serves the upload page too; any other path gets a friendly page
	// with a link back instead of a bare 404.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			fs.handleUploadPage(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, notFoundPage)
	})

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		// No ReadTimeout/WriteTimeout to allow large file uploads
		MaxHeaderBytes: 1 << 20, // 1MB
	}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()
	actual := ln.Addr().(*net.TCPAddr).Port

	if err := selfCheck(fmt.Sprintf("http://%s:%d/api/ping", fs.lanIP, actual), nonce); err != nil {
		srv.Close()
		return err
	}
	fs.server = srv
	fs.port = actual
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
