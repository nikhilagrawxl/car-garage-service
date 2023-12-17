package storage

import "sync"

type Database struct {
	sync.Mutex
	Cars []Car
}

func NewDatabase() *Database {
	return &Database{}
}

func (db *Database) ListCars() []Car {
	db.Lock()
	defer db.Unlock()
	return db.Cars
}

func (db *Database) AddCar(newCar Car) {
	db.Lock()
	defer db.Unlock()
	newCar.ID = len(db.Cars) + 1
	db.Cars = append(db.Cars, newCar)
}

func (db *Database) UpdateCar(id int, updatedCar Car) bool {
	db.Lock()
	defer db.Unlock()
	for i, car := range db.Cars {
		if car.ID == id {
			db.Cars[i] = updatedCar
			return true
		}
	}
	return false
}

func (db *Database) DeleteCar(id int) bool {
	db.Lock()
	defer db.Unlock()
	for i, car := range db.Cars {
		if car.ID == id {
			db.Cars = append(db.Cars[:i], db.Cars[i+1:]...)
			return true
		}
	}
	return false
}
