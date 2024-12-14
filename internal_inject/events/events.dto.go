package events_inject

type (
	CreateOrUpdateReq struct {
		Title    string `json:"title"`
		CoverImg string `json:"cover_img"`
		Content  string `json:"content"`

		StartAt string `json:"start_at"`
		EndAt   string `json:"end_at"`

		Metadata string `json:"metadata"`
	}
)
