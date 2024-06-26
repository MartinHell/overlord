package main

import (
	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/models"
)

func init() {
	initializers.LoadEnvVariables()
	initializers.ConnectToDB()
}

func main() {
	/* initializers.DB.Debug().AutoMigrate(&models.Event{}, &models.Player{}, &models.Unit{}, &models.Weapon{}) */
	initializers.DB.Debug().AutoMigrate(&models.Player{}, &models.Unit{}, &models.Weapon{}, &models.Target{}, &models.Event{})
}
