package metaforo

import (
	"strconv"
	"time"
)

////////////////////////////
// Request object: Some predefined struct
////////////////////////////

type TokenType int
type ChainType int

////////////////////////////
// Response objects defines struct for Metaforo API responses
////////////////////////////

type ApiResponseWrapper[T any] struct {
	Status      bool   `json:"status"`
	Code        int    `json:"code"`
	Description string `json:"description"`
	Server      string `json:"server"`
	Data        *T     `json:"data"`
}

type User struct {
	// TODO delete unused fields
	Id             int         `json:"id"`
	Email          interface{} `json:"email"`
	PhotoUrl       string      `json:"photo_url"`
	Likes          int         `json:"likes"`
	Posts          int         `json:"posts"`
	Activate       int         `json:"activate"`
	LastPostTime   time.Time   `json:"last_post_time"`
	Web3PublicKey  string      `json:"web3_public_key"`
	Web3PublicKeys []struct {
		Type    int    `json:"type"`
		Address string `json:"address"`
	} `json:"web3_public_keys"`
	GroupProfiles []struct {
		GroupId       int         `json:"group_id"`
		GroupName     string      `json:"group_name"`
		DisplayName   interface{} `json:"display_name"`
		DisplayAvatar string      `json:"display_avatar"`
	} `json:"group_profiles"`
	Username string `json:"username"`
	IsNft    int    `json:"is_nft"`
}

type UserActivity struct {
	// TODO delete unused fields
	Id                      int         `json:"id"`
	ThreadId                int         `json:"thread_id"`
	UserId                  int         `json:"user_id"`
	Sign                    interface{} `json:"sign"`
	SignData                interface{} `json:"sign_data"`
	CreatedAt               time.Time   `json:"created_at"`
	SignMsg                 interface{} `json:"sign_msg"`
	Content                 string      `json:"content"`
	Ipfs                    interface{} `json:"ipfs"`
	Arweave                 string      `json:"arweave"`
	UpdatedAt               string      `json:"updated_at"`
	GroupPostId             interface{} `json:"group_post_id"`
	GroupThreadId           interface{} `json:"group_thread_id"`
	ParentId                int         `json:"parent_id"`
	Depth                   int         `json:"depth"`
	GroupId                 int         `json:"group_id"`
	Deleted                 int         `json:"deleted"`
	DeletedBy               int         `json:"deleted_by"`
	DeletedAt               interface{} `json:"deleted_at"`
	Nsfw                    int         `json:"nsfw"`
	NsfwScore               int         `json:"nsfw_score"`
	Cooked                  interface{} `json:"cooked"`
	ReplyUid                int         `json:"reply_uid"`
	ReplyPid                int         `json:"reply_pid"`
	ReplyCount              int         `json:"reply_count"`
	ReplyCountWithSoftDel   int         `json:"reply_count_with_soft_del"`
	Html                    string      `json:"html"`
	EditorType              int         `json:"editor_type"`
	ImportSourceImportId    interface{} `json:"_import_source_import_id"`
	ImportSourceUserId      interface{} `json:"_import_source_user_id"`
	ImportSourceThreadId    interface{} `json:"_import_source_thread_id"`
	ImportSourcePostId      interface{} `json:"_import_source_post_id"`
	ImportSourceDeletedById interface{} `json:"_import_source_deleted_by_id"`
	ImportSourcePostNumber  interface{} `json:"_import_source_post_number"`
	Username                string      `json:"username"`
	UserAvatar              string      `json:"user_avatar"`
	GroupName               string      `json:"group_name"`
	GroupTitle              string      `json:"group_title"`
	FirstPostId             int         `json:"first_post_id"`
	ThreadTitle             string      `json:"thread_title"`
	ThreadPosterId          int         `json:"thread_poster_id"`
	ParentPosterName        interface{} `json:"parent_poster_name"`
	ThreadPosterName        string      `json:"thread_poster_name"`
	DomainUrl               interface{} `json:"domain_url"`
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
	UserId      int           `json:"user_id"`
	GroupId     int           `json:"group_id"`
	ParentId    int           `json:"parent_id"`
	Content     string        `json:"content"`
	Depth       int           `json:"depth"`
	ThreadId    int           `json:"thread_id"`
	ReplyUid    int           `json:"reply_uid"`
	ReplyPid    int           `json:"reply_pid"`
	Sign        interface{}   `json:"sign"`
	SignMsg     interface{}   `json:"sign_msg"`
	EditorType  int           `json:"editor_type"`
	Html        string        `json:"html"`
	UpdatedAt   time.Time     `json:"updated_at"`
	CreatedAt   time.Time     `json:"created_at"`
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
	LastPostId      int           `json:"last_post_id"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
	LikesCount      int           `json:"likes_count"`
	PostsCount      int           `json:"posts_count"`
	CategoryIndexId int           `json:"category_index_id"`
	CategoryId      int           `json:"category_id"`
	CategoryName    string        `json:"category_name"`
	IsDelete        int           `json:"is_delete"`
	GalleryId       int           `json:"gallery_id"`
	LotteryId       int           `json:"lottery_id"`
	CanReply        bool          `json:"can_reply"`
	Slug            string        `json:"slug"`
	Badge           []interface{} `json:"badge"`
	UserTitle       []interface{} `json:"user_title"`
	Posts           []interface{} `json:"posts"`
	//PostsMap        []interface{} `json:"posts_map"` // map[string]int
	FirstLevelCount int           `json:"first_level_count"`
	IsSubscribe     bool          `json:"is_subscribe"`
	IsPin           int           `json:"is_pin"`
	Pinned          []interface{} `json:"pinned"`
	Hots            []struct {
		Id       int    `json:"id"`
		Title    string `json:"title"`
		UserId   int    `json:"user_id"`
		Username string `json:"username"`
		IsNft    int    `json:"is_nft"`
		PhotoUrl string `json:"photo_url"`
		Content  string `json:"content"`
	} `json:"hots"`
	EditHistory struct {
		Count int                      `json:"count"`
		Lists []*PostEditHistoryRecord `json:"lists"`
	} `json:"edit_history"`
	Polls          []interface{} `json:"polls"`
	PollStatus     string        `json:"poll_status"` // null, open or expired
	Tags           []interface{} `json:"tags"`
	SnapshotId     string        `json:"snapshot_id"`
	SnapshotAuthor string        `json:"snapshot_author"`
	IsGallery      bool          `json:"is_gallery"`
	Gallery        interface{}   `json:"gallery"`
	Participant    int           `json:"participant"`
	Tips           []interface{} `json:"tips"`
	TipCount       int           `json:"tip_count"`
	TipList        []interface{} `json:"tip_list"`
	Tipped         bool          `json:"tipped"`
	User           *UserData     `json:"user"`
	Group          *GroupInfo    `json:"group"`
	FirstPost      *PostData     `json:"first_post"`
	UpdateCount    struct {
		ParentId    int `json:"parent_id"`
		ThreadCount int `json:"thread_count"`
		CategoryId  int `json:"category_id"`
	} `json:"update_count"`
}

type PostEditHistoryRecord struct {
	Username  string    `json:"username"`
	Id        int       `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	PostId    int       `json:"post_id"`
	UserId    int       `json:"user_id"`
	Arweave   string    `json:"arweave"`
	PostType  int       `json:"post_type"`
}

