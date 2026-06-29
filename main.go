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

type DeployCommitRequest struct {
	ProjectID     string   `json:"project_id"`
	EnvironmentID string   `json:"environment_id"`
	RepoPrefixes  []string `json:"repo_prefixes"`
	CommitSha     string   `json:"commit_sha"`
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

	http.HandleFunc("/deploy-commit", func(w http.ResponseWriter, r *http.Request) {
		handleDeployCommit(w, r, client)
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

	for _, prefix := range req.ImagePrefixes {
		if strings.TrimSpace(prefix) == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "image_prefixes entries cannot be empty"})
			return
		}
	}

	if req.NewVersion == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "new_version cannot be empty"})
		return
	}

	if !verifyProjectScope(w, client, req.ProjectID, req.EnvironmentID) {
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

func handleDeployCommit(w http.ResponseWriter, r *http.Request, client *RailwayClient) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPut {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Method not allowed, use PUT"})
		return
	}

	var req DeployCommitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("Invalid JSON: %v", err)})
		return
	}

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

	if len(req.RepoPrefixes) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "repo_prefixes cannot be empty"})
		return
	}

	for _, prefix := range req.RepoPrefixes {
		if strings.TrimSpace(prefix) == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "repo_prefixes entries cannot be empty"})
			return
		}
	}

	if req.CommitSha == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "commit_sha cannot be empty"})
		return
	}

	if !verifyProjectScope(w, client, req.ProjectID, req.EnvironmentID) {
		return
	}

	updatedServices, err := client.DeployServicesByCommit(req.EnvironmentID, req.RepoPrefixes, req.CommitSha)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("Failed to deploy services: %v", err)})
		return
	}

	if len(updatedServices) == 0 {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(SuccessResponse{
			Message:         "No services matched the provided repo prefixes",
			UpdatedServices: []string{},
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(SuccessResponse{
		Message:         fmt.Sprintf("Successfully deployed %d service(s)", len(updatedServices)),
		UpdatedServices: updatedServices,
	})
}

func verifyProjectScope(w http.ResponseWriter, client *RailwayClient, projectID, environmentID string) bool {
	actualProjectID, err := client.getProjectID(environmentID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("Failed to resolve project for environment: %v", err)})
		return false
	}
	if actualProjectID != projectID {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "project_id does not match the project for the given environment_id"})
		return false
	}
	return true
}

func matchesPrefix(image string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(image, prefix) {
			return true
		}
	}
	return false
}
