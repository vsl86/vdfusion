package media

type Metadata struct {
	Duration float64
	Width    int
	Height   int
	Codec    string
	Bitrate  int64
	FPS      float64
	Warnings []string
}
