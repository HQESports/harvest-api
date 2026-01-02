# Harvest

Harvest is a robust data processing system built with Go, designed to efficiently collect, process, and manage PUBG game data through a distributed job processing architecture.

## 🚀 Features

- **Job Queue System**: Asynchronous job processing with RabbitMQ
- **Caching Layer**: Redis-based caching for improved performance
- **Data Storage**: MongoDB for flexible document storage
- **API Authentication**: Secure token-based API authentication
- **Extensible Architecture**: Easily add new job processors
- **Batch Processing**: Efficient handling of large data volumes
- **Health Monitoring**: Built-in health checks for all components

## 🏗️ Architecture

The system consists of several core components:

- **API Server**: HTTP API for job management and data access
- **Job Processor**: Background worker for processing queued jobs
- **Cache**: Redis-based caching layer
- **Database**: MongoDB for data persistence
- **Message Queue**: RabbitMQ for job distribution

## 🔧 Components

### Controllers
- **JobController**: Manages job creation, queueing, and processing
- **PubgController**: Handles PUBG API interactions for player and tournament data
- **TokenController**: Manages API authentication tokens
- **ServerController**: Provides service health monitoring

### Database
- MongoDB collections for players, tournaments, matches, tokens, and jobs

### Caching
- Redis-based cache implementation

### Message Queue
- RabbitMQ for reliable job distribution

### Processors
- Extensible job processor system with batch processing capabilities

## 📋 Prerequisites

- Go 1.19+
- Docker and Docker Compose
- Make (optional, for using the provided Makefile)

## 🚀 Getting Started

### Setup with Docker Compose

The easiest way to set up the required infrastructure is with Docker Compose:

```bash
# Start all required services (MongoDB, Redis, RabbitMQ)
make docker-up

# Generate initial admin token
make token

# Run the API server
make api

# Run the RabbitMQ testing script
make rabbit
```

### Manual Setup

If you prefer to run services without Docker:

1. Install and configure MongoDB, Redis, and RabbitMQ
2. Update `config.json` with appropriate connection details
3. Generate an admin token with `make token`
4. Run the API server with `make api`
5. Run the RabbitMQ testing script with `make rabbit`

## 🔑 API Authentication

The system uses token-based authentication. An initial admin token is generated during setup, which can be used to create additional tokens.

### Token Types

- **ADMIN**: Can manage tokens and access all API endpoints
- **SERVICE**: Can only access API endpoints, not token management

## 🛠️ Development

For development with hot-reloading:

```bash
# Install Air (hot-reloading tool)
make install-air

# Run with hot-reloading
make dev
```

## 📦 Project Structure

```
harvest/
├── cmd/                    # Application entry points
│   ├── api/                # API server
│   ├── rabbit/             # RabbitMQ testing script
│   └── tokengen/           # Token generator tool
├── internal/               # Internal packages
│   ├── cache/              # Redis cache implementation
│   ├── config/             # Configuration handling
│   ├── controller/         # Business logic controllers
│   ├── database/           # Database operations
│   ├── model/              # Data models
│   ├── processor/          # Job processors
│   ├── rabbitmq/           # RabbitMQ client
│   └── server/             # HTTP server and API routes
├── pkg/                    # Public packages
│   └── pubg/               # PUBG API client
├── docker-compose.yml      # Docker Compose configuration
├── Makefile                # Build and development commands
└── config.json             # Application configuration
```

## ⚙️ Configuration

The application is configured through a `config.json` file:

```json
{
  "env": "development",
  "port": 8080,
  "app_name": "harvest",
  "pubg": {
    "api_key": "your-pubg-api-key",
    "base_url": "https://api.pubg.com",
    "shards": {
      "steam": "steam",
      "tournament": "tournament"
    },
    "request_delay": 1000,
    "max_retries": 3,
    "requests_per_minute": 10,
    "old_enough_min": 5,
    "cache": true,
    "default_cache_ttl": 3600
  },
  "mongodb": {
    "uri": "mongodb://root:password@localhost:27017",
    "username": "root",
    "password": "password",
    "db": "harvest",
    "options": {}
  },
  "redis": {
    "address": "localhost:6379",
    "password": "",
    "db": 0,
    "prefix": "harvest"
  },
  "logging": {
    "level": "debug",
    "format": "json",
    "directory": "logs"
  },
  "cors": {
    "allowed_origins": ["*"],
    "allowed_methods": ["GET", "POST", "PUT", "DELETE", "OPTIONS"],
    "allowed_headers": ["Authorization", "Content-Type"],
    "allow_credentials": true,
    "max_age": 86400
  },
  "rabbitmq": {
    "username": "admin",
    "password": "password",
    "host": "localhost",
    "port": 5672,
    "vhost": "/",
    "exchange_name": "harvest_jobs",
    "queue_name": "jobs",
    "routing_key": "jobs",
    "prefetch_count": 10,
    "max_retries": 3
  },
  "jobs": {
    "worker_count": 4,
    "default_batch_size": 10,
    "job_types": [
      {
        "type": "player-search-and-expand",
        "batch_size": 20
      }
    ]
  }
}
```

## 🔄 Job Processing

Jobs flow through the system as follows:

1. Job creation via API
2. Job stored in MongoDB
3. Job message sent to RabbitMQ
4. Job processor consumes message
5. Processor executes job with appropriate batch size
6. Job progress and results updated in MongoDB
7. Job status updated to completed or failed

## 📝 API Documentation

### Health Endpoints
- `GET /ready` - Check system health
  - Returns the status of database, cache, and RabbitMQ connections
  - Response: `200 OK` if all systems are operational, `503 Service Unavailable` otherwise
  - No authentication required

- `GET /online` - Simple online check
  - Returns a simple "Online" response to confirm the API server is running
  - Response: `200 OK` with "Online" text body
  - No authentication required

### PUBG Data Endpoints
All PUBG data endpoints require authentication with either ADMIN or SERVICE role tokens.

- `POST /api  ` - Create player entities from names
  - Request body: `{ "names": "player1,player2,player3" }`
  - Creates player entities for the provided comma-separated list of player names
  - Response: `201 Created` with `{ "playersProcessed": count }` indicating the number of processed players

- `POST /api/buildMatches` - Build match entities with filters
  - Request body: 
    ```json
    {
      "mapName": "Erangel_Main",
      "matchType": "live",
      "limit": 100
    }
    ```
  - Processes matches based on the provided filters
  - Valid map names include: `Baltic_Main`, `Chimera_Main`, `Desert_Main`, etc.
  - Valid match types: `live`, `event`
  - Response: `201 Created` with `{ "numMatches": count, "successful": count }`

- `POST /api/tournaments` - Create tournament entities
  - Fetches tournament data from PUBG API and creates entities in the database
  - No request body required
  - Response: `201 Created` with `{ "tournamentsProcessed": count }`

- `POST /api/expandPlayers` - Search and expand player entities
  - Query parameter: `limit` (number of players to process)
  - Searches for matches and extracts new players to expand the player database
  - Response: `201 Created` with `{ "playersUpserted": count }`

### Job Management
All job management endpoints require authentication with either ADMIN or SERVICE role tokens.

- `POST /api/jobs` - Create a new job
  - Request body:
    ```json
    {
      "type": "player-search-and-expand",
      "payload": {
        "property1": "value1",
        "property2": "value2"
      }
    }
    ```
  - Creates and queues a new job of the specified type with the provided payload
  - Response: `201 Created` with the created job details

- `GET /api/jobs` - List jobs for the current token
  - Query parameters:
    - `limit` (default: 20): Maximum number of jobs to return
    - `offset` (default: 0): Number of jobs to skip
  - Lists jobs created by the current token with pagination
  - Response: `200 OK` with an array of job objects

- `GET /api/jobs/all` - List all jobs (with optional status filter)
  - Query parameters:
    - `status`: Filter by job status (`queued`, `processing`, `completed`, `failed`)
    - `limit` (default: 20): Maximum number of jobs to return
    - `offset` (default: 0): Number of jobs to skip
  - Lists all jobs in the system with optional status filtering and pagination
  - Response: `200 OK` with an array of job objects

- `GET /api/jobs/types` - List available job types
  - Returns a list of all registered job processor types and their names
  - Response: `200 OK` with a map of job types to processor names

- `GET /api/jobs/:id` - Get a specific job
  - Path parameter: `id`: Job ID
  - Returns details for the specified job
  - Response: `200 OK` with the job object, `404 Not Found` if the job doesn't exist

### Token Management
All token management endpoints require authentication with an ADMIN role token.

- `POST /token` - Create a new token
  - Request body: `{ "name": "Token Name" }`
  - Creates a new SERVICE role token with the provided name
  - Response: `201 Created` with token details including the token string (only shown once)
    ```json
    {
      "token": "actual-token-string",
      "info": {
        "id": "token-id",
        "name": "Token Name",
        "role": "SERVICE",
        "createdAt": "2023-01-01T00:00:00Z",
        "lastUsed": "2023-01-01T00:00:00Z",
        "revoked": false
      }
    }
    ```

- `GET /token` - List all tokens
  - Returns a list of all tokens in the system (without the actual token strings)
  - Response: `200 OK` with an array of token objects

- `GET /token/:id` - Get a specific token
  - Path parameter: `id`: Token ID
  - Returns details for the specified token (without the actual token string)
  - Response: `200 OK` with the token object, `404 Not Found` if the token doesn't exist

- `DELETE /token/:id` - Revoke a token
  - Path parameter: `id`: Token ID
  - Revokes the specified token, preventing its further use
  - Response: `200 OK` with `{ "message": "Token revoked successfully" }`

## 📖 Documentation

### Code Documentation

For detailed code documentation, you can generate GoDoc for the project:

```bash
go doc -all ./...
```

### Database Schema

#### Players Collection
Stores player information retrieved from the PUBG API.
```json
{
  "id": "account-123456",
  "name": "PlayerName",
  "active": true
}
```

#### Tournaments Collection
Stores tournament information retrieved from the PUBG API.
```json
{
  "id": "eu-tournament-123",
  "name": "Europe PUBG Continental Series 5",
  "active": true
}
```

#### Matches Collection
Stores match data with metadata for processing.
```json
{
  "match_id": "match-123456",
  "shard_id": "steam",
  "map_name": "Erangel_Main",
  "game_mode": "squad-fpp",
  "duration": 1800,
  "is_custom_match": true,
  "created_at": "2023-01-01T00:00:00Z",
  "match_type": "LIVE_SCRIM",
  "processed": false,
  "imported_at": "2023-01-02T00:00:00Z",
  "player_count": 100,
  "team_count": 25,
  "telemetry_url": "https://example.com/telemetry/match-123456"
}
```

#### Jobs Collection
Stores job data and processing state.
```json
{
  "_id": "ObjectId('123456')",
  "type": "player-search-and-expand",
  "status": "queued",
  "progress": 0,
  "metrics": {
    "total_items": 100,
    "processed_items": 0,
    "success_count": 0,
    "warning_count": 0,
    "failure_count": 0,
    "batches_total": 10,
    "batches_complete": 0
  },
  "payload": {},
  "results": [],
  "error_list": [],
  "created_at": "2023-01-01T00:00:00Z",
  "updated_at": "2023-01-01T00:00:00Z",
  "user_id": "token-123",
  "batch_size": 10
}
```

#### Tokens Collection
Stores API authentication tokens.
```json
{
  "_id": "ObjectId('123456')",
  "token_hash": "hashed-token-value",
  "name": "API Token",
  "role": "SERVICE",
  "created_at": "2023-01-01T00:00:00Z",
  "expires_at": "2024-01-01T00:00:00Z",
  "last_used": "2023-01-02T00:00:00Z",
  "revoked": false
}
```

