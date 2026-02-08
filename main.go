package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/google/uuid"
)

type UpdateRequest struct {
	ProjectID     string   `json:"project_id"`
	EnvironmentID string   `json:"environment_id"`
	ImagePrefixes []string `json:"image_prefixes"`
	NewVersion    string   `json:"new_version"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type SuccessResponse struct {
	Message         string   `json:"message"`
	UpdatedServices []string `json:"updated_services"`
}

func main() {
	token := os.Getenv("RAILWAY_API_TOKEN")
	if token == "" {
		log.Fatal("RAILWAY_API_TOKEN environment variable is required")
	}

	registryUser := os.Getenv("RAILWAY_DOCKER_REGISTRY_USER")
	registryPass := os.Getenv("RAILWAY_DOCKER_REGISTRY_TOKEN")

	client := NewRailwayClient(token, registryUser, registryPass)

	http.HandleFunc("/update", func(w http.ResponseWriter, r *http.Request) {
		handleUpdate(w, r, client)
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

func handleUpdate(w http.ResponseWriter, r *http.Request, client *RailwayClient) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPut {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Method not allowed, use PUT"})
		return
	}

	var req UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("Invalid JSON: %v", err)})
		return
	}

	// Validate UUIDs
	if _, err := uuid.Parse(req.ProjectID); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid project_id: must be a valid UUID"})
		return
	}

	if _, err := uuid.Parse(req.EnvironmentID); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid environment_id: must be a valid UUID"})
		return
	}

	if len(req.ImagePrefixes) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "image_prefixes cannot be empty"})
		return
	}

	if req.NewVersion == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "new_version cannot be empty"})
		return
	}

	// Get services and update matching ones
	updatedServices, err := client.UpdateServices(req.EnvironmentID, req.ImagePrefixes, req.NewVersion)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("Failed to update services: %v", err)})
		return
	}

	if len(updatedServices) == 0 {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(SuccessResponse{
			Message:         "No services matched the provided image prefixes",
			UpdatedServices: []string{},
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(SuccessResponse{
		Message:         fmt.Sprintf("Successfully updated %d service(s)", len(updatedServices)),
		UpdatedServices: updatedServices,
	})
}

func matchesPrefix(image string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(image, prefix) {
			return true
		}
	}
	return false
}
