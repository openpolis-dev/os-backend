package push_inject

type CreateReq struct {
	// TODO support multi language
	//Title   map[string]string `json:"title"`   // [zh]你好,[en]Hello
	//Content map[string]string `json:"content"` // [zh]你好,[en]Hello
	Title   string `json:"title"`
	Content string `json:"content"`

	JumpURL string `json:"jump_url"`

	//PushDate time.Time `json:"push_date"`
}
