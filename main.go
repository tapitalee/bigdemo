package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

type PageData struct {
	EnvVars        []EnvVar
	DBStatus       StatusInfo
	RedisStatus    StatusInfo
	Uptime         string
	MemoryUsed     string
	ECSInfo        *ECSInfo
	ECSError       string
}

type EnvVar struct {
	Name  string
	Value string
}

type StatusInfo struct {
	Present   bool
	Connected bool
	Message   string
}

type ECSInfo struct {
	AvailabilityZone string `json:"AvailabilityZone"`
	Containers       []struct {
		ImageID string `json:"ImageID"`
		Name    string `json:"Name"`
	} `json:"Containers"`
}

func getEnvVars() []EnvVar {
	keys := []string{
		"TAP_DEPLOY_NUMBER",
		"TAP_DOCKER_TAG",
		"TAP_APP_URL",
		"TAP_APP_NAME",
		"TAP_TEAM_NAME",
	}
	vars := make([]EnvVar, 0, len(keys))
	for _, k := range keys {
		vars = append(vars, EnvVar{Name: k, Value: os.Getenv(k)})
	}
	return vars
}

func checkDB() StatusInfo {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return StatusInfo{Present: false, Message: "DATABASE_URL not set"}
	}

	var driver string
	if strings.HasPrefix(dbURL, "postgres") {
		driver = "postgres"
	} else if strings.HasPrefix(dbURL, "mysql") {
		driver = "mysql"
		// Convert mysql:// URL to DSN format if needed
		dbURL = strings.TrimPrefix(dbURL, "mysql://")
	} else {
		// Try to guess from content
		driver = "postgres"
	}

	db, err := sql.Open(driver, dbURL)
	if err != nil {
		return StatusInfo{Present: true, Connected: false, Message: fmt.Sprintf("Failed to open: %v", err)}
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return StatusInfo{Present: true, Connected: false, Message: fmt.Sprintf("Ping failed: %v", err)}
	}
	return StatusInfo{Present: true, Connected: true, Message: "Connected and responding"}
}

func checkRedis() StatusInfo {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		return StatusInfo{Present: false, Message: "REDIS_URL not set"}
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return StatusInfo{Present: true, Connected: false, Message: fmt.Sprintf("Invalid URL: %v", err)}
	}

	client := redis.NewClient(opts)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return StatusInfo{Present: true, Connected: false, Message: fmt.Sprintf("Ping failed: %v", err)}
	}
	return StatusInfo{Present: true, Connected: true, Message: "Connected and responding"}
}

func getUptime() string {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return fmt.Sprintf("Unable to read: %v", err)
	}
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return "Unable to parse"
	}
	return fields[0] + " seconds"
}

func getMemoryUsed() string {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return fmt.Sprintf("%.2f MB (Alloc) / %.2f MB (Sys)", float64(m.Alloc)/1024/1024, float64(m.Sys)/1024/1024)
}

func getECSInfo() (*ECSInfo, string) {
	metaURI := os.Getenv("ECS_CONTAINER_METADATA_URI_V4")
	if metaURI == "" {
		return nil, "ECS_CONTAINER_METADATA_URI_V4 not set"
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(metaURI + "/task")
	if err != nil {
		return nil, fmt.Sprintf("Failed to fetch: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Sprintf("Failed to read response: %v", err)
	}

	var info ECSInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Sprintf("Failed to parse JSON: %v", err)
	}
	return &info, ""
}

var tmpl = template.Must(template.New("page").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>BigDemo - Running</title>
<style>
  :root {
    --bg: #0f172a;
    --surface: #1e293b;
    --border: #334155;
    --text: #e2e8f0;
    --muted: #94a3b8;
    --accent: #38bdf8;
    --green: #4ade80;
    --red: #f87171;
    --yellow: #fbbf24;
  }
  * { margin:0; padding:0; box-sizing:border-box; }
  body {
    font-family: 'Segoe UI', system-ui, -apple-system, sans-serif;
    background: var(--bg);
    color: var(--text);
    min-height: 100vh;
    display: flex;
    justify-content: center;
    padding: 2rem 1rem;
  }
  .container { max-width: 800px; width: 100%; }
  .header {
    text-align: center;
    margin-bottom: 2rem;
    padding: 2rem;
    background: linear-gradient(135deg, #1e293b 0%, #0f172a 100%);
    border: 1px solid var(--border);
    border-radius: 12px;
  }
  .header h1 {
    font-size: 2rem;
    background: linear-gradient(90deg, var(--accent), #a78bfa);
    -webkit-background-clip: text;
    -webkit-text-fill-color: transparent;
    margin-bottom: 0.5rem;
  }
  .header p { color: var(--muted); font-size: 1.05rem; }
  .status-badge {
    display: inline-block;
    padding: 0.25rem 0.75rem;
    border-radius: 9999px;
    font-size: 0.8rem;
    font-weight: 600;
    margin-top: 0.75rem;
  }
  .badge-ok { background: rgba(74,222,128,0.15); color: var(--green); border: 1px solid rgba(74,222,128,0.3); }
  .badge-err { background: rgba(248,113,113,0.15); color: var(--red); border: 1px solid rgba(248,113,113,0.3); }
  .badge-warn { background: rgba(251,191,36,0.15); color: var(--yellow); border: 1px solid rgba(251,191,36,0.3); }
  .card {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 1.5rem;
    margin-bottom: 1.25rem;
  }
  .card h2 {
    font-size: 1rem;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: var(--accent);
    margin-bottom: 1rem;
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  .card h2::before {
    content: '';
    display: inline-block;
    width: 4px;
    height: 1em;
    background: var(--accent);
    border-radius: 2px;
  }
  table { width: 100%; border-collapse: collapse; }
  td, th {
    padding: 0.6rem 0.75rem;
    text-align: left;
    border-bottom: 1px solid var(--border);
    font-size: 0.9rem;
  }
  th { color: var(--muted); font-weight: 500; width: 40%; }
  td { font-family: 'SF Mono', 'Fira Code', monospace; word-break: break-all; }
  .empty { color: var(--muted); font-style: italic; }
  .metric-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 1rem;
  }
  .metric {
    background: var(--bg);
    border-radius: 8px;
    padding: 1rem;
    text-align: center;
  }
  .metric-label { font-size: 0.75rem; color: var(--muted); text-transform: uppercase; letter-spacing: 0.05em; }
  .metric-value { font-size: 1.1rem; margin-top: 0.25rem; font-family: monospace; }
  .svc-status {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.75rem 1rem;
    background: var(--bg);
    border-radius: 8px;
    margin-bottom: 0.75rem;
  }
  .svc-status:last-child { margin-bottom: 0; }
  .dot {
    width: 10px; height: 10px;
    border-radius: 50%;
    flex-shrink: 0;
  }
  .dot-green { background: var(--green); box-shadow: 0 0 6px rgba(74,222,128,0.5); }
  .dot-red { background: var(--red); box-shadow: 0 0 6px rgba(248,113,113,0.5); }
  .dot-gray { background: var(--muted); }
  .svc-name { font-weight: 600; min-width: 80px; }
  .svc-msg { color: var(--muted); font-size: 0.85rem; }
  .footer { text-align: center; color: var(--muted); font-size: 0.8rem; margin-top: 1rem; }
</style>
</head>
<body>
<div class="container">
  <div class="header">
    <h1>⚡ BigDemo</h1>
    <p>Demonstration application is running successfully</p>
    <div class="status-badge badge-ok">● OPERATIONAL</div>
  </div>

  <div class="card">
    <h2>Environment Variables</h2>
    <table>
      {{range .EnvVars}}
      <tr>
        <th>{{.Name}}</th>
        <td>{{if .Value}}{{.Value}}{{else}}<span class="empty">not set</span>{{end}}</td>
      </tr>
      {{end}}
    </table>
  </div>

  <div class="card">
    <h2>Service Connectivity</h2>
    <div class="svc-status">
      <div class="dot {{if not .DBStatus.Present}}dot-gray{{else if .DBStatus.Connected}}dot-green{{else}}dot-red{{end}}"></div>
      <span class="svc-name">Database</span>
      <span class="svc-msg">{{.DBStatus.Message}}</span>
    </div>
    <div class="svc-status">
      <div class="dot {{if not .RedisStatus.Present}}dot-gray{{else if .RedisStatus.Connected}}dot-green{{else}}dot-red{{end}}"></div>
      <span class="svc-name">Redis</span>
      <span class="svc-msg">{{.RedisStatus.Message}}</span>
    </div>
  </div>

  <div class="card">
    <h2>System Metrics</h2>
    <div class="metric-grid">
      <div class="metric">
        <div class="metric-label">Uptime</div>
        <div class="metric-value">{{.Uptime}}</div>
      </div>
      <div class="metric">
        <div class="metric-label">Memory</div>
        <div class="metric-value">{{.MemoryUsed}}</div>
      </div>
    </div>
  </div>

  <div class="card">
    <h2>ECS Container Metadata</h2>
    {{if .ECSError}}
      <div class="svc-status">
        <div class="dot dot-gray"></div>
        <span class="svc-msg">{{.ECSError}}</span>
      </div>
    {{else if .ECSInfo}}
      <table>
        <tr><th>Availability Zone</th><td>{{if .ECSInfo.AvailabilityZone}}{{.ECSInfo.AvailabilityZone}}{{else}}<span class="empty">n/a</span>{{end}}</td></tr>
        {{range .ECSInfo.Containers}}
        <tr><th>Container: {{.Name}}</th><td>{{if .ImageID}}{{.ImageID}}{{else}}<span class="empty">n/a</span>{{end}}</td></tr>
        {{end}}
      </table>
    {{end}}
  </div>

  <div class="footer">github.com/tapitalee/bigdemo</div>
</div>
</body>
</html>
`))

func handler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	ecsInfo, ecsErr := getECSInfo()

	data := PageData{
		EnvVars:     getEnvVars(),
		DBStatus:    checkDB(),
		RedisStatus: checkRedis(),
		Uptime:      getUptime(),
		MemoryUsed:  getMemoryUsed(),
		ECSInfo:     ecsInfo,
		ECSError:    ecsErr,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, "Template error: "+err.Error(), 500)
	}
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "80"
	}

	http.HandleFunc("/", handler)
	fmt.Printf("BigDemo listening on :%s\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
