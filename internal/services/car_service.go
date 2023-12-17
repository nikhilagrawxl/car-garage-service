package services

import "strconv"

type Car struct {
	ID    int    `json:"id"`
	Model string `json:"model"`
}

type CarService struct {
	cars []Car
}

func NewCarService() *CarService {
	return &CarService{}
}

func (s *CarService) ListCars() []Car {
	return s.cars
}

func (s *CarService) AddCar(newCar Car) Car {
	newCar.ID = len(s.cars) + 1
	s.cars = append(s.cars, newCar)
	return newCar
}

func (s *CarService) UpdateCar(id string, updatedCar Car) Car {
	for i, car := range s.cars {
		if strconv.Itoa(car.ID) == id {
			s.cars[i] = updatedCar
			return updatedCar
		}
	}
	return Car{}
}

func (s *CarService) DeleteCar(id string) {
	for i, car := range s.cars {
		if strconv.Itoa(car.ID) == id {
			s.cars = append(s.cars[:i], s.cars[i+1:]...)
			return
		}
	}
}
