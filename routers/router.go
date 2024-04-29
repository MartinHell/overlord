package routers

import (
	"net/http"

	"github.com/MartinHell/overlord/controllers"
	"github.com/gin-gonic/gin"
)

func Route(r *gin.Engine) {

	v1 := "/api/v1"
	r.POST(v1+"/players", controllers.PlayerCreate)
	r.GET(v1+"/players", controllers.PlayerList)
	r.GET(v1+"/players/:id", controllers.PlayerShow)
	r.PUT(v1+"/players/:id", controllers.PlayerUpdate)
	r.GET(v1+"/players/:id/events", controllers.PlayerEvents)
	r.GET(v1+"/players/name/:id", controllers.PlayerSearchByName)
	r.GET(v1+"/players/ucid/:id", controllers.PlayerSearchByUCID)

	r.POST(v1+"/events", controllers.EventCreate)
	/* 	r.GET("/events", controllers.EventList)
	   	r.GET("/events/:id", controllers.EventShow)
	   	r.PUT("/events/:id", controllers.EventUpdate)
	   	r.DELETE("/events/:id", controllers.EventDelete) */

	r.POST(v1+"/units", controllers.UnitCreate)
	r.GET(v1+"/units", controllers.UnitList)
	r.GET(v1+"/units/:id", controllers.UnitShow)
	r.PUT(v1+"/units/:id", controllers.UnitUpdate)
	r.GET(v1+"/units/name/:id", controllers.UnitSearchByName)

	r.POST(v1+"/weapons", controllers.WeaponCreate)
	r.GET(v1+"/weapons", controllers.WeaponList)
	r.GET(v1+"/weapons/:id", controllers.WeaponShow)
	r.PUT(v1+"/weapons/:id", controllers.WeaponUpdate)
	r.GET(v1+"/weapons/name/:id", controllers.WeaponSearchByName)

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "pong",
		})
	})

}
