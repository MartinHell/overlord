package routers

import (
	"net/http"

	"github.com/MartinHell/overlord/controllers"
	"github.com/gin-gonic/gin"
)

func Route(r *gin.Engine) {

	v1 := "/api/v1"
	r.GET(v1+"/players", controllers.GetPlayers)
	r.GET(v1+"/players/:id", controllers.GetPlayer)
	r.GET(v1+"/players/:id/events", controllers.GetPlayerEvents)
	r.GET(v1+"/players/name/:id", controllers.GetPlayerByName)
	r.GET(v1+"/players/ucid/:id", controllers.GetPlayerByUCID)

	r.GET(v1+"/events", controllers.GetEvents)
	r.GET(v1+"/events/:id", controllers.GetEvent)

	r.GET(v1+"/units", controllers.ApiGetUnits)
	r.GET(v1+"/units/:id", controllers.ApiGetUnit)
	r.GET(v1+"/units/name/:id", controllers.ApiGetUnitByName)

	r.GET(v1+"/weapons", controllers.GetWeapons)
	r.GET(v1+"/weapons/:id", controllers.GetWeapon)
	r.GET(v1+"/weapons/name/:id", controllers.GetWeaponByName)

	r.GET("/healthcheck", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "OK",
		})
	})

}
