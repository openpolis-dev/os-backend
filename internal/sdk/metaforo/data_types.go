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

type Tag struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	ThreadCount int    `json:"thread_count"`
	Id          int    `json:"id"`
	Order       int    `json:"order"`
	Type        int    `json:"type"`
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
	Tags            []*Tag        `json:"tags"`
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
		Tags []*Tag `json:"tags"`
	}
}

type GroupInfoResponse struct {
	ApiResponseWrapper
	Data struct {
		Group *GroupInfo `json:"group"`
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

type GroupInfo struct {
	Id                 int           `json:"id"`
	Name               string        `json:"name"`
	Title              string        `json:"title"`
	CreatedAt          time.Time     `json:"created_at"`
	UpdatedAt          time.Time     `json:"updated_at"`
	DomainUrl          string        `json:"domain_url"`
	Owner              int           `json:"owner"`
	Cover              string        `json:"cover"`
	Logo               string        `json:"logo"`
	Description        string        `json:"description"`
	NoRecommend        int           `json:"no_recommend"`
	SuperNoRecommend   int           `json:"super_no_recommend"`
	Joining            int           `json:"joining"`
	Visibility         int           `json:"visibility"`
	ShowSnapshot       int           `json:"show_snapshot"`
	PollTemplate       []interface{} `json:"poll_template"`
	SnapshotSpace      string        `json:"snapshot_space"`
	DaoName            string        `json:"daoname"`
	ShowConnectDiscord int           `json:"show_connect_discord"`
	PollSetting        []interface{} `json:"poll_setting"`
	Feature            []struct {
		Id          int    `json:"id"`
		FeatureName string `json:"feature_name"`
		IsSetting   int    `json:"is_setting"`
		Status      *int   `json:"status"`
	} `json:"feature"`
	AttachedFiles struct {
		AllowEveryone    int `json:"allow_everyone"`
		AllowPost        int `json:"allow_post"`
		AllowAllFileType int `json:"allow_all_file_type"`
	} `json:"attached_files"`
	GroupAdmin []struct {
		Id      int `json:"id"`
		GroupId int `json:"group_id"`
		Level   int `json:"level"`
		UserId  int `json:"user_id"`
	} `json:"group_admin"`
	OnlineMembers     int           `json:"online_members"`
	PendingUser       []interface{} `json:"pending_user"`
	GroupSubscription struct {
		CurrentPlan string `json:"current_plan"`
		GroupId     int    `json:"group_id"`
		IsCanceled  bool   `json:"is_canceled"`
		PeriodEnd   string `json:"period_end"`
	} `json:"group_subscription"`
	Members        int           `json:"members"`
	Gallery        string        `json:"gallery"`
	CategoryAdmin  []interface{} `json:"category_admin"`
	CategoryAdmins []interface{} `json:"category_admins"`
	PrivacyType    int           `json:"privacy_type"`
	FtCount        int           `json:"ft_count"`
	ChainType      int           `json:"chain_type"`
	OpenReadonly   int           `json:"open_readonly"`
	PrimaryNft     string        `json:"primary_nft"`
	PrimaryToken   string        `json:"primary_token"`
	GroupSettings  struct {
		KudosVote  string `json:"kudos_vote"`
		NeuronVote string `json:"neuron_vote"`
		ReplyLevel string `json:"reply_level"`
	} `json:"group_settings"`
	Tags                        []*Tag        `json:"tags"`
	OrderTags                   []*Tag        `json:"order_tags"`
	PollCategory                []interface{} `json:"poll_category"`
	MainTokenAddress            string        `json:"main_token_address"`
	TipTokenInfo                []interface{} `json:"tip_token_info"`
	PostsTotal                  int           `json:"posts_total"`
	ThreadsTotal                int           `json:"threads_total"`
	ThreadTemplate              []interface{} `json:"thread_template"`
	PollExtensions              []interface{} `json:"poll_extensions"`
	TitleList                   []interface{} `json:"title_list"`
	Categories                  []*Category   `json:"categories"`
	CurrentUserStatus           string        `json:"current_user_status"`
	AuthVerificationResult      bool          `json:"auth_verification_result"`
	AuthVerificationDescription string        `json:"auth_verification_description"`
	ReplyLevel                  int           `json:"reply_level"`
	GroupExtraInfo              string        `json:"group_extra_info"`
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
