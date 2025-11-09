# railway-image-updater

A Golang HTTP service that updates Railway services with new Docker image tags.

## Features

- HTTP endpoint accepting JSON PUT requests to update Railway service images
- Validates UUIDs and input parameters
- Filters services by Docker image prefixes
- Automatically updates matching services to a new version
- Triggers deployment of updated services
- Health check endpoint

## Prerequisites

- Go 1.20 or higher
- Railway API token

## Installation

```bash
go mod download
go build -o railway-image-updater .
```

## Configuration

Set the following environment variable:

- `RAILWAY_API_TOKEN`: Your Railway API token (required)
- `PORT`: Port to run the server on (optional, defaults to 8080)

## Usage

### Starting the Server

```bash
export RAILWAY_API_TOKEN=your-railway-token
./railway-image-updater
```

The server will start on port 8080 by default.

### API Endpoints

#### Update Services

**Endpoint:** `PUT /update`

**Request Body:**

```json
{
  "project_id": "550e8400-e29b-41d4-a716-446655440000",
  "environment_id": "550e8400-e29b-41d4-a716-446655440001",
  "image_prefixes": ["myapp", "docker.io/myorg/myapp"],
  "new_version": "v1.2.3"
}
```

**Parameters:**

- `project_id` (string, required): Railway project UUID
- `environment_id` (string, required): Railway environment UUID
- `image_prefixes` (array of strings, required): List of Docker image name prefixes (without version tags)
- `new_version` (string, required): New Docker image tag to update to

**Success Response (200 OK):**

```json
{
  "message": "Successfully updated 2 service(s)",
  "updated_services": ["api-service", "worker-service"]
}
```

**Error Response (4xx/5xx):**

```json
{
  "error": "Error message describing what went wrong"
}
```

#### Health Check

**Endpoint:** `GET /health`

**Response:**

```json
{
  "status": "ok"
}
```

## How It Works

1. The endpoint receives a PUT request with project ID, environment ID, image prefixes, and new version
2. Input validation is performed (UUIDs, non-empty arrays, etc.)
3. The service queries Railway API for all services in the specified environment
4. Services with Docker images matching any of the provided prefixes are identified
5. Each matching service's image tag is updated to the new version
6. The updated services are redeployed
7. A list of updated service names is returned

## Example

Update all services using the "myapp" Docker image to version "v2.0.0":

```bash
curl -X PUT http://localhost:8080/update \
  -H "Content-Type: application/json" \
  -d '{
    "project_id": "550e8400-e29b-41d4-a716-446655440000",
    "environment_id": "550e8400-e29b-41d4-a716-446655440001",
    "image_prefixes": ["myapp"],
    "new_version": "v2.0.0"
  }'
```

## Testing

Run the test suite:

```bash
go test -v .
```

## Deployment

### Docker

Build and run with Docker:

```bash
docker build -t railway-image-updater .
docker run -p 8080:8080 -e RAILWAY_API_TOKEN=your-token railway-image-updater
```

### Railway

Deploy to Railway:

1. Push your code to a Git repository
2. Connect the repository to Railway
3. Set the `RAILWAY_API_TOKEN` environment variable in Railway
4. Railway will automatically build and deploy your service

## License

See LICENSE file for details.