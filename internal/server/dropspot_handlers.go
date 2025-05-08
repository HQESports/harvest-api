package server

import (
	"harvest/internal/model"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

func (s *Server) CreateDropSpotLocationHandler(c *gin.Context) {
	var dropSpotLocation model.DropSpotLocation
	if err := c.ShouldBindJSON(&dropSpotLocation); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	createdLocation, err := s.dc.CreateDropSpotLocation(c.Request.Context(), &dropSpotLocation)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create drop spot location: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, createdLocation)
}

func (s *Server) GetDropSpotLocationByIDHandler(c *gin.Context) {
	id := c.Param("id")
	dropSpotLocation, err := s.dc.GetDropSpotLocationByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get drop spot location: " + err.Error()})
		return
	}

	if dropSpotLocation == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Drop spot location not found"})
		return
	}

	c.JSON(http.StatusOK, dropSpotLocation)
}

func (s *Server) GetDropSpotLocationByMapHandler(c *gin.Context) {
	mapName := c.Param("map_name")
	dropSpotLocations, err := s.dc.GetDropSpotLocationByMap(c.Request.Context(), mapName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get drop spot locations: " + err.Error()})
		return
	}

	if len(dropSpotLocations) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "No drop spot locations found for the specified map"})
		return
	}

	c.JSON(http.StatusOK, dropSpotLocations)
}

func (s *Server) UpdateDropSpotLocationHandler(c *gin.Context) {
	id := c.Param("id")
	var dropSpotLocation model.DropSpotLocation
	if err := c.ShouldBindJSON(&dropSpotLocation); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.dc.UpdateDropSpotLocation(c.Request.Context(), id, &dropSpotLocation); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update drop spot location: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, dropSpotLocation)
}

func (s *Server) DeleteDropSpotLocationHandler(c *gin.Context) {
	id := c.Param("id")
	if err := s.dc.DeleteDropSpotLocation(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete drop spot location: " + err.Error()})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

func (s *Server) BulkUpsertDropSpotLocationsHandler(c *gin.Context) {
	var dropSpotLocations []model.DropSpotLocation
	if err := c.ShouldBindJSON(&dropSpotLocations); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	log.Info().Int("locations_count", len(dropSpotLocations)).Msg("Bulk upsert drop spot locations")

	if err := s.dc.BulkUpsertDropSpotLocations(c.Request.Context(), dropSpotLocations); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to bulk upsert drop spot locations: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Bulk upsert successful"})
}
