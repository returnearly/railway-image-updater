package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

const railwayAPIURL = "https://backboard.railway.app/graphql/v2"

type RailwayClient struct {
	token                  string
	httpClient             *http.Client
	registryCredentialUser string
	registryCredentialPass string
}

type GraphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

type GraphQLResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors,omitempty"`
}

type Service struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Image       string `json:"image"`
	NumReplicas int    `json:"numReplicas"`
}

func NewRailwayClient(token string, registryUser string, registryPass string) *RailwayClient {
	return &RailwayClient{
		token:                  token,
		httpClient:             &http.Client{},
		registryCredentialUser: registryUser,
		registryCredentialPass: registryPass,
	}
}

func (c *RailwayClient) doRequest(query string, variables map[string]interface{}) (json.RawMessage, error) {
	reqBody := GraphQLRequest{
		Query:     query,
		Variables: variables,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Debug logging
	log.Printf("GraphQL Request: %s", string(jsonData))

	req, err := http.NewRequest("POST", railwayAPIURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Debug logging
	log.Printf("GraphQL Response (Status %d): %s", resp.StatusCode, string(body))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var graphqlResp GraphQLResponse
	if err := json.Unmarshal(body, &graphqlResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(graphqlResp.Errors) > 0 {
		return nil, fmt.Errorf("GraphQL error: %s", graphqlResp.Errors[0].Message)
	}

	return graphqlResp.Data, nil
}

func (c *RailwayClient) GetServices(environmentID string) ([]Service, error) {
	query := `
		query Environment($environmentId: String!) {
			environment(id: $environmentId) {
				id
				name
				projectId
				serviceInstances(after: null) {
					edges {
						node {
							id
							serviceId
							serviceName
							latestDeployment {
								meta
							}
							source {
								image
								repo
							}
						}
					}
					pageInfo {
						endCursor
						hasNextPage
						hasPreviousPage
						startCursor
					}
				}
			}
		}
	`

	variables := map[string]interface{}{
		"environmentId": environmentID,
	}

	data, err := c.doRequest(query, variables)
	if err != nil {
		return nil, err
	}

	var result struct {
		Environment struct {
			ID               string `json:"id"`
			Name             string `json:"name"`
			ProjectID        string `json:"projectId"`
			ServiceInstances struct {
				Edges []struct {
					Node struct {
						ID               string `json:"id"`
						ServiceID        string `json:"serviceId"`
						ServiceName      string `json:"serviceName"`
						LatestDeployment *struct {
							Meta json.RawMessage `json:"meta"`
						} `json:"latestDeployment"`
						Source struct {
							Image string `json:"image"`
							Repo  string `json:"repo"`
						} `json:"source"`
					} `json:"node"`
				} `json:"edges"`
				PageInfo struct {
					EndCursor       string `json:"endCursor"`
					HasNextPage     bool   `json:"hasNextPage"`
					HasPreviousPage bool   `json:"hasPreviousPage"`
					StartCursor     string `json:"startCursor"`
				} `json:"pageInfo"`
			} `json:"serviceInstances"`
		} `json:"environment"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse services: %w", err)
	}

	services := make([]Service, 0)
	for _, edge := range result.Environment.ServiceInstances.Edges {
		if edge.Node.Source.Image != "" {
			replicas := resolveReplicaCount(edge.Node.ServiceName, edge.Node.LatestDeployment)
			services = append(services, Service{
				ID:          edge.Node.ServiceID,
				Name:        edge.Node.ServiceName,
				Image:       edge.Node.Source.Image,
				NumReplicas: replicas,
			})
		}
	}

	return services, nil
}

// resolveReplicaCount extracts the replica count from the latest deployment meta.
// It looks in meta.serviceManifest.deploy.multiRegionConfig for numReplicas values.
// Falls back to 1 if no replica count is found.
func resolveReplicaCount(serviceName string, latestDeployment *struct {
	Meta json.RawMessage `json:"meta"`
}) int {
	replicas := 1

	if latestDeployment == nil || latestDeployment.Meta == nil {
		log.Printf("Replica count for %s: no deployment meta, defaulting to %d", serviceName, replicas)
		return replicas
	}

	// The meta field can be either a JSON object or a stringified JSON string (double-encoded).
	// Try direct unmarshal first, then handle the double-encoded case.
	var meta map[string]interface{}
	if err := json.Unmarshal(latestDeployment.Meta, &meta); err != nil {
		// meta might be a JSON string containing JSON — try double-decoding
		var metaStr string
		if strErr := json.Unmarshal(latestDeployment.Meta, &metaStr); strErr != nil {
			log.Printf("Failed to parse meta JSON for %s: %v", serviceName, err)
			return replicas
		}
		if err2 := json.Unmarshal([]byte(metaStr), &meta); err2 != nil {
			log.Printf("Failed to double-decode meta JSON for %s: %v", serviceName, err2)
			return replicas
		}
	}

	// Navigate: meta.serviceManifest.deploy.multiRegionConfig.<region>.numReplicas
	serviceManifest, ok := meta["serviceManifest"].(map[string]interface{})
	if !ok {
		log.Printf("Replica count for %s: no serviceManifest, defaulting to %d", serviceName, replicas)
		return replicas
	}

	deploy, ok := serviceManifest["deploy"].(map[string]interface{})
	if !ok {
		log.Printf("Replica count for %s: no deploy config, defaulting to %d", serviceName, replicas)
		return replicas
	}

	multiRegionConfig, ok := deploy["multiRegionConfig"].(map[string]interface{})
	if !ok {
		log.Printf("Replica count for %s: no multiRegionConfig, defaulting to %d", serviceName, replicas)
		return replicas
	}

	// Check all regions and use the first valid numReplicas found
	for region, regionConfig := range multiRegionConfig {
		regionMap, ok := regionConfig.(map[string]interface{})
		if !ok {
			continue
		}
		if numReplicas, ok := regionMap["numReplicas"].(float64); ok && numReplicas > 0 {
			replicas = int(numReplicas)
			log.Printf("Replica count for %s: region=%s numReplicas=%d", serviceName, region, replicas)
			return replicas
		}
	}

	log.Printf("Replica count for %s: no valid numReplicas in multiRegionConfig, defaulting to %d", serviceName, replicas)
	return replicas
}

func (c *RailwayClient) UpdateServiceImage(serviceID, environmentID, newImage string, numReplicas int) error {
	// Step 1: Update the service instance image using ServiceInstanceUpdate
	updateQuery := `
		mutation ServiceInstanceUpdate($environmentId: String!, $serviceId: String!, $input: ServiceInstanceUpdateInput!) {
			serviceInstanceUpdate(environmentId: $environmentId, serviceId: $serviceId, input: $input)
		}
	`

	input := map[string]interface{}{
		"source": map[string]interface{}{
			"image": newImage,
		},
		"numReplicas": numReplicas,
	}

	// Include registry credentials if configured
	if c.registryCredentialUser != "" && c.registryCredentialPass != "" {
		input["registryCredentials"] = map[string]interface{}{
			"username": c.registryCredentialUser,
			"password": c.registryCredentialPass,
		}
	}

	updateVariables := map[string]interface{}{
		"environmentId": environmentID,
		"serviceId":     serviceID,
		"input":         input,
	}

	_, err := c.doRequest(updateQuery, updateVariables)
	if err != nil {
		return fmt.Errorf("failed to update service instance: %w", err)
	}

	// Step 2: Deploy the service using serviceInstanceDeploy
	deployQuery := `
		mutation ServiceInstanceDeploy($serviceId: String!, $environmentId: String!, $latestCommit: Boolean) {
			serviceInstanceDeploy(serviceId: $serviceId, environmentId: $environmentId, latestCommit: $latestCommit)
		}
	`

	deployVariables := map[string]interface{}{
		"serviceId":     serviceID,
		"environmentId": environmentID,
		"latestCommit":  false,
	}

	_, err = c.doRequest(deployQuery, deployVariables)
	if err != nil {
		return fmt.Errorf("failed to deploy service instance: %w", err)
	}

	return nil
}

func (c *RailwayClient) getProjectID(environmentID string) (string, error) {
	query := `
		query Environment($environmentId: String!) {
			environment(id: $environmentId) {
				projectId
			}
		}
	`

	variables := map[string]interface{}{
		"environmentId": environmentID,
	}

	data, err := c.doRequest(query, variables)
	if err != nil {
		return "", err
	}

	var result struct {
		Environment struct {
			ProjectID string `json:"projectId"`
		} `json:"environment"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("failed to parse project ID: %w", err)
	}

	return result.Environment.ProjectID, nil
}

func (c *RailwayClient) UpdateServices(environmentID string, imagePrefixes []string, newVersion string) ([]string, error) {
	services, err := c.GetServices(environmentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get services: %w", err)
	}

	updatedServices := make([]string, 0)

	for _, service := range services {
		// Check if service image matches any of the prefixes
		matched := false
		var imagePrefix string
		for _, prefix := range imagePrefixes {
			if strings.HasPrefix(service.Image, prefix) {
				matched = true
				imagePrefix = prefix
				break
			}
		}

		if !matched {
			continue
		}

		// Extract the image name without tag
		imageParts := strings.Split(service.Image, ":")
		var newImage string
		if len(imageParts) > 1 {
			// Has a tag, replace it
			newImage = imageParts[0] + ":" + newVersion
		} else {
			// No tag, add it
			newImage = service.Image + ":" + newVersion
		}

		// Ensure we're still using the same prefix (in case the image has registry path)
		if !strings.HasPrefix(newImage, imagePrefix) {
			// Try with the prefix directly
			newImage = imagePrefix + ":" + newVersion
		}

		log.Printf("Updating service %s from %s to %s (replicas=%d)", service.Name, service.Image, newImage, service.NumReplicas)

		// Update the service and trigger deployment
		if err := c.UpdateServiceImage(service.ID, environmentID, newImage, service.NumReplicas); err != nil {
			return updatedServices, fmt.Errorf("failed to update service %s: %w", service.Name, err)
		}

		updatedServices = append(updatedServices, service.Name)
	}

	return updatedServices, nil
}
