package controller

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"harvest/internal/aws"
	"harvest/internal/database"
	"harvest/internal/model"
)

// TeamController handles operations related to teams
type TeamController struct {
	teamDB      database.Database
	fileService aws.FileService
}

// NewTeamController creates a new team controller
func NewTeamController(teamDB database.Database, fileService aws.FileService) *TeamController {
	return &TeamController{
		teamDB:      teamDB,
		fileService: fileService,
	}
}

// CreateTeam creates a new team with image upload to S3
func (c *TeamController) CreateTeam(ctx context.Context, team *model.Team, imageData io.Reader, imageFileName string) error {
	log.Debug().Str("teamName", team.Name).Msg("Creating new team")

	// Handle image upload if provided
	if imageData != nil && imageFileName != "" {
		imageURL, err := c.uploadTeamImage(imageFileName, imageData)
		if err != nil {
			log.Error().Err(err).Str("teamName", team.Name).Msg("Failed to upload team image")
			return fmt.Errorf("failed to upload team image: %w", err)
		}
		team.ImageURL = imageURL
	}

	// Create the team in the database
	err := c.teamDB.CreateTeam(ctx, team)
	if err != nil {
		log.Error().Err(err).Str("teamName", team.Name).Msg("Failed to create team in database")
		return err
	}

	return nil
}

// GetTeamByID retrieves a team by its ID
func (c *TeamController) GetTeamByID(ctx context.Context, id primitive.ObjectID) (*model.Team, error) {
	log.Debug().Str("teamID", id.Hex()).Msg("Retrieving team by ID")

	team, err := c.teamDB.GetTeamByID(ctx, id)
	if err != nil {
		log.Error().Err(err).Str("teamID", id.Hex()).Msg("Failed to retrieve team")
		return nil, err
	}

	if team == nil {
		log.Debug().Str("teamID", id.Hex()).Msg("Team not found")
		return nil, nil
	}

	return team, nil
}

// ListTeams retrieves all teams
func (c *TeamController) ListTeams(ctx context.Context) ([]model.Team, error) {
	log.Debug().Msg("Listing all teams")

	teams, err := c.teamDB.ListTeams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list teams")
		return nil, err
	}

	return teams, nil
}

// UpdateTeam updates an existing team with optional image update
func (c *TeamController) UpdateTeam(ctx context.Context, team *model.Team, imageData io.Reader, imageFileName string) error {
	log.Debug().Str("teamID", team.ID.Hex()).Str("teamName", team.Name).Msg("Updating team")

	// First, retrieve the current team to check if we need to update the image
	currentTeam, err := c.teamDB.GetTeamByID(ctx, team.ID)
	if err != nil {
		log.Error().Err(err).Str("teamID", team.ID.Hex()).Msg("Failed to retrieve current team data")
		return err
	}

	if currentTeam == nil {
		log.Error().Str("teamID", team.ID.Hex()).Msg("Team not found for update")
		return fmt.Errorf("team not found")
	}

	// Handle image upload if provided
	if imageData != nil && imageFileName != "" {
		imageURL, err := c.uploadTeamImage(imageFileName, imageData)
		if err != nil {
			log.Error().Err(err).Str("teamID", team.ID.Hex()).Msg("Failed to upload team image")
			return fmt.Errorf("failed to upload team image: %w", err)
		}
		team.ImageURL = imageURL
	} else if team.ImageURL == "" {
		// If no new image is provided and image URL is empty, keep the existing image URL
		team.ImageURL = currentTeam.ImageURL
	}

	// Update the team in the database
	err = c.teamDB.UpdateTeam(ctx, team)
	if err != nil {
		log.Error().Err(err).Str("teamID", team.ID.Hex()).Msg("Failed to update team in database")
		return err
	}

	return nil
}

// DeleteTeam deletes a team by its ID
func (c *TeamController) DeleteTeam(ctx context.Context, id primitive.ObjectID) error {
	log.Debug().Str("teamID", id.Hex()).Msg("Deleting team")

	// Note: We're not deleting the image from S3 here
	// You might want to add that functionality if needed

	err := c.teamDB.DeleteTeam(ctx, id)
	if err != nil {
		log.Error().Err(err).Str("teamID", id.Hex()).Msg("Failed to delete team")
		return err
	}

	return nil
}

// Helper method to upload a team image to S3
func (c *TeamController) uploadTeamImage(filename string, data io.Reader) (string, error) {
	// Generate a unique filename to avoid conflicts
	ext := filepath.Ext(filename)
	baseFilename := strings.TrimSuffix(filepath.Base(filename), ext)
	timestamp := time.Now().UnixNano()
	uniqueFilename := fmt.Sprintf("teams/%s_%d%s", baseFilename, timestamp, ext)

	// Upload the file
	imageURL, err := c.fileService.UploadFile(uniqueFilename, data)
	if err != nil {
		return "", err
	}

	return imageURL, nil
}

func (c *TeamController) GetTeamRotations(context context.Context, teamID string, startDate time.Time, endDate time.Time) ([]model.TeamRotationTiny, error) {
	rotations, err := c.teamDB.GetTeamRotations(context, teamID, startDate, endDate)
	if err != nil {
		return nil, err
	}

	return rotations, nil
}

func (c *TeamController) GetRotationByID(context context.Context, rotationID string) (*model.TeamRotation, error) {
	id, err := primitive.ObjectIDFromHex(rotationID)
	if err != nil {
		return nil, err
	}
	return c.teamDB.GetRotationByID(context, id)
}