### Testing

To run tests for the project:

```bash
make test
```

Additional test documentation is available in each package's test files.

## 🛠️ Available Makefile Commands

- `make api` - Run the API server
- `make rabbit` - Run the RabbitMQ testing script
- `make token` - Generate initial admin token
- `make build` - Build the application
- `make test` - Run tests
- `make docker-up` - Start Docker Compose services
- `make docker-down` - Stop Docker Compose services
- `make install-air` - Install Air for hot reloading
- `make dev` - Run with hot reloading
- `make tidy` - Tidy Go module dependencies

## 🎮 PUBG API Client

The `pkg/pubg` package contains a custom client for interacting with the PUBG (PlayerUnknown's Battlegrounds) API. This client handles rate limiting, caching, and error handling while providing a clean interface for retrieving game data.

### Features

- **Automatic Rate Limiting**: Respects PUBG API rate limits to prevent throttling
- **Caching**: Optional Redis-based caching to improve performance and reduce API calls
- **Telemetry Processing**: Extracts useful data from match telemetry files
- **Error Handling**: Comprehensive error handling and logging
- **Tournament Support**: Methods for accessing tournament data

### Key Components

#### Core Client

- **Client**: The main client struct with methods for making API requests
- **Rate Limiting**: Implements token bucket algorithm for API request throttling
- **Caching**: Integration with Redis cache for API responses
- **Telemetry**: Methods for downloading and processing telemetry data

#### Data Models

- **Player Data**: Structures for player information
- **Match Data**: Structures for match details and relationships
- **Telemetry Data**: Structures for processing game telemetry
- **Tournament Data**: Structures for tournament information

### Client Methods

#### Player Methods

- `GetPlayersByNames(shard string, playerNames []string) (*PUBGPlayerResponse, error)`  
  Retrieves data for up to 10 players by their in-game names from a specific shard.

- `GetPlayersByIDs(shard string, playerIDs []string) (*PUBGPlayerResponse, error)`  
  Retrieves data for up to 10 players by their account IDs from a specific shard.

- `GetPlayerIDs(shard string, playerNames []string) (map[string]string, error)`  
  Returns a map of player names to their IDs, handling batches of 10 at a time.

- `GetMatchIDsForPlayers(shard string, playerIDs []string) ([]string, error)`  
  Retrieves all unique match IDs for a list of players by batching requests in groups of 10.

#### Match Methods

- `GetMatch(shard string, matchID string) (*PUBGMatchResponse, error)`  
  Retrieves data for a specific match by ID.

- `GetMatchTelemetry(shard string, matchID string) (*TelemetryData, error)`  
  Retrieves and processes telemetry data for a specific match.

#### Tournament Methods

- `GetTournaments() (*TournamentsResponse, error)`  
  Retrieves the list of all tournaments.

- `GetTournamentByID(tournamentID string) (*TournamentDetail, error)`  
  Retrieves detailed information about a specific tournament.

- `GetMatchIDsByTournamentID(tournamentID string) ([]string, error)`  
  Retrieves match IDs associated with a specific tournament.

#### Telemetry Processing

- `ProcessTelemetry(data []byte) (*TelemetryData, error)`  
  Parses telemetry data from a JSON byte array and extracts relevant information like circle phases and plane path.

### Usage Examples

```go
// Initialize the client
pubgClient := pubg.New(config.PUBG, redisCache)

// Get player data
playerResp, err := pubgClient.GetPlayersByNames("steam", []string{"PlayerName1", "PlayerName2"})

// Get match data
matchResp, err := pubgClient.GetMatch("steam", "match-id-123")

// Get and process telemetry data
telemetryData, err := pubgClient.GetMatchTelemetry("steam", "match-id-123")

// Get tournament data
tournaments, err := pubgClient.GetTournaments()
```

## 📄 License

This project is licensed under the MIT License.