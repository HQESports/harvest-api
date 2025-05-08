package controller

import (
	"context"
	"fmt"
	"harvest/internal/database"
	"harvest/internal/model"
	"harvest/pkg/pubg"
)

type DropSpotLocationController interface {
	CreateDropSpotLocation(ctx context.Context, dropSpotLocation *model.DropSpotLocation) (*model.DropSpotLocation, error)
	GetDropSpotLocationByID(ctx context.Context, id string) (*model.DropSpotLocation, error)
	GetDropSpotLocationByMap(ctx context.Context, mapName string) ([]model.DropSpotLocation, error)
	UpdateDropSpotLocation(ctx context.Context, id string, dropSpotLocation *model.DropSpotLocation) error
	DeleteDropSpotLocation(ctx context.Context, id string) error
	BulkUpsertDropSpotLocations(ctx context.Context, dropSpotLocations []model.DropSpotLocation) error
}

type dropSpotLocationController struct {
	db database.DropSpotLocationDatabase
}

func NewDropSpotLocationController(db database.DropSpotLocationDatabase) DropSpotLocationController {
	return &dropSpotLocationController{
		db: db,
	}
}

func (c *dropSpotLocationController) CreateDropSpotLocation(ctx context.Context, dropSpotLocation *model.DropSpotLocation) (*model.DropSpotLocation, error) {
	if pubg.MAP_NAME_MAP[dropSpotLocation.MapName] == "" {
		return nil, fmt.Errorf("invalid map name: %s", dropSpotLocation.MapName)
	}

	return c.db.CreateDropSpotLocation(ctx, dropSpotLocation)
}

func (c *dropSpotLocationController) GetDropSpotLocationByID(ctx context.Context, id string) (*model.DropSpotLocation, error) {
	return c.db.GetDropSpotLocationByID(ctx, id)
}

func (c *dropSpotLocationController) GetDropSpotLocationByMap(ctx context.Context, mapName string) ([]model.DropSpotLocation, error) {
	if pubg.MAP_NAME_MAP[mapName] == "" {
		return nil, fmt.Errorf("invalid map name: %s", mapName)
	}
	return c.db.GetDropSpotLocationByMap(ctx, mapName)
}

func (c *dropSpotLocationController) UpdateDropSpotLocation(ctx context.Context, id string, dropSpotLocation *model.DropSpotLocation) error {
	return c.db.UpdateDropSpotLocation(ctx, id, dropSpotLocation)
}

func (c *dropSpotLocationController) DeleteDropSpotLocation(ctx context.Context, id string) error {
	return c.db.DeleteDropSpotLocation(ctx, id)
}

func (c *dropSpotLocationController) BulkUpsertDropSpotLocations(ctx context.Context, dropSpotLocations []model.DropSpotLocation) error {
	if len(dropSpotLocations) == 0 {
		return nil
	}

	for _, dropSpotLocation := range dropSpotLocations {
		if pubg.MAP_NAME_MAP[dropSpotLocation.MapName] == "" {
			return fmt.Errorf("invalid map name: %s", dropSpotLocation.MapName)
		}
	}

	return c.db.BulkUpsertDropSpotLocations(ctx, dropSpotLocations)
}
