# Harvest API Documentation

## Overview
Harvest is a Go-based API for collecting, storing, and analyzing PUBG (PlayerUnknown's Battlegrounds) match data. The API serves as a centralized harvesting system for player statistics, match data, and tournament information.

## Architecture

The API follows a layered architecture pattern:

```
┌─────────────┐
│    Server   │ HTTP Handlers, Routing
├─────────────┤
│ Controllers │ Business Logic
├─────────────┤
│  Database   │ Data Access Layer
└─────────────┘
```

### Server Layer
Manages HTTP requests/responses using the Gin web framework. Handles routing, validation, and error responses.

### Controller Layer
Contains the business logic for interacting with the PUBG API and orchestrating operations between the server and database layers.

### Database Layer
MongoDB interface for storing and retrieving PUBG data, including matches, players, and tournaments.

## Data Models

### Entity
A common structure representing players and tournaments:
```go
type Entity struct {
    ID     string `json:"id" bson:"id"`
    Name   string `json:"name" bson:"name"`
    Active bool   `json:"active" bson:"active"`
}
```

### Match
Represents a PUBG match with metadata:
```go
type Match struct {
    MatchID       string    `bson:"match_id,omitempty"`
    ShardID       string    `bson:"shard_id,omitempty"`
    MapName       string    `bson:"map_name,omitempty"`
    GameMode      string    `bson:"game_mode,omitempty"`
    Duration      int       `bson:"duration,omitempty"`
    IsCustomMatch bool      `bson:"is_custom_match,omitempty"`
    CreatedAt     time.Time `bson:"created_at,omitempty"`
    MatchType     string    `bson:"match_type,omitempty"`
    
    // Processing metadata
    Processed   bool      `bson:"processed,omitempty"`
    ProcessedAt time.Time `bson:"processed_at,omitempty"`
    ImportedAt  time.Time `bson:"imported_at,omitempty"`
    
    // Statistics
    PlayerCount int `bson:"player_count,omitempty"`
    TeamCount   int `bson:"team_count,omitempty"`
    
    TelemetryURL string `bson:"telemetry_url,omitempty"`
}
```

### TrainData
Used for storing telemetry data for machine learning:
```go
type TrainData struct {
    MatchID       string             `json:"matchId" bson:"matchId"`
    MapName       string             `json:"mapName" bson:"mapName"`
    MatchType     string             `json:"matchType" bson:"matchType"`
    TelemetryData pubg.TelemetryData `json:"telemetryData" bson:"telemetryData"`
}
```

### MatchFilter
Used for filtering PUBG matches:
```go
type MatchFilter struct {
    MapName   string `json:"mapName"`
    MatchType string `json:"matchType"`
    Limit     int    `json:"limit"`
}
```

## Server Setup

The server is initialized with configuration and required dependencies:

```go
func New(config *config.Config, db database.Database, client pubg.Client) *http.Server {
    sc := controller.NewServer(db)
    pc := controller.NewPUBG(db, client)

    server := Server{
        sc: sc,
        pc: pc,
    }

    return &http.Server{
        Addr:         fmt.Sprintf(":%v", config.Port),
        Handler:      server.RegisterRoutes(),
        IdleTimeout:  time.Minute,
        ReadTimeout:  10 * time.Second,
        WriteTimeout: 10 * time.Second,
    }
}
```

The server uses Gin for routing and includes CORS configuration:

```go
func (s *Server) RegisterRoutes() http.Handler {
    r := gin.Default()

    r.Use(cors.New(cors.Config{
        AllowOrigins:     []string{"http://localhost:5173"}, // Frontend URL
        AllowMethods:     []string{"GET", "POST"},
        AllowHeaders:     []string{"Accept", "Authorization", "Content-Type"},
        AllowCredentials: true, // Enable cookies/auth
    }))

    // Register routes
    r.GET("/health", s.healthHandler)
    r.GET("/online", s.onlineHandler)
    r.POST("/names", s.namesHandler)
    r.POST("/buildMatches", s.BuildMatchesFromFilter)
    r.POST("/tournaments", s.tournamentsHandler)
    r.POST("/expandPlayers", s.expandPlayers)

    return r
}
```

## API Endpoints

### Health Checks

#### GET /health
Checks the health of the service and database.

**Response:**
- 200 OK: "Database Online"
- 500 Internal Server Error: "Database Offline"

#### GET /online
Checks if the service is online.

**Response:**
- 200 OK: "Online"

### Player Management

#### POST /names
Creates or updates players in the database.

**Request Body:**
```json
{
  "names": "player1,player2,player3"
}
```

**Response:**
- 201 Created:
```json
{
  "playersProcessed": 3
}
```
- 400 Bad Request: Invalid request format
- 500 Internal Server Error: Error processing the request

#### POST /expandPlayers
Searches for matches and extracts new players to add to the database.

**Query Parameters:**
- `limit`: Maximum number of players to process

**Response:**
- 201 Created:
```json
{
  "playersUpserted": 42
}
```
- 400 Bad Request: Invalid limit parameter
- 500 Internal Server Error: Error processing the request

### Tournament Management

#### POST /tournaments
Fetches tournament data from the PUBG API and stores it in the database.

**Response:**
- 201 Created:
```json
{
  "tournamentsProcessed": 15
}
```
- 500 Internal Server Error: Error processing the request

### Match Management

#### POST /buildMatches
Builds matches based on filtering criteria.

**Request Body:**
```json
{
  "mapName": "Erangel_Main",
  "matchType": "live",
  "limit": 100
}
```

**Valid Map Names:**
- Baltic_Main
- Chimera_Main
- Desert_Main
- DihorOtok_Main
- Erangel_Main
- Heaven_Main
- Kiki_Main
- Range_Main
- Savage_Main
- Summerland_Main
- Tiger_Main
- Neon_Main

**Valid Match Types:**
- live
- event

**Response:**
- 201 Created:
```json
{
  "numMatches": 42,
  "successful": 40
}
```
- 400 Bad Request: Invalid filter parameters
- 500 Internal Server Error: Error processing the request

## Database Operations

The API provides the following database operations:

### Player/Tournament Operations
- `BulkUpsertEntities`: Add or update multiple entities (players or tournaments)
- `GetActivePlayers`: Retrieve active players
- `GetActiveTournaments`: Retrieve active tournaments

### Match Operations
- `ImportMatch`: Import a single match
- `ImportMatches`: Import multiple matches
- `GetProcessedMatchIDs`: Get IDs of processed matches
- `GetUnprocessedMatches`: Get matches that haven't been processed
- `GetMatchesByType`: Get matches of a specific type
- `MarkMatchAsProcessed`: Mark a match as processed

### Telemetry Operations
- `UpsertTrainData`: Add or update training data

## Controller Functions

### Server Controller
The `ServerController` interface handles basic server management functions:

```go
type ServerController interface {
    Health() (string, error)
    Online() string
}
```

- `Health()`: Checks the database connection and returns a status message
- `Online()`: Returns the server's running status

### PUBG Controller
The `PubgController` interface handles PUBG data harvesting:

```go
type PubgController interface {
    CreatePlayers([]string) (int, error)
    CreateTournaments() (int, error)
    SearchAndExpandPlayers(int) (int, error)
}
```

- `CreatePlayers`: Create player entities from names
- `CreateTournaments`: Fetch tournament details and create entity records
- `SearchAndExpandPlayers`: Search for matches and extract new players

## Performance Considerations

The API includes performance optimizations:

- **Concurrent Processing**: Uses goroutines with controlled concurrency for processing match data
- **Bulk Operations**: Uses MongoDB bulk operations for efficient database updates
- **Progress Tracking**: Implements JobMetrics for tracking long-running operations
- **Rate Limiting**: Implements semaphores to control API request rates

## Configuration

The API uses a structured JSON configuration system:

```go
type Config struct {
    Env     string        `json:"env"`
    Port    int           `json:"port"`
    PUBG    PUBGConfig    `json:"pubg"`
    MongoDB MongoDBConfig `json:"mongodb"`
    Logging LoggingConfig `json:"logging"`
    S3      S3Config      `json:"s3"`
}
```

### PUBG API Configuration
```go
type PUBGConfig struct {
    APIKey            string            `json:"api_key"`
    BaseURL           string            `json:"base_url"`
    Shards            map[string]string `json:"shards"`
    RequestDelay      int               `json:"requestDelay"`
    MaxRetries        int               `json:"maxRetries"`
    RequestsPerMinute int               `json:"requestsPerMinute"`
    OldEnoughCap      int               `json:"oldEnoughCap"`
}
```

### MongoDB Configuration
```go
type MongoDBConfig struct {
    URI      string                 `json:"uri"`
    Username string                 `json:"username"`
    Password string                 `json:"password"`
    DB       string                 `json:"db"`
    Options  map[string]interface{} `json:"options"`
}
```

### Logging Configuration
```go
type LoggingConfig struct {
    Level     string `json:"level"`
    Format    string `json:"format"`
    Directory string `json:"directory"`
}
```

### S3 Configuration
```go
type S3Config struct {
    AccessKeyID     string         `json:"accessKeyId"`
    SecretAccessKey string         `json:"secretAccessKey"`
    Region          string         `json:"region"`
    Bucket          string         `json:"bucket"`
    Endpoint        string         `json:"endpoint"`
    ForcePathStyle  bool           `json:"forcePathStyle"`
    UploadOptions   map[string]int `json:"uploadOptions"`
}
```

Configuration is loaded from a JSON file using the `LoadConfig()` function:

```go
func LoadConfig(filePath string) (*Config, error)
```

## PUBG Client Library

The API includes a custom PUBG client library that handles API communications with rate limiting, request tracking, and extensive logging. This library is located in the `pkg/pubg` package.

### Core Components

#### Client
The main client struct that handles API communications:

```go
type Client struct {
    httpClient    *http.Client
    apiKey        string
    baseURL       string
    OldEnoughMin  int
    requestTicker *time.Ticker
    requestChan   chan struct{}
}
```

- Implements rate limiting with token bucket approach
- Handles request retries
- Provides extensive logging for monitoring and debugging
- Supports both rate-limited and non-rate-limited requests

#### Data Models

**Player Data:**
- `PUBGPlayerResponse`: Root structure for player data
- `PlayerData`: Information about a player
- `PlayerAttributes`: Player details (name, stats, etc.)

**Match Data:**
- `PUBGMatchResponse`: Root structure for match data
- `MatchData`: Information about a match
- `MatchAttributes`: Match details (duration, mode, map, etc.)

**Telemetry Data:**
- `TelemetryData`: Extracted and processed telemetry data
- `CirclePhase`: Data for a single circle phase
- `PlanePath`: Airplane trajectory
- Various event types for telemetry processing

**Tournament Data:**
- `TournamentsResponse`: Response from the tournaments endpoint
- `TournamentData`: Information about a tournament
- `TournamentDetail`: Detailed tournament information

### Key Functions

#### Player Functions
- `GetPlayersByNames`: Get data for up to 10 players by in-game names
- `GetPlayersByIDs`: Get data for up to 10 players by account IDs
- `GetPlayerIDs`: Get a map of player names to their IDs (batched)
- `GetMatchIDsForPlayers`: Get all unique match IDs for a list of players

#### Match Functions
- `GetMatch`: Retrieve data for a specific match by ID
- `IsValidMatch`: Check if a match is valid based on rules
- `GetMatchType`: Determine match type (ranked, scrim, etc.)
- `IsMatchOldEnough`: Check if match duration meets threshold

#### Telemetry Functions
- `GetTelemetry`: Retrieve telemetry file (with optional rate limiting)
- `GetMatchTelemetry`: Retrieve and process telemetry for a match
- `ProcessTelemetry`: Extract relevant data from telemetry

#### Tournament Functions
- `GetTournaments`: Retrieve list of all tournaments
- `GetTournamentByID`: Get detailed information about a tournament
- `GetMatchIDsByTournamentID`: Get match IDs associated with a tournament
- `BuildTournamentName`: Construct readable tournament name from ID

### Usage Example

```go
import (
    "harvest/pkg/pubg"
)

// Initialize client with configuration
client := pubg.New(
    config.PUBG.APIKey,
    config.PUBG.BaseURL,
    config.PUBG.OldEnoughCap,
    config.PUBG.RequestsPerMinute,
)

// Get player IDs
playerIDs, err := client.GetPlayerIDs(pubg.SteamPlatform, []string{"PlayerName1", "PlayerName2"})
if err != nil {
    // Handle error
}

// Get match data
match, err := client.GetMatch(pubg.SteamPlatform, "match-id")
if err != nil {
    // Handle error
}

// Get and process telemetry
telemetry, err := client.GetMatchTelemetry(pubg.SteamPlatform, "match-id")
if err != nil {
    // Handle error
}
```

## Error Handling

The API uses structured logging via zerolog for comprehensive error tracking. All operations log their progress, success, and failure states with relevant contextual information such as job IDs, match IDs, and timing metrics.

### Logging Features

- **Operation IDs**: Each major operation gets a unique ID for tracing
- **Structured Metrics**: Duration, counts, and rates for performance monitoring
- **Phase Tracking**: Multi-phase operations track timing for each phase
- **Error Context**: Detailed error information with relevant context
- **Progress Updates**: Long-running operations log periodic progress updates

### Example Log Output

```
2023-06-01T12:34:56Z INFO Request completed {"request_id": "req_1234567890", "endpoint": "/shards/steam/players", "status_code": 200, "response_size": 8192, "duration": 0.354}
```