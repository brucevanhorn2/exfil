package transfer

type Direction string

const (
	DirectionDownload Direction = "download"
	DirectionUpload   Direction = "upload"
)

type Job struct {
	ID           int
	Direction    Direction
	SourcePath   string
	DestPath     string
	Filename     string
}
