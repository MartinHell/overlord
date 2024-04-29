package routers

import (
	"net/http"

	"github.com/MartinHell/overlord/controllers"
	"github.com/gin-gonic/gin"
)

func Route(r *gin.Engine) {

	r.POST("/players", controllers.PlayerCreate)
	r.GET("/players", controllers.PlayerList)
	r.GET("/players/:id", controllers.PlayerShow)
	r.PUT("/players/:id", controllers.PlayerUpdate)
	r.DELETE("/players/:id", controllers.PlayerDelete)
	r.GET("/players/:id/events", controllers.PlayerEvents)
	r.GET("/players/search", controllers.PlayerSearch)

	r.POST("/events", controllers.EventCreate)
	/* 	r.GET("/events", controllers.EventList)
	   	r.GET("/events/:id", controllers.EventShow)
	   	r.PUT("/events/:id", controllers.EventUpdate)
	   	r.DELETE("/events/:id", controllers.EventDelete) */

	r.POST("/units", controllers.UnitCreate)
	r.GET("/units", controllers.UnitList)
	r.GET("/units/:id", controllers.UnitShow)
	r.PUT("/units/:id", controllers.UnitUpdate)

	r.POST("/weapons", controllers.WeaponCreate)
	r.GET("/weapons", controllers.WeaponList)
	r.GET("/weapons/:id", controllers.WeaponShow)
	r.PUT("/weapons/:id", controllers.WeaponUpdate)
	r.DELETE("/weapons/:id", controllers.WeaponDelete)

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "pong",
		})
	})

}
