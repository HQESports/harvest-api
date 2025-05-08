package database

import (
	"context"
	"fmt"
	"harvest/internal/model"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type DropSpotLocationDatabase interface {
	CreateDropSpotLocation(ctx context.Context, dropSpotLocation *model.DropSpotLocation) (*model.DropSpotLocation, error)
	GetDropSpotLocationByID(ctx context.Context, id string) (*model.DropSpotLocation, error)
	GetDropSpotLocationByMap(ctx context.Context, mapName string) ([]model.DropSpotLocation, error)
	UpdateDropSpotLocation(ctx context.Context, id string, dropSpotLocation *model.DropSpotLocation) error
	DeleteDropSpotLocation(ctx context.Context, id string) error
	BulkUpsertDropSpotLocations(ctx context.Context, dropSpotLocations []model.DropSpotLocation) error
}

func (m *mongoDB) CreateDropSpotLocation(ctx context.Context, dropSpotLocation *model.DropSpotLocation) (*model.DropSpotLocation, error) {
	createdLocation, err := m.dropSpotLocationsCol.InsertOne(ctx, dropSpotLocation)
	if err != nil {
		return nil, err
	}
	dropSpotLocation.ID = createdLocation.InsertedID.(primitive.ObjectID)
	return dropSpotLocation, nil
}

func (m *mongoDB) GetDropSpotLocationByID(ctx context.Context, id string) (*model.DropSpotLocation, error) {
	var dropSpotLocation model.DropSpotLocation
	filter := bson.M{"_id": id}
	err := m.dropSpotLocationsCol.FindOne(ctx, filter).Decode(&dropSpotLocation)
	if err != nil {
		return nil, err
	}
	return &dropSpotLocation, nil
}

func (m *mongoDB) GetDropSpotLocationByMap(ctx context.Context, mapName string) ([]model.DropSpotLocation, error) {
	var dropSpotLocations []model.DropSpotLocation
	filter := bson.M{"map_name": mapName}
	cursor, err := m.dropSpotLocationsCol.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var dropSpotLocation model.DropSpotLocation
		if err := cursor.Decode(&dropSpotLocation); err != nil {
			return nil, err
		}
		dropSpotLocations = append(dropSpotLocations, dropSpotLocation)
	}

	if err := cursor.Err(); err != nil {
		return nil, err
	}

	return dropSpotLocations, nil
}

func (m *mongoDB) UpdateDropSpotLocation(ctx context.Context, id string, dropSpotLocation *model.DropSpotLocation) error {
	filter := bson.M{"_id": id}
	update := bson.M{"$set": dropSpotLocation}
	_, err := m.dropSpotLocationsCol.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	return nil
}

func (m *mongoDB) DeleteDropSpotLocation(ctx context.Context, id string) error {
	filter := bson.M{"_id": id}
	_, err := m.dropSpotLocationsCol.DeleteOne(ctx, filter)
	if err != nil {
		return err
	}
	return nil
}

func (m *mongoDB) BulkUpsertDropSpotLocations(ctx context.Context, dropSpotLocations []model.DropSpotLocation) error {
	var operations []mongo.WriteModel
	for _, dropSpotLocation := range dropSpotLocations {
		// Update the UpdatedAt field to the current time
		dropSpotLocation.UpdatedAt = time.Now()

		// If it's a new record, also set CreatedAt
		if dropSpotLocation.ID.IsZero() {
			dropSpotLocation.CreatedAt = dropSpotLocation.UpdatedAt
			// Let MongoDB generate an ID if none exists
			dropSpotLocation.ID = primitive.NewObjectID()
		}

		filter := bson.M{"_id": dropSpotLocation.ID}

		// Instead of directly using the struct, create a document representation
		update := bson.D{
			{Key: "$set", Value: bson.D{
				{Key: "map_name", Value: dropSpotLocation.MapName},
				{Key: "names", Value: dropSpotLocation.Names},
				{Key: "updated_at", Value: dropSpotLocation.UpdatedAt},
				{Key: "created_at", Value: dropSpotLocation.CreatedAt},
				{Key: "x", Value: dropSpotLocation.X},
				{Key: "y", Value: dropSpotLocation.Y},
			}},
		}

		operation := mongo.NewUpdateOneModel().
			SetFilter(filter).
			SetUpdate(update).
			SetUpsert(true)

		operations = append(operations, operation)
	}

	log.Printf("Bulk upsert drop spot locations: %d", len(operations))

	if len(operations) > 0 {
		result, err := m.dropSpotLocationsCol.BulkWrite(ctx, operations)
		if err != nil {
			return fmt.Errorf("bulk write failed: %w", err)
		}

		log.Printf("Bulk write results: Matched: %d, Modified: %d, Inserted: %d",
			result.MatchedCount, result.ModifiedCount, result.UpsertedCount)
	}

	return nil
}
 