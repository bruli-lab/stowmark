package model

import "time"

type File struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}

type Manifest struct {
	ID          string      `json:"id"`
	Files       []File      `json:"files"`
	CreatedAt   time.Time   `json:"created_at"`
	Source      string      `json:"source"`
	Compression Compression `json:"compression"`
}
