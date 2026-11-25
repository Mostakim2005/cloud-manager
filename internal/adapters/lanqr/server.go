package lanqr

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"html"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Mostakim2005/cloud-manager/internal/config"
	"github.com/Mostakim2005/cloud-manager/internal/ingest"
	"github.com/Mostakim2005/cloud-manager/internal/qrsvg"
	"github.com/Mostakim2005/cloud-manager/internal/security"
)

// Server implements the opt-in HTTPS TLS Browser QR Drop-Zone on :53317 (OFF by default).
type Server struct {
	mu          sync.Mutex
	enabled     bool
	port        int
	pin         string
	lanIP       string
	autoRoute   bool
	failedPINs  int
	lockedUntil time.Time
	srv         *http.Server
	ln          net.Listener
	Pipeline    *ingest.Pipeline
	Settings    func() config.Settings
	StagedDir   string
}

// Status captures current Drop-Zone state for UI & CLI inspection.
type Status struct {
	Enabled   bool   `json:"enabled"`
	URL       string `json:"url,omitempty"`
	PIN       string `json:"pin,omitempty"`
	Port      int    `json:"port"`
	AutoRoute bool   `json:"autoRoute"`
	QRSVG     string `json:"qrSvg,omitempty"`
}

// NewServer creates a disabled-by-default Browser QR Drop-Zone server.
func NewServer(port int, stagedDir string, pipe *ingest.Pipeline, settingsFn func() config.Settings) *Server {
	if port <= 0 {
		port = config.DefaultLANPort
	}
	return &Server{
		port:      port,
		StagedDir: stagedDir,
		Pipeline:  pipe,
		Settings:  settingsFn,
		autoRoute: true,
	}
}

// Enable starts the ephemeral HTTPS TLS Drop-Zone listener with a fresh 6-digit PIN.
func (s *Server) Enable() (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.enabled && s.srv != nil {
		return s.statusLocked(), nil
	}

	pin, err := randomSixDigitPIN()
	if err != nil {
		return Status{}, err
	}
	tlsCfg, err := ephemeralTLSConfig()
	if err != nil {
		return Status{}, err
	}
	addr := fmt.Sprintf("0.0.0.0:%d", s.port)
	ln, err := tls.Listen("tcp", addr, tlsCfg)
	if err != nil {
		return Status{}, fmt.Errorf("listen LAN QR %s: %w", addr, err)
	}

	s.pin = pin
	s.lanIP = detectLANIP()
	s.enabled = true
	s.failedPINs = 0
	s.lockedUntil = time.Time{}
	if s.Settings != nil {
		s.autoRoute = s.Settings().AutoRouteLAN
	}
	s.ln = ln
	s.srv = &http.Server{
		Handler:           s.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		_ = s.srv.Serve(ln)
	}()
	return s.statusLocked(), nil
}

// Disable shuts down the LAN Drop-Zone listener and wipes the session PIN.
func (s *Server) Disable(ctx context.Context) error {
	s.mu.Lock()
	srv := s.srv
	s.srv = nil
	s.ln = nil
	s.enabled = false
	s.pin = ""
	s.mu.Unlock()
	if srv != nil {
		return srv.Shutdown(ctx)
	}
	return nil
}

// Status returns current Drop-Zone availability, URL, PIN, and pairing QR SVG.
func (s *Server) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked()
}

func (s *Server) statusLocked() Status {
	st := Status{
		Enabled:   s.enabled,
		Port:      s.port,
		AutoRoute: s.autoRoute,
	}
	if s.enabled {
		actualPort := s.port
		if s.ln != nil {
			if tcpAddr, ok := s.ln.Addr().(*net.TCPAddr); ok {
				actualPort = tcpAddr.Port
			}
		}
		st.Port = actualPort
		st.PIN = s.pin
		st.URL = fmt.Sprintf("https://%s:%d/?pin=%s", s.lanIP, actualPort, s.pin)
		if svg, err := qrsvg.RenderSVG(st.URL, 200); err == nil {
			st.QRSVG = svg
		}
	}
	return st
}

// SetAutoRoute configures whether incoming phone files queue immediately for cloud routing.
func (s *Server) SetAutoRoute(v bool) {
	s.mu.Lock()
	s.autoRoute = v
	s.mu.Unlock()
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleDropPage)
	mux.HandleFunc("/upload", s.handleUpload)
	mux.HandleFunc("/staged", s.handleListStaged)
	mux.HandleFunc("/download", s.handleDownload)
	return mux
}

