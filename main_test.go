package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestGetEnvVars(t *testing.T) {
	// Set one env var to verify it's picked up
	os.Setenv("TAP_APP_NAME", "test-app")
	defer os.Unsetenv("TAP_APP_NAME")

	vars := getEnvVars()

	if len(vars) != 5 {
		t.Fatalf("expected 5 env vars, got %d", len(vars))
	}

	expectedKeys := []string{
		"TAP_DEPLOY_NUMBER",
		"TAP_DOCKER_TAG",
		"TAP_APP_URL",
		"TAP_APP_NAME",
		"TAP_TEAM_NAME",
	}
	for i, key := range expectedKeys {
		if vars[i].Name != key {
			t.Errorf("expected vars[%d].Name = %q, got %q", i, key, vars[i].Name)
		}
	}

	// Check the one we set
	for _, v := range vars {
		if v.Name == "TAP_APP_NAME" && v.Value != "test-app" {
			t.Errorf("expected TAP_APP_NAME = %q, got %q", "test-app", v.Value)
		}
	}
}

func TestCheckDB_NoEnvVar(t *testing.T) {
	os.Unsetenv("DATABASE_URL")
	status := checkDB()

	if status.Present {
		t.Error("expected Present=false when DATABASE_URL is not set")
	}
	if status.Message != "DATABASE_URL not set" {
		t.Errorf("unexpected message: %s", status.Message)
	}
}

func TestCheckRedis_NoEnvVar(t *testing.T) {
	os.Unsetenv("REDIS_URL")
	status := checkRedis()

	if status.Present {
		t.Error("expected Present=false when REDIS_URL is not set")
	}
	if status.Message != "REDIS_URL not set" {
		t.Errorf("unexpected message: %s", status.Message)
	}
}

func TestCheckRedis_InvalidURL(t *testing.T) {
	os.Setenv("REDIS_URL", "not-a-valid-url")
	defer os.Unsetenv("REDIS_URL")

	status := checkRedis()

	if !status.Present {
		t.Error("expected Present=true when REDIS_URL is set")
	}
	if status.Connected {
		t.Error("expected Connected=false for invalid URL")
	}
	if !strings.Contains(status.Message, "Invalid URL") {
		t.Errorf("expected 'Invalid URL' in message, got: %s", status.Message)
	}
}

func TestGetMemoryUsed(t *testing.T) {
	mem := getMemoryUsed()

	if !strings.Contains(mem, "MB (Alloc)") {
		t.Errorf("expected memory string to contain 'MB (Alloc)', got: %s", mem)
	}
	if !strings.Contains(mem, "MB (Sys)") {
		t.Errorf("expected memory string to contain 'MB (Sys)', got: %s", mem)
	}
}

func TestGetECSInfo_NoEnvVar(t *testing.T) {
	os.Unsetenv("ECS_CONTAINER_METADATA_URI_V4")

	info, errMsg := getECSInfo()

	if info != nil {
		t.Error("expected nil ECSInfo when env var not set")
	}
	if errMsg != "ECS_CONTAINER_METADATA_URI_V4 not set" {
		t.Errorf("unexpected error message: %s", errMsg)
	}
}

func TestHandler_RootPath(t *testing.T) {
	// Clear external service env vars so handler doesn't try to connect
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("REDIS_URL")
	os.Unsetenv("ECS_CONTAINER_METADATA_URI_V4")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Errorf("expected Content-Type text/html, got %s", ct)
	}

	body := w.Body.String()
	if !strings.Contains(body, "BigDemo") {
		t.Error("expected response body to contain 'BigDemo'")
	}
	if !strings.Contains(body, "DATABASE_URL not set") {
		t.Error("expected response body to contain 'DATABASE_URL not set'")
	}
}

func TestHandler_NotFoundPath(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404 for /nonexistent, got %d", w.Code)
	}
}

func TestHandler_EnvVarsInOutput(t *testing.T) {
	os.Setenv("TAP_APP_NAME", "my-demo-app")
	os.Setenv("TAP_TEAM_NAME", "platform-team")
	defer os.Unsetenv("TAP_APP_NAME")
	defer os.Unsetenv("TAP_TEAM_NAME")

	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("REDIS_URL")
	os.Unsetenv("ECS_CONTAINER_METADATA_URI_V4")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "my-demo-app") {
		t.Error("expected response to contain env var value 'my-demo-app'")
	}
	if !strings.Contains(body, "platform-team") {
		t.Error("expected response to contain env var value 'platform-team'")
	}
}
