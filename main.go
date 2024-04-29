package main

import (
	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/routers"
	"github.com/gin-gonic/gin"
)

func init() {
	initializers.LoadEnvVariables()

	initializers.ConnectToDB()
}

func main() {

	r := gin.Default()

	routers.Route(r)

	r.Run()
}