func (s *Server) checkPIN(r *http.Request) (bool, int) {
	pin := strings.TrimSpace(r.Header.Get("X-Drop-PIN"))
	if pin == "" {
		pin = strings.TrimSpace(r.URL.Query().Get("pin"))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.enabled {
		return false, http.StatusServiceUnavailable
	}
	if !s.lockedUntil.IsZero() && time.Now().Before(s.lockedUntil) {
		return false, http.StatusTooManyRequests
	}
	if pin == "" || pin != s.pin {
		s.failedPINs++
		if s.failedPINs >= 5 {
			s.lockedUntil = time.Now().Add(60 * time.Second)
		}
		return false, http.StatusUnauthorized
	}
	s.failedPINs = 0
	return true, http.StatusOK
}

func (s *Server) handleDropPage(w http.ResponseWriter, r *http.Request) {
	ok, code := s.checkPIN(r)
	if !ok {
		http.Error(w, "Valid 6-digit pairing PIN required", code)
		return
	}
	pin := html.EscapeString(r.URL.Query().Get("pin"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Cloud Manager QR Drop-Zone</title><style>body{font-family:system-ui,sans-serif;max-width:560px;margin:24px auto;padding:0 16px;background:#0b0f17;color:#f1f5f9}button,input{font:inherit;padding:10px 14px;border-radius:10px;border:1px solid #334155;background:#192234;color:#f1f5f9}button{background:#93c5fd;color:#0b1220;font-weight:600;border:none;cursor:pointer}.card{background:#121826;border:1px solid #334155;border-radius:14px;padding:16px;margin:16px 0}</style></head><body><h1>Cloud Manager Drop-Zone</h1><div class="card"><h2>Send Phone -> PC</h2><form method="POST" action="/upload?pin=%s" enctype="multipart/form-data"><input type="file" name="file" required> <button type="submit">Send to PC</button></form></div><div class="card"><h2>Pull PC -> Phone</h2><div id="files">Loading...</div></div><script>fetch('/staged?pin=%s').then(r=>r.json()).then(d=>{document.getElementById('files').innerHTML=(d.files||[]).map(f=>'<div><a style="color:#93c5fd" href="/download?pin=%s&file='+encodeURIComponent(f)+'">'+f+'</a></div>').join('')||'No staged files.';});</script></body></html>`, pin, pin, pin)
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ok, code := s.checkPIN(r)
	if !ok {
		http.Error(w, "Unauthorized PIN", code)
		return
	}
	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "Multipart form required", http.StatusBadRequest)
		return
	}
	var results []any
	s.mu.Lock()
	autoRoute := s.autoRoute
	s.mu.Unlock()

	for {
		part, err := mr.NextPart()
		if err != nil {
			break
		}
		if part.FileName() == "" {
			continue
		}
		relName := filepath.Base(part.FileName())
		if _, serr := security.SanitizeRelPath(relName); serr != nil {
			_ = part.Close()
			http.Error(w, "Unsafe filename rejected", http.StatusBadRequest)
			return
		}
		res, ierr := s.Pipeline.Ingest(r.Context(), ingest.Request{
			RelPath:      filepath.ToSlash(filepath.Join("LAN_DropZone", relName)),
			SourceReader: part,
			OriginType:   "lan-qr",
			AutoQueue:    autoRoute,
		})
		_ = part.Close()
		if ierr != nil {
			http.Error(w, ierr.Error(), http.StatusInsufficientStorage)
			return
		}
		results = append(results, res)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"uploaded": results})
}

func (s *Server) handleListStaged(w http.ResponseWriter, r *http.Request) {
	ok, code := s.checkPIN(r)
	if !ok {
		http.Error(w, "Unauthorized PIN", code)
		return
	}
	var names []string
	entries, _ := os.ReadDir(s.StagedDir)
	for _, e := range entries {
		if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			names = append(names, e.Name())
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"files": names})
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	ok, code := s.checkPIN(r)
	if !ok {
		http.Error(w, "Unauthorized PIN", code)
		return
	}
	rel := r.URL.Query().Get("file")
	target, err := security.ResolveConfinedPath(s.StagedDir, rel)
	if err != nil {
		http.Error(w, "Unsafe file path", http.StatusBadRequest)
		return
	}
	http.ServeFile(w, r, target)
}

func randomSixDigitPIN() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func detectLANIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}
	return "127.0.0.1"
}

func ephemeralTLSConfig() (*tls.Config, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	if err != nil {
		return nil, err
	}
	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "Cloud Manager LAN Drop-Zone"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		return nil, err
	}
	cert := tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  priv,
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
	}, nil
}
