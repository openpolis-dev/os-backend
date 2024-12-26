package publicity_inject

type CreateReq struct {
	Title   string `json:"title"` // base64 encoded image string
	Content string `json:"content"`

	IsSaveDraft bool `json:"isSaveDraft"`
	ID          uint `json:"id"`
}

type UpdateReq struct {
	ID      uint   `json:"id"`
	Title   string `json:"title"` // base64 encoded image string
	Content string `json:"content"`
}
