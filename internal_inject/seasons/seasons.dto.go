package seasons_inject

type SeasonResponse struct {
	ID      uint   `json:"id"`
	Name    string `json:"name"`
	StartAt string `json:"start_at"`
	EndAt   string `json:"end_at"`
}
