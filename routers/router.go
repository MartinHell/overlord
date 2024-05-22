package routers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func Route(r *gin.Engine) {

	/* 	v1 := "/api/v1" */
	/* 	r.POST(v1+"/players", controllers.CreatePlayer)
		r.GET(v1+"/players", controllers.GetPlayers)
		r.GET(v1+"/players/hits/:id", controllers.GetPlayerHits)
		r.GET(v1+"/players/:id", controllers.GetPlayer)
		r.PUT(v1+"/players/:id", controllers.UpdatePlayer)
		r.GET(v1+"/players/:id/events", controllers.GetPlayerEvents)
		r.GET(v1+"/players/name/:id", controllers.GetPlayerByName)
		r.GET(v1+"/players/ucid/:id", controllers.GetPlayerByUCID)

	 	r.POST(v1+"/events", controllers.CreateEvent)
		r.GET(v1+"/events", controllers.GetEvents)
		r.GET("/events/:id", controllers.EventShow)
		r.PUT("/events/:id", controllers.EventUpdate)
		r.DELETE("/events/:id", controllers.EventDelete) */

	/* 	r.POST(v1+"/units", controllers.CreateUnit)
	   	r.GET(v1+"/units", controllers.GetUnits)
	   	r.GET(v1+"/units/:id", controllers.GetUnit)
	   	r.PUT(v1+"/units/:id", controllers.UpdateUnit)
	   	r.GET(v1+"/units/name/:id", controllers.GetUnitByName) */

	/* 	r.POST(v1+"/weapons", controllers.CreateWeapon)
	   	r.GET(v1+"/weapons", controllers.GetWeapons)
	   	r.GET(v1+"/weapons/:id", controllers.GetWeapon)
	   	r.PUT(v1+"/weapons/:id", controllers.UpdateWeapon)
	   	r.GET(v1+"/weapons/name/:id", controllers.GetWeaponByName) */

	r.GET("/healthcheck", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "OK",
		})
	})

}
