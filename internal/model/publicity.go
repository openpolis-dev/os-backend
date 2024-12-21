package model

type Publicity struct {
	ID       uint   `json:"id" gorm:"primaryKey"`
	CreateAt int64  `json:"createAt" gorm:"index"`
	Title    string `json:"title"`
	Content  string `json:"content"`
	Creator  string `json:"creator"`
	UpdateAt int64  `json:"updateAt"`
	IsDel    int    `json:"isDel"`
	Eidtor   string `json:"editor"`
}
