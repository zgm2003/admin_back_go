package queuemonitor

import (
	"errors"
	"html/template"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	"admin_back_go/internal/config"
	"admin_back_go/internal/module/queuemonitor/asynqmonui"
	"admin_back_go/internal/platform/taskqueue"

	"github.com/hibiken/asynqmon"
)

const (
	// UIPath is the authenticated admin route where the official Asynq monitor
	// UI is mounted.
	UIPath = "/api/admin/v1/queue-monitor-ui"

	ErrUIUnavailable = "队列监控 UI 未配置"
)

// MonitorUI owns the official asynqmon HTTP handler. It is intentionally thin:
// queue dashboard behavior belongs to asynqmon, not to project hand-written UI.
type MonitorUI struct {
	handler *asynqmon.HTTPHandler
}

// NewMonitorUI builds the official Asynq monitor in read-only mode.
func NewMonitorUI(redisCfg config.RedisConfig, queueCfg config.QueueConfig) (*MonitorUI, error) {
	redisOpt, err := taskqueue.RedisConnOpt(redisCfg, queueCfg)
	if err != nil {
		return nil, err
	}
	handler := asynqmon.New(asynqmon.Options{
		RootPath:     UIPath,
		RedisConnOpt: redisOpt,
		ReadOnly:     true,
	})
	return &MonitorUI{handler: handler}, nil
}

func (m *MonitorUI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if m == nil || m.handler == nil {
		http.Error(w, ErrUIUnavailable, http.StatusServiceUnavailable)
		return
	}
	if isAsynqmonAPIRequest(r) {
		m.handler.ServeHTTP(w, r)
		return
	}
	serveAsynqmonUI(w, r)
}

func (m *MonitorUI) Close() error {
	if m == nil || m.handler == nil {
		return nil
	}
	return m.handler.Close()
}

// RootPath returns the mount path without a trailing slash.
func (m *MonitorUI) RootPath() string {
	if m == nil || m.handler == nil {
		return UIPath
	}
	root := strings.TrimSuffix(m.handler.RootPath(), "/")
	if root == "" {
		return UIPath
	}
	return root
}

// IsUIConfigError reports whether monitor construction failed because queue
// Redis config is absent. Other errors should remain visible in logs.
func IsUIConfigError(err error) bool {
	return errors.Is(err, taskqueue.ErrRedisAddrRequired)
}

func isAsynqmonAPIRequest(r *http.Request) bool {
	if r == nil || r.URL == nil {
		return false
	}
	apiRoot := strings.TrimSuffix(UIPath, "/") + "/api"
	return r.URL.Path == apiRoot || strings.HasPrefix(r.URL.Path, apiRoot+"/")
}

func serveAsynqmonUI(w http.ResponseWriter, r *http.Request) {
	if r == nil || r.URL == nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	assetPath, ok := asynqmonAssetPath(r.URL.Path)
	if !ok {
		http.Error(w, "unexpected path prefix", http.StatusBadRequest)
		return
	}
	if assetPath == "" || assetPath == "index.html" {
		renderAsynqmonIndex(w)
		return
	}

	filePath := path.Join("build", assetPath)
	bytes, err := asynqmonui.Build.ReadFile(filePath)
	if err != nil {
		var pathErr *fs.PathError
		if errors.As(err, &pathErr) {
			renderAsynqmonIndex(w)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	contentType := mime.TypeByExtension(path.Ext(filePath))
	if contentType == "" {
		contentType = http.DetectContentType(bytes)
	}
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(bytes)
}

func asynqmonAssetPath(requestPath string) (string, bool) {
	root := strings.TrimSuffix(UIPath, "/")
	if requestPath != root && !strings.HasPrefix(requestPath, root+"/") {
		return "", false
	}
	assetPath := strings.TrimPrefix(requestPath, root)
	assetPath = strings.TrimPrefix(assetPath, "/")
	assetPath = path.Clean("/" + assetPath)
	assetPath = strings.TrimPrefix(assetPath, "/")
	if assetPath == "." {
		return "", true
	}
	return assetPath, true
}

func renderAsynqmonIndex(w http.ResponseWriter) {
	tmpl, err := template.New("index.html").Delims("/[[", "]]").ParseFS(asynqmonui.Build, "build/index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := struct {
		RootPath       string
		PrometheusAddr string
		ReadOnly       bool
	}{
		RootPath: UIPath,
		ReadOnly: true,
	}
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
