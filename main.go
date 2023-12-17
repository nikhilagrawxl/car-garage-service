package main

import (
	"car-garage-service/internal/handlers"
	"log"

	"gofr.dev/pkg/gofr"
)

func main() {
	app := gofr.New()

	carHandler := &handlers.CarHandler{}

	app.GET("/cars", carHandler.ListCars)
	app.POST("/cars", carHandler.AddCar)
	app.PUT("/cars/:id", carHandler.UpdateCar)
	app.DELETE("/cars/:id", carHandler.DeleteCar)

	// Start the server
	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
