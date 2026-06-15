package webui

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/diagnostics"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
)

//go:embed static/*
var staticFiles embed.FS

type Options struct {
	ListenAddr    string
	EndpointsPath string
	Version       string
}

type statusResponse struct {
	Product       string `json:"product"`
	Version       string `json:"version"`
	EndpointsPath string `json:"endpoints_path"`
	Mode          string `json:"mode"`
}

type endpointSummary struct {
	Total      int `json:"total"`
	Production int `json:"production"`
	Dev        int `json:"dev"`
	Groups     int `json:"groups"`
}

type endpointsResponse struct {
	GeneratedAt   string            `json:"generated_at"`
	EndpointsPath string            `json:"endpoints_path"`
	Summary       endpointSummary   `json:"summary"`
	Items         []models.Endpoint `json:"items"`
}

func Start(ctx context.Context, opts Options) error {
	handler, err := newHandler(opts)
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", opts.ListenAddr)
	if err != nil {
		return err
	}

	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	diagnostics.LogInfo("web ui listening", map[string]interface{}{
		"listen_addr":    listener.Addr().String(),
		"endpoints_path": opts.EndpointsPath,
	})

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			diagnostics.LogError("web ui shutdown failed", err, nil)
		}
	}()

	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			diagnostics.LogError("web ui serve failed", err, map[string]interface{}{
				"listen_addr": listener.Addr().String(),
			})
		}
	}()

	return nil
}

func newHandler(opts Options) (http.Handler, error) {
	staticRoot, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, statusResponse{
			Product:       "Ping Monitor",
			Version:       opts.Version,
			EndpointsPath: opts.EndpointsPath,
			Mode:          "read-only",
		})
	})
	mux.HandleFunc("/api/endpoints", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		endpoints, err := config.LoadEndpoints(opts.EndpointsPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusOK, endpointsResponse{
			GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
			EndpointsPath: opts.EndpointsPath,
			Summary:       summarizeEndpoints(endpoints),
			Items:         endpoints,
		})
	})
	mux.Handle("/", staticHandler(staticRoot))

	return mux, nil
}

func summarizeEndpoints(endpoints []models.Endpoint) endpointSummary {
	groups := make(map[string]struct{})
	summary := endpointSummary{Total: len(endpoints)}
	for _, endpoint := range endpoints {
		group := strings.TrimSpace(endpoint.Group)
		if group == "" {
			group = "default"
		}
		groups[group] = struct{}{}
		if endpoint.Dev {
			summary.Dev++
			continue
		}
		summary.Production++
	}
	summary.Groups = len(groups)
	return summary
}

func staticHandler(root fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleanPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if cleanPath == "." || cleanPath == "" {
			serveIndex(w, r, fileServer)
			return
		}
		if _, err := fs.Stat(root, cleanPath); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		serveIndex(w, r, fileServer)
	})
}

func serveIndex(w http.ResponseWriter, r *http.Request, fileServer http.Handler) {
	b, err := fs.ReadFile(staticFiles, "static/index.html")
	if err != nil {
		http.Error(w, "ui shell unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(b))
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	b, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, "json encode failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

func sortedGroups(endpoints []models.Endpoint) []string {
	seen := make(map[string]struct{})
	for _, endpoint := range endpoints {
		group := strings.TrimSpace(endpoint.Group)
		if group == "" {
			group = "default"
		}
		seen[group] = struct{}{}
	}
	groups := make([]string, 0, len(seen))
	for group := range seen {
		groups = append(groups, group)
	}
	sort.Strings(groups)
	return groups
}
