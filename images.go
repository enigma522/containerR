package main

import (
	"time"
)

type imageMetadata struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"create_at"`
}

func (i imageMetadata) GetID() string {
	return i.ID
}

func (i imageMetadata) GetName() string {
	return i.Name
}
