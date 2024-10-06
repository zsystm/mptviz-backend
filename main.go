package main

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/zsystm/mpt/api"
	"github.com/zsystm/mpt/db"
	"github.com/zsystm/mpt/handlers"
)

func main() {
	e := echo.New()
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"http://educhain.guru", "https://educhain.guru"},
		AllowHeaders: []string{
			echo.HeaderOrigin,
			echo.HeaderContentType,
			echo.HeaderAccept,
			echo.HeaderAuthorization,
		},
		AllowMethods: []string{echo.GET, echo.POST, echo.DELETE},
	}))

	sessDB := db.NewInMemorySessionStorage()
	mptHandler := handlers.NewMPTHandler(sessDB)
	mptApi := api.NewApiMPT(mptHandler)

	v1 := e.Group("/v1/mpt")
	// Define the API routes
	{
		v1.POST("/session", mptApi.CreateSession)
		v1.GET("/mpt", mptApi.GetMPT)
		v1.POST("/insert", mptApi.Insert)
		//v1.PUT("/update/:key", api.UpdateMPT)
		v1.DELETE("/delete", mptApi.Delete)
		//v1.POST("/rollback", api.RollbackMPT)
	}
	e.Logger.Fatal(e.Start(":1323"))
}
