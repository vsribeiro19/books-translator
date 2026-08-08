package model

// Block is a single unit of extracted text: a heading or a paragraph.
type Block struct {
	Type  string `json:"type"`
	Level int    `json:"level"`
	Text  string `json:"text"`
}

// Chapter groups blocks under a title.
type Chapter struct {
	Title  string  `json:"title"`
	Blocks []Block `json:"blocks"`
}

// ExtractResult is the output of the pdf-service /extract endpoint.
type ExtractResult struct {
	PageCount int       `json:"pageCount"`
	Chapters  []Chapter `json:"chapters"`
}

// RebuildInput is the payload sent to the pdf-service /rebuild endpoint.
type RebuildInput struct {
	Title    string    `json:"title"`
	Chapters []Chapter `json:"chapters"`
}
