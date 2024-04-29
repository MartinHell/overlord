package controllers

import (
	"net/http"

	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/models"
	"github.com/gin-gonic/gin"
)

func EventCreate(c *gin.Context) {
	// Get data off req body
	var event models.Event

	if err := c.ShouldBindJSON(&event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate data

	// Create player
	if result := initializers.DB.Create(&event); result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}

	// Return player
	c.JSON(http.StatusOK, gin.H{"message": "Event created successfully!", "event": event})
}
