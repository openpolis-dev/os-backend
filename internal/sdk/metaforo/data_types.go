package metaforo

import (
	"strconv"
	"time"
)

type ApiResponseWrapper struct {
	Status      bool   `json:"status"`
	Code        int    `json:"code"`
	Description string `json:"description"`
	Server      string `json:"server"`
}

type Tags struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

type UserData struct {
	Id        int           `json:"id"`
	PhotoUrl  string        `json:"photo_url"`
	UserTitle interface{}   `json:"user_title"`
	Badges    []interface{} `json:"badges"`
	Online    bool          `json:"online"`
	Username  string        `json:"username"`
	IsNft     int           `json:"is_nft"`
}

type GroupInfo struct {
	Id        int    `json:"id"`
	Name      string `json:"name"`
	Title     string `json:"title"`
	DomainUrl string `json:"domain_url"`
}

type PostData struct {
	Id          int           `json:"id"`
	Content     string        `json:"content"`
	UserId      int           `json:"user_id"`
	EditorType  int           `json:"editor_type"`
	Html        string        `json:"html"`
	TotalLikes  int           `json:"total_likes"`
	TotalReport int           `json:"total_report"`
	Attachments []interface{} `json:"attachments"`
}

type Thread struct {
	Id              int           `json:"id"`
	Title           string        `json:"title"`
	UserId          int           `json:"user_id"`
	GroupId         int           `json:"group_id"`
	FirstPostId     int           `json:"first_post_id"`
	UpdatedAt       time.Time     `json:"updated_at"`
	LikesCount      int           `json:"likes_count"`
	PostsCount      int           `json:"posts_count"`
	IsDelete        int           `json:"is_delete"`
	CategoryIndexId int           `json:"category_index_id"`
	CategoryName    string        `json:"category_name"`
	CategoryId      int           `json:"category_id"`
	GalleryId       int           `json:"gallery_id"`
	Slug            any           `json:"slug"`
	PollStatus      interface{}   `json:"poll_status"`
	IsPin           int           `json:"is_pin"`
	Tags            []*Tags       `json:"tags"`
	User            *UserData     `json:"user"`
	Group           *GroupInfo    `json:"group"`
	FirstPost       *PostData     `json:"first_post"`
	Tips            []interface{} `json:"tips"`
}

type ProposalsResponse struct {
	ApiResponseWrapper
	Data struct {
		Threads []*Thread `json:"threads"`
	}
}

type CategoriesListResponse struct {
	ApiResponseWrapper
	Data struct {
		Categories []*Category `json:"categories"`
	}
}

type TagsListResponse struct {
	ApiResponseWrapper
	Data struct {
		Tags []*Tags `json:"tags"`
	}
}

// PaginationParams saves pagination and some other query params for API call
type PaginationParams struct {
	Page            int
	PerPage         int
	Filter          string // available values: all, proposals, subscribed
	CategoryIndexId int
	TagId           int
	Sort            string // available values: new, old
	GroupName       string
}

type Category struct {
	Id          int         `json:"id"`
	GroupId     int         `json:"group_id"`
	Name        string      `json:"name"`
	CategoryId  int         `json:"category_id"`
	ParentId    int         `json:"parent_id"`
	Type        int         `json:"type"`
	Order       interface{} `json:"order"`
	ThreadCount int         `json:"thread_count"`
	TemplateId  int         `json:"template_id"`
	PostCount   int         `json:"post_count"`
	IconUnicode string      `json:"icon_unicode"`
	CanSee      int         `json:"can_see"`
	CanCreate   int         `json:"can_create"`
	Children    []*Category `json:"children"`
}

type Tag struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	ThreadCount int    `json:"thread_count"`
	Id          int    `json:"id"`
	Order       int    `json:"order"`
	Type        int    `json:"type"`
}

func (c *PaginationParams) ToMap() map[string]string {
	return map[string]string{
		"page":              strconv.Itoa(c.Page),
		"per_page":          strconv.Itoa(c.PerPage),
		"filter":            c.Filter,
		"category_index_id": strconv.Itoa(c.CategoryIndexId),
		"tag_id":            strconv.Itoa(c.TagId),
		"sort":              c.Sort,
		"group_name":        c.GroupName,
	}
}
