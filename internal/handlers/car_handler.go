package handlers

import (
	"car-garage-service/internal/services"

	"gofr.dev/pkg/gofr"
)

type CarHandler struct {
	carService *services.CarService
}

func NewCarHandler(carService *services.CarService) *CarHandler {
	return &CarHandler{carService: carService}
}

func (h *CarHandler) ListCars(ctx *gofr.Context) (interface{}, error) {
	return h.carService.ListCars(), nil
}

func (h *CarHandler) AddCar(ctx *gofr.Context) (interface{}, error) {
	var newCar services.Car
	if err := ctx.BindJSON(&newCar); err != nil {
		return nil, err
	}

	return h.carService.AddCar(newCar), nil
}

func (h *CarHandler) UpdateCar(ctx *gofr.Context) (interface{}, error) {
	id := ctx.Param("id")
	var updatedCar services.Car
	if err := ctx.BindJSON(&updatedCar); err != nil {
		return nil, err
	}

	return h.carService.UpdateCar(id, updatedCar), nil
}

func (h *CarHandler) DeleteCar(ctx *gofr.Context) (interface{}, error) {
	id := ctx.Param("id")
	h.carService.DeleteCar(id)
	return nil, nil
}
