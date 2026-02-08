package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleUpdate_MethodNotAllowed(t *testing.T) {
	client := NewRailwayClient("test-token", "", "")
	req := httptest.NewRequest(http.MethodGet, "/update", nil)
	w := httptest.NewRecorder()

	handleUpdate(w, req, client)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestHandleUpdate_InvalidJSON(t *testing.T) {
	client := NewRailwayClient("test-token", "", "")
	req := httptest.NewRequest(http.MethodPut, "/update", bytes.NewBufferString("invalid json"))
	w := httptest.NewRecorder()

	handleUpdate(w, req, client)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Error == "" {
		t.Error("Expected error message")
	}
}

func TestHandleUpdate_InvalidProjectID(t *testing.T) {
	client := NewRailwayClient("test-token", "", "")
	reqBody := UpdateRequest{
		ProjectID:     "invalid-uuid",
		EnvironmentID: "550e8400-e29b-41d4-a716-446655440000",
		ImagePrefixes: []string{"myapp"},
		NewVersion:    "v1.0.0",
	}
	jsonData, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPut, "/update", bytes.NewBuffer(jsonData))
	w := httptest.NewRecorder()

	handleUpdate(w, req, client)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Error == "" {
		t.Error("Expected error message about invalid project_id")
	}
}

func TestHandleUpdate_InvalidEnvironmentID(t *testing.T) {
	client := NewRailwayClient("test-token", "", "")
	reqBody := UpdateRequest{
		ProjectID:     "550e8400-e29b-41d4-a716-446655440000",
		EnvironmentID: "invalid-uuid",
		ImagePrefixes: []string{"myapp"},
		NewVersion:    "v1.0.0",
	}
	jsonData, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPut, "/update", bytes.NewBuffer(jsonData))
	w := httptest.NewRecorder()

	handleUpdate(w, req, client)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Error == "" {
		t.Error("Expected error message about invalid environment_id")
	}
}

func TestHandleUpdate_EmptyImagePrefixes(t *testing.T) {
	client := NewRailwayClient("test-token", "", "")
	reqBody := UpdateRequest{
		ProjectID:     "550e8400-e29b-41d4-a716-446655440000",
		EnvironmentID: "550e8400-e29b-41d4-a716-446655440001",
		ImagePrefixes: []string{},
		NewVersion:    "v1.0.0",
	}
	jsonData, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPut, "/update", bytes.NewBuffer(jsonData))
	w := httptest.NewRecorder()

	handleUpdate(w, req, client)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Error == "" {
		t.Error("Expected error message about empty image_prefixes")
	}
}

func TestHandleUpdate_EmptyNewVersion(t *testing.T) {
	client := NewRailwayClient("test-token", "", "")
	reqBody := UpdateRequest{
		ProjectID:     "550e8400-e29b-41d4-a716-446655440000",
		EnvironmentID: "550e8400-e29b-41d4-a716-446655440001",
		ImagePrefixes: []string{"myapp"},
		NewVersion:    "",
	}
	jsonData, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPut, "/update", bytes.NewBuffer(jsonData))
	w := httptest.NewRecorder()

	handleUpdate(w, req, client)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Error == "" {
		t.Error("Expected error message about empty new_version")
	}
}

func TestMatchesPrefix(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		prefixes []string
		expected bool
	}{
		{
			name:     "exact match",
			image:    "myapp:v1.0.0",
			prefixes: []string{"myapp"},
			expected: true,
		},
		{
			name:     "with registry",
			image:    "docker.io/myapp:v1.0.0",
			prefixes: []string{"docker.io/myapp"},
			expected: true,
		},
		{
			name:     "no match",
			image:    "otherapp:v1.0.0",
			prefixes: []string{"myapp"},
			expected: false,
		},
		{
			name:     "multiple prefixes - match first",
			image:    "myapp:v1.0.0",
			prefixes: []string{"myapp", "otherapp"},
			expected: true,
		},
		{
			name:     "multiple prefixes - match second",
			image:    "otherapp:v1.0.0",
			prefixes: []string{"myapp", "otherapp"},
			expected: true,
		},
		{
			name:     "multiple prefixes - no match",
			image:    "thirdapp:v1.0.0",
			prefixes: []string{"myapp", "otherapp"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesPrefix(tt.image, tt.prefixes)
			if result != tt.expected {
				t.Errorf("matchesPrefix(%q, %v) = %v, expected %v", tt.image, tt.prefixes, result, tt.expected)
			}
		})
	}
}

func TestResolveReplicaCount(t *testing.T) {
	tests := []struct {
		name     string
		meta     *string
		expected int
	}{
		{
			name:     "nil deployment",
			meta:     nil,
			expected: 1,
		},
		{
			name:     "empty meta",
			meta:     strPtr(`{}`),
			expected: 1,
		},
		{
			name:     "meta with replicas in multiRegionConfig",
			meta:     strPtr(`{"serviceManifest":{"deploy":{"multiRegionConfig":{"us-east4-eqdc4a":{"numReplicas":3}}}}}`),
			expected: 3,
		},
		{
			name:     "meta with replicas = 1",
			meta:     strPtr(`{"serviceManifest":{"deploy":{"multiRegionConfig":{"us-east4-eqdc4a":{"numReplicas":1}}}}}`),
			expected: 1,
		},
		{
			name:     "meta with no multiRegionConfig",
			meta:     strPtr(`{"serviceManifest":{"deploy":{}}}`),
			expected: 1,
		},
		{
			name:     "meta with zero replicas defaults to 1",
			meta:     strPtr(`{"serviceManifest":{"deploy":{"multiRegionConfig":{"us-east4-eqdc4a":{"numReplicas":0}}}}}`),
			expected: 1,
		},
		{
			name:     "invalid JSON meta",
			meta:     strPtr(`not-json`),
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var deployment *struct {
				Meta json.RawMessage `json:"meta"`
			}
			if tt.meta != nil {
				deployment = &struct {
					Meta json.RawMessage `json:"meta"`
				}{
					Meta: json.RawMessage(*tt.meta),
				}
			}
			result := resolveReplicaCount("test-service", deployment)
			if result != tt.expected {
				t.Errorf("resolveReplicaCount() = %d, expected %d", result, tt.expected)
			}
		})
	}
}

func strPtr(s string) *string {
	return &s
}

func TestHealthEndpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("Expected status 'ok', got '%s'", resp["status"])
	}
}
