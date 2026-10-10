// I will use this in the future for the containers and volumes Repositories
package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

var (
	ErrNotFound = errors.New("record not found")
)

type Record interface {
	GetID() string
	GetName() string
}

type Repository[T Record] struct {
	FileName string
}

func NewRepository[T Record](FileName string) *Repository[T] {
	return &Repository[T]{
		FileName: FileName,
	}
}

func (r *Repository[T]) All() ([]T, error) {
	data, err := os.ReadFile(r.FileName)

	if errors.Is(err, os.ErrNotExist) {
		return []T{}, nil
	}
	if err != nil {
		return nil, err
	}

	var records []T

	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}

	return records, nil
}

func (r *Repository[T]) write(records []T) error {
	data, err := json.MarshalIndent(records, "", "  ")

	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(r.FileName), 0755); err != nil {
		return err
	}

	return os.WriteFile(r.FileName, data, 0644)
}

func (r *Repository[T]) Save(record T) error {
	records, err := r.All()
	if err != nil {
		return err
	}

	for i, curr := range records {
		if curr.GetName() == record.GetName() {
			records[i] = record
			return r.write(records)
		}
	}

	records = append(records, record)
	return r.write(records)
}

func (r *Repository[T]) GetByName(name string) (T, error) {
	records, err := r.All()
	if err != nil {
		var zero T
		return zero, err
	}

	for _, record := range records {
		if record.GetName() == name {
			return record, nil
		}
	}

	var zero T
	return zero, ErrNotFound
}

func (r *Repository[T]) DeleteByName(name string) (T, error) {
	records, err := r.All()
	if err != nil {
		var zero T
		return zero, err
	}

	for i, record := range records {
		if record.GetName() == name {
			updated := append(records[:i:i], records[i+1:]...)
			if err := r.write(updated); err != nil {
				var zero T
				return zero, err
			}
			return record, nil
		}
	}

	var zero T
	return zero, ErrNotFound
}
