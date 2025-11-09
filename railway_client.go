package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const railwayAPIURL = "https://backboard.railway.app/graphql/v2"

type RailwayClient struct {
	token      string
	httpClient *http.Client
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
	ID    string `json:"id"`
	Name  string `json:"name"`
	Image string `json:"image"`
}

func NewRailwayClient(token string) *RailwayClient {
	return &RailwayClient{
		token:      token,
		httpClient: &http.Client{},
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

func (c *RailwayClient) GetServices(projectID, environmentID string) ([]Service, error) {
	query := `
		query GetServices($projectId: String!, $environmentId: String!) {
			services(projectId: $projectId, environmentId: $environmentId) {
				edges {
					node {
						id
						name
						source {
							image
						}
					}
				}
			}
		}
	`

	variables := map[string]interface{}{
		"projectId":     projectID,
		"environmentId": environmentID,
	}

	data, err := c.doRequest(query, variables)
	if err != nil {
		return nil, err
	}

	var result struct {
		Services struct {
			Edges []struct {
				Node struct {
					ID     string `json:"id"`
					Name   string `json:"name"`
					Source struct {
						Image string `json:"image"`
					} `json:"source"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"services"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse services: %w", err)
	}

	services := make([]Service, 0)
	for _, edge := range result.Services.Edges {
		if edge.Node.Source.Image != "" {
			services = append(services, Service{
				ID:    edge.Node.ID,
				Name:  edge.Node.Name,
				Image: edge.Node.Source.Image,
			})
		}
	}

	return services, nil
}

func (c *RailwayClient) UpdateServiceImage(serviceID, newImage string) error {
	query := `
		mutation ServiceUpdate($serviceId: String!, $input: ServiceUpdateInput!) {
			serviceUpdate(id: $serviceId, input: $input) {
				id
			}
		}
	`

	variables := map[string]interface{}{
		"serviceId": serviceID,
		"input": map[string]interface{}{
			"source": map[string]interface{}{
				"image": newImage,
			},
		},
	}

	_, err := c.doRequest(query, variables)
	return err
}

func (c *RailwayClient) DeployService(serviceID, environmentID string) error {
	query := `
		mutation ServiceInstanceRedeploy($serviceId: String!, $environmentId: String!) {
			serviceInstanceRedeploy(serviceId: $serviceId, environmentId: $environmentId)
		}
	`

	variables := map[string]interface{}{
		"serviceId":     serviceID,
		"environmentId": environmentID,
	}

	_, err := c.doRequest(query, variables)
	return err
}

func (c *RailwayClient) UpdateServices(projectID, environmentID string, imagePrefixes []string, newVersion string) ([]string, error) {
	services, err := c.GetServices(projectID, environmentID)
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

		// Update the service
		if err := c.UpdateServiceImage(service.ID, newImage); err != nil {
			return updatedServices, fmt.Errorf("failed to update service %s: %w", service.Name, err)
		}

		// Deploy the service
		if err := c.DeployService(service.ID, environmentID); err != nil {
			return updatedServices, fmt.Errorf("failed to deploy service %s: %w", service.Name, err)
		}

		updatedServices = append(updatedServices, service.Name)
	}

	return updatedServices, nil
}