type LoginResponse struct {
	User     *User  `json:"user"`
	ApiToken string `json:"api_token"`
}

type UserActivitiesResponse struct {
	UserActivities []*UserActivity `json:"user_activities"`
}

type ProposalListResponse struct {
	Threads []*Thread `json:"threads"`
}

type ProposalResponse struct {
	Thread      *Thread       `json:"thread"`
	Post        *PostData     `json:"post"`
	Attachments []interface{} `json:"attachments"`
}

type CategoriesListResponse struct {
	Categories []*Category `json:"categories"`
}

type TagsListResponse struct {
	Tags []*Tag `json:"tags"`
}

type GroupInfoResponse struct {
	Group *GroupInfo `json:"group"`
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
	IconUnicode interface{} `json:"icon_unicode"`
	CanSee      int         `json:"can_see"`
	CanCreate   int         `json:"can_create"`
	Children    []*Category `json:"children"`
}

type TokenGateSetting struct {
	Id      int `json:"id"`
	GroupId int `json:"group_id"`

	// Token spec for this gate record.
	// Regards TokenType field, here are the available values
	//   0 - ERC20
	//   1 - ERC721
	//   2 - ERC1155, and tokenId is
	ChainType ChainType `json:"chain_type"`
	TokenType TokenType `json:"token_type"`
	Address   string    `json:"address"`
	TokenId   int       `json:"token_id"`
	Alias     string    `json:"alias"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	DeletedAt string `json:"deleted_at"` // Seems this field is useless in the response
}

type GroupInfo struct {
	Id                 int                `json:"id"`
	Name               string             `json:"name"`
	Title              string             `json:"title"`
	CreatedAt          time.Time          `json:"created_at"`
	UpdatedAt          time.Time          `json:"updated_at"`
	DomainUrl          string             `json:"domain_url"`
	Owner              int                `json:"owner"`
	Cover              string             `json:"cover"`
	Logo               string             `json:"logo"`
	Description        string             `json:"description"`
	NoRecommend        int                `json:"no_recommend"`
	SuperNoRecommend   int                `json:"super_no_recommend"`
	Joining            int                `json:"joining"`
	Visibility         int                `json:"visibility"`
	ShowSnapshot       int                `json:"show_snapshot"`
	PollTemplate       []interface{}      `json:"poll_template"`
	SnapshotSpace      string             `json:"snapshot_space"`
	DaoName            string             `json:"daoname"`
	ShowConnectDiscord int                `json:"show_connect_discord"`
	PollSetting        []TokenGateSetting `json:"poll_setting"`
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
	Gallery        interface{}   `json:"gallery"`
	CategoryAdmin  []interface{} `json:"category_admin"`
	CategoryAdmins []interface{} `json:"category_admins"`
	PrivacyType    int           `json:"privacy_type"`
	FtCount        int           `json:"ft_count"`
	ChainType      ChainType     `json:"chain_type"`
	OpenReadonly   int           `json:"open_readonly"`
	PrimaryNft     string        `json:"primary_nft"`
	PrimaryToken   string        `json:"primary_token"`
	GroupSettings  struct {
		KudosVote  string `json:"kudos_vote"`
		NeuronVote string `json:"neuron_vote"`
		ReplyLevel int    `json:"reply_level"`
	} `json:"group_settings"`
	Tags                        []*Tag        `json:"tags"`
	OrderTags                   []*Tag        `json:"order_tags"`
	PollCategory                []interface{} `json:"poll_category"`
	MainTokenAddress            string        `json:"main_token_address"`
	TipTokenInfo                interface{}   `json:"tip_token_info"`
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
	GroupExtraInfo              struct {
		Id       int         `json:"id"`
		GroupId  int         `json:"group_id"`
		Twitter  interface{} `json:"twitter"`
		Discord  interface{} `json:"discord"`
		Telegram interface{} `json:"telegram"`
		Website  interface{} `json:"website"`
	} `json:"group_extra_info"`
}

////////////////////////////
// Request objects defines struct for sending Metaforo API requests
////////////////////////////

// [
//  {
//    "id": 0,
//    "name": "Everyone",
//    "can_see": 1,
//    "can_reply": 1,
//    "can_create": 1
//  }
//]

type NewCategoryPermissionRequest struct {
	Id        int    `json:"id"`
	Name      string `json:"name"`
	CanSee    int    `json:"can_see"`
	CanReply  int    `json:"can_reply"`
	CanCreate int    `json:"can_create"`
}

// [
//   {
//      "insert":"测试"
//   }
//]

// NewContentRequest defines content request data structure
type NewContentRequest struct {
	Insert string `json:"insert"`
}

// [
//   {
//      "name":"待审核",
//      "id":471
//   },
//   {
//      "name":"投票中",
//      "id":472
//   }
//]

// NewProposalTagRequest defines tag request data structure used for creating proposals
type NewProposalTagRequest struct {
	Name string `json:"name"`
	Id   int    `json:"id"`
}

// [
//
//	{
//	   "options":[
//	      {
//	         "text":"选项1",
//	         "type":1
//	      },
//	      {
//	         "text":"选项2",
//	         "type":0
//	      }
//	   ],
//	   "type":"1",
//	   "title":"every one can vote",
//	   "showType":"1",
//	   "showResult":true,
//	   "chartType":"1",
//	   "voteType":"1",
//	   "chain_type":0,
//	   "contract_type":0,
//	   "setting_id":0,
//	   "period":"1",
//	   "close_at":"2023-12-28 23:59:59",
//	   "poll_start_at":"2023-12-20 00:00:00",
//	   "max":1,
//	   "min_tokens":"0",
//	   "token_address":"",
//	   "token_icon":"",
//	   "token_image":{
//	   },
//	   "PollCategory":"0",
//	   "LastCategroyChange":"0",
//	   "poll_category":[
//	   ],
//	   "token_id":0,
//	   "strategy":[
//	   ],
//	   "quorum":false,
//	   "weight":true,
//	   "percent":"",
//	   "min_number":"",
//	   "step":2
//	}
//
// ]
//
// [
//
//	{
//	   "options":[
//	      {
//	         "text":"选项1",
//	         "type":1
//	      },
//	      {
//	         "text":"选项2",
//	         "type":0
//	      }
//	   ],
//	   "type":"1",
//	   "title":"ERC20 vote",
//	   "showType":"1",
//	   "showResult":true,
//	   "chartType":"1",
//	   "voteType":"2",
//	   "chain_type":1,
//	   "setting_id":63,
//	   "period":"1",
//	   "close_at":"2023-12-29 23:59:59",
//	   "poll_start_at":"2023-12-20 00:00:00",
//	   "max":1,
//	   "min_tokens":"10",
//	   "token_address":"0xdac17f958d2ee523a2206206994597c13d831ec7",
//	   "token_icon":"",
//	   "token_image":{
//	   },
//	   "PollCategory":"0",
//	   "LastCategroyChange":"0",
//	   "poll_category":[
//	   ],
//	   "token_id":0,
//	   "strategy":[
//	   ],
//	   "quorum":false,
//	   "weight":true,
//	   "percent":"",
//	   "min_number":"",
//	   "step":2
//	}
//
// ]
//
// [
//
//	{
//	   "options":[
//	      {
//	         "text":"选项1",
//	         "type":1
//	      },
//	      {
//	         "text":"选项2",
//	         "type":0
//	      }
//	   ],
//	   "type":"1",
//	   "title":"ERC721 vote",
//	   "showType":"1",
//	   "showResult":true,
//	   "chartType":"1",
//	   "voteType":"3",
//	   "chain_type":1,
//	   "contract_type":1,
//	   "setting_id":64,
//	   "period":"1",
//	   "close_at":"2023-12-29 23:59:59",
//	   "poll_start_at":"2023-12-20 00:00:00",
//	   "max":1,
//	   "min_tokens":"0",
//	   "token_address":"0xfdf5acd92840e796955736b1bb9cc832740744ba",
//	   "token_icon":"",
//	   "token_image":{
//	   },
//	   "PollCategory":"0",
//	   "LastCategroyChange":"0",
//	   "poll_category":[
//	   ],
//	   "token_id":0,
//	   "strategy":[
//	   ],
//	   "quorum":false,
//	   "weight":true,
//	   "percent":"",
//	   "min_number":"",
//	   "step":2
//	}
//
// ]
//
// [
//
//	{
//	   "options":[
//	      {
//	         "text":"选项1",
//	         "type":1
//	      },
//	      {
//	         "text":"选项2",
//	         "type":0
//	      }
//	   ],
//	   "type":"1",
//	   "title":"ERC1155 vote",
//	   "showType":"1",
//	   "showResult":true,
//	   "chartType":"1",
//	   "voteType":"3",
//	   "chain_type":1,
//	   "contract_type":2,
//	   "setting_id":65,
//	   "period":"1",
//	   "close_at":"2023-12-28 23:59:59",
//	   "poll_start_at":"2023-12-20 00:00:00",
//	   "max":1,
//	   "min_tokens":"0",
//	   "token_address":"0x6811f2f20c42f42656a3c8623ad5e9461b83f719",
//	   "token_icon":"",
//	   "token_image":{
//	   },
//	   "PollCategory":"0",
//	   "LastCategroyChange":"0",
//	   "poll_category":[
//	   ],
//	   "token_id":100200402,
//	   "strategy":[
//	   ],
//	   "quorum":false,
//	   "weight":true,
//	   "percent":"",
//	   "min_number":"",
//	   "step":2
//	}
//
// ]

// NewVoteFormRequest defines poll request data structure used for creating vote while creating proposals
type NewVoteFormRequest struct {
	// Options stands for vote options, the text is option value
	// TODO: What's the meaning of type for each option?
	Options []struct {
		Text string `json:"text"`
		Type int    `json:"type"`
	} `json:"options"`

	Type string `json:"type"`

	// Vote title, optional
	Title string `json:"title"`

	// Show result type
	//   1:
	//   2:
	//   3:
	ShowType string `json:"showType"`

	// Whether the result is shown to user
	ShowResult bool `json:"showResult"`

	// TODO: What's the meaning of this?
	ChartType string `json:"chartType"`

	// Who can vote on this poll
	// 1: Everyone
	// 2: Require specified token and amount
	// 3: Require specified NFT
	VoteType string `json:"voteType"`

	// TODO: What's the meaning of those fields?
	ChainType    ChainType `json:"chain_type"`
	ContractType int       `json:"contract_type"`
	SettingId    int       `json:"setting_id"`
	Period       string    `json:"period"`

	// Poll start and end UTC datetime string, YYYY-mm-DD HH:MM:SS format
	CloseAt     string `json:"close_at"`
	PollStartAt string `json:"poll_start_at"`

	// Max polls an user can vote
	Max int `json:"max"`

	// Vote gate, the MinTokens only works when VoteType is 2
	MinTokens    string `json:"min_tokens"`
	TokenAddress string `json:"token_address"`

	// TODO: What's the meaning of those fields?
	TokenIcon  string `json:"token_icon"`
	TokenImage struct {
	} `json:"token_image"`

	PollCategory       string        `json:"PollCategory"`
	LastCategroyChange string        `json:"LastCategroyChange"`
	PollCategory1      []interface{} `json:"poll_category"`
	TokenId            int           `json:"token_id"`
	Strategy           []interface{} `json:"strategy"`
	Quorum             bool          `json:"quorum"`
	Weight             bool          `json:"weight"`
	Percent            string        `json:"percent"`
	MinNumber          string        `json:"min_number"`
	Step               int           `json:"step"`
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
