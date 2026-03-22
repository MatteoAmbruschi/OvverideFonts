package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
)

// Port is the fixed localhost port used by the Chrome extension to poll font state.
const Port = 59842

// Server serves font state to the Chrome extension and font files for @font-face injection.
type Server struct {
	mu           sync.RWMutex
	font         string
	active       bool
	srv          *http.Server
	fontAssets   embed.FS
	bundledFonts map[string]string // display name -> embed.FS path
}

func New(fontAssets embed.FS) *Server {
	s := &Server{fontAssets: fontAssets}
	s.bundledFonts = buildFontMap(fontAssets)
	return s
}

// buildFontMap scans assets/fonts/ from the embed.FS and returns displayName -> embed path.
func buildFontMap(fsys embed.FS) map[string]string {
	m := make(map[string]string)
	entries, err := fs.ReadDir(fsys, "assets/fonts")
	if err != nil {
		return m
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		switch strings.ToLower(filepath.Ext(name)) {
		case ".ttf", ".otf", ".woff2", ".woff":
			m[bundledDisplayName(name)] = "assets/fonts/" + name
		}
	}
	return m
}

func bundledDisplayName(filename string) string {
	name := strings.TrimSuffix(filename, filepath.Ext(filename))
	for _, suf := range []string{
		"-Regular", "_Regular", " Regular",
		"-Bold", "-Italic", "-Light", "-Medium", "-SemiBold", "-Thin",
	} {
		if strings.HasSuffix(name, suf) {
			name = name[:len(name)-len(suf)]
			break
		}
	}
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	return strings.TrimSpace(name)
}

// BundledFontNames returns display names of all bundled fonts.
func (s *Server) BundledFontNames() []string {
	names := make([]string, 0, len(s.bundledFonts))
	for n := range s.bundledFonts {
		names = append(names, n)
	}
	return names
}

// BundledFontFilename returns the bare filename for a bundled display name, or "".
func (s *Server) BundledFontFilename(displayName string) string {
	path := s.bundledFonts[displayName]
	if path == "" {
		return ""
	}
	return filepath.Base(path)
}

func (s *Server) SetFont(font string, active bool) {
	s.mu.Lock()
	s.font, s.active = font, active
	s.mu.Unlock()
}

func (s *Server) GetFont() (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.font, s.active
}

func (s *Server) Start() {
	mux := http.NewServeMux()
	mux.HandleFunc("/font", s.handleFont)
	mux.HandleFunc("/fonts/", s.handleFontFile)
	s.srv = &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", Port),
		Handler: mux,
	}
	go func() { _ = s.srv.ListenAndServe() }()
}

func (s *Server) Stop() {
	if s.srv != nil {
		_ = s.srv.Close()
	}
}

func (s *Server) handleFont(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	s.mu.RLock()
	font, active := s.font, s.active
	s.mu.RUnlock()

	fontURL := ""
	if active && font != "" {
		if fname := s.BundledFontFilename(font); fname != "" {
			fontURL = fmt.Sprintf("http://127.0.0.1:%d/fonts/%s", Port, fname)
		}
	}

	_ = json.NewEncoder(w).Encode(struct {
		Font    string `json:"font"`
		Active  bool   `json:"active"`
		FontURL string `json:"fontUrl"`
	}{font, active, fontURL})
}

func (s *Server) handleFontFile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	filename := strings.TrimPrefix(r.URL.Path, "/fonts/")
	if filename == "" || strings.ContainsAny(filename, "/\\") || strings.Contains(filename, "..") {
		http.NotFound(w, r)
		return
	}
	data, err := s.fontAssets.ReadFile("assets/fonts/" + filename)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mimeForExt(filepath.Ext(filename)))
	_, _ = w.Write(data)
}

func mimeForExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".ttf":
		return "font/ttf"
	case ".otf":
		return "font/otf"
	case ".woff2":
		return "font/woff2"
	case ".woff":
		return "font/woff"
	default:
		return "application/octet-stream"
	}
}
