package model

type Compression struct {
	Type  string `json:"type"`
	Level *int   `json:"level,omitempty"`
}
type Config struct {
	ID            string      `json:"id"`
	FormatVersion int         `json:"format_version"`
	CreatedAt     string      `json:"created_at"`
	Compression   Compression `json:"compression"`
}
