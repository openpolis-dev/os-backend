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

type PublicityInfo struct {
	ID       uint   `json:"id"`
	Title    string `json:"title"`
	Content  string `json:"content"`
	Creator  string `json:"creator"`
	Avatar   string `json:"avatar"`
	UpdateAt int64  `json:"updateAt"`
	IsDel    int    `json:"isDel"`
	Season   int    `json:"season"`
	IsDraft  int    `json:"isDraft"`
}

type PublicityLogInfo struct {
	ID       uint   `json:"id"`
	UpdateAt int64  `json:"updateAt"`
	Eidtor   string `json:"eidtor"`
	Avatar   string `json:"avatar"`
}

type PublicityDetail struct {
	Detail *PublicityInfo
	Log    []*PublicityLogInfo
}
