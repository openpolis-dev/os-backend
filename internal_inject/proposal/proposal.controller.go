package proposal_inject

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/facebookgo/inject"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/global_object"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/middleware"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ProposalController struct {
	// inject
	Gin *gin.Engine `inject:""`

	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`

	ProposalService *ProposalService `inject:""`
}

func Register(fatherGroup *gin.RouterGroup) {
	g := global_object.GetGlobalObject()

	var proposal ProposalController

	err := inject.Populate(&proposal, g.Gin, g.Db, g.Cfg)

	if err != nil {
		panic(err)
	}

	var proposalGroup *gin.RouterGroup
	var proposalAuthGroup *gin.RouterGroup

	var proposalComponentsGroup *gin.RouterGroup
	// var proposalComponentsAuthGroup *gin.RouterGroup

	var proposalVoteGatesGroup *gin.RouterGroup
	// var proposalVoteGatesAuthGroup *gin.RouterGroup

	var proposalCategoriesGroup *gin.RouterGroup
	var proposalCategoriesAuthGroup *gin.RouterGroup

	var proposalTmplGroup *gin.RouterGroup
	var proposalTmplAuthGroup *gin.RouterGroup

	if fatherGroup != nil {
		proposalGroup = fatherGroup.Group("/proposals", middleware.AuthOption)
		proposalComponentsGroup = fatherGroup.Group("/proposal_components")
		proposalVoteGatesGroup = fatherGroup.Group("/proposal_vote_gates")
		proposalCategoriesGroup = fatherGroup.Group("/proposal_categories")
		proposalTmplGroup = fatherGroup.Group("/proposal_tmpl")

		proposalAuthGroup = fatherGroup.Group("/", middleware.AuthRequired).Group("/proposals")
		// proposalComponentsAuthGroup = fatherGroup.Group("/", middleware.AuthRequired).Group("/proposal_components")
		// proposalVoteGatesAuthGroup = fatherGroup.Group("/", middleware.AuthRequired).Group("/proposal_vote_gates")
		proposalCategoriesAuthGroup = fatherGroup.Group("/", middleware.AuthRequired).Group("/proposal_categories")
		proposalTmplAuthGroup = fatherGroup.Group("/", middleware.AuthRequired).Group("/proposal_tmpl")
	} else {
		proposalGroup = proposal.Gin.Group("/proposals", middleware.AuthOption)
		proposalComponentsGroup = proposal.Gin.Group("/proposal_components")
		proposalVoteGatesGroup = proposal.Gin.Group("/proposal_vote_gates")
		proposalCategoriesGroup = proposal.Gin.Group("/proposal_categories")
		proposalTmplGroup = proposal.Gin.Group("/proposal_tmpl")

		proposalAuthGroup = proposal.Gin.Group("/", middleware.AuthRequired).Group("/proposals")
		// proposalComponentsAuthGroup = proposal.Gin.Group("/", middleware.AuthRequired).Group("/proposal_components")
		// proposalVoteGatesAuthGroup = proposal.Gin.Group("/", middleware.AuthRequired).Group("/proposal_vote_gates")
		proposalCategoriesAuthGroup = proposal.Gin.Group("/", middleware.AuthRequired).Group("/proposal_categories")
		proposalTmplAuthGroup = proposal.Gin.Group("/", middleware.AuthRequired).Group("/proposal_tmpl")
	}

	// no auth
	proposalComponentsGroup.GET("/", proposal.ListComponents)
	proposalComponentsGroup.GET("/:id", proposal.GetComponent)

	proposalVoteGatesGroup.GET("/", proposal.ListVoteGates)

	proposalGroup.GET("/list", proposal.List)
	proposalGroup.GET("/show/:id", proposal.Detail)
	proposalGroup.GET("/vote_detail/:vote_option_id", proposal.ShowVoteDetail)

	proposalCategoriesGroup.GET("/list", proposal.ListAllCategories)
	proposalTmplGroup.GET("/list", proposal.ListTemplates)

	// auth
	proposalAuthGroup.POST("/create", proposal.Create)
	proposalAuthGroup.POST("/update/:id", proposal.Update)
	proposalAuthGroup.POST("/add_comment/:id", proposal.AddComment)
	proposalAuthGroup.POST("/edit_comment/:id", proposal.EditComment)
	proposalAuthGroup.POST("/delete_comment/:id", proposal.DeleteComment)
	proposalAuthGroup.GET("/my", proposal.MyList)
	proposalAuthGroup.GET("/creating_project_proposals", proposal.GetProposalsUsedForCreatingProjects)
	// State change actions for proposals
	proposalAuthGroup.POST("/withdraw/:id", proposal.Withdraw)
	proposalAuthGroup.POST("/approve/:id", proposal.Approve)
	proposalAuthGroup.POST("/reject/:id", proposal.Reject)
	proposalAuthGroup.POST("/can_vote/:id", proposal.CheckVotePermission)
	proposalAuthGroup.POST("/vote/:id", proposal.CastVote)
	proposalAuthGroup.POST("/revoke_vote/:id", proposal.RevokeVote)
	proposalAuthGroup.POST("/close_vote/:id", proposal.CloseVote)

	proposalTmplAuthGroup.GET("/list_with_perm", proposal.ListTemplatesWithPerm)

	proposalCategoriesAuthGroup.GET("/list_with_perm", proposal.ListCategoriesWithPerm)
}

func (c *ProposalController) ListComponents(ctx *gin.Context) {
	var records []*ComponentResponse
	err := c.Db.Model(&model.ProposalComponent{}).Find(&records).Error
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(500, api.ServerError(errors.New("list components failed detail:"+err.Error())))
		return
	}
	ctx.JSON(200, api.Success(records))
}

func (c *ProposalController) GetComponent(ctx *gin.Context) {
	idParam := ctx.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	var rcd *ComponentResponse
	err = c.Db.First(&rcd, id).Error
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(500, api.ServerError(errors.New("list components failed detail:"+err.Error())))
		return
	}
	ctx.JSON(200, api.Success(rcd))
}

func (c *ProposalController) ListVoteGates(ctx *gin.Context) {
	var dbRecords []*model.ProposalVoteGate
	if err := c.Db.Model(&model.ProposalVoteGate{}).Find(&dbRecords).Error; err != nil {
		log.Error().Msgf("fetch poll gates error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list poll gates error detail:"+err.Error())))
		return
	}

	responseRecords := lo.Map(dbRecords, func(r *model.ProposalVoteGate, _ int) *FrontendVoteGateResponse {
		return &FrontendVoteGateResponse{
			ID:        r.ID,
			Name:      r.Name,
			TokenAddr: r.TokenAddress,
			TokenId:   r.TokenId,
			TokenType: r.TokenTypeName(),
			ChainType: r.ChainName(),
		}
	})

	ctx.JSON(http.StatusOK, api.Success(responseRecords))
}

func (c *ProposalController) List(ctx *gin.Context) {
	user, _ := api.ForContextUserAndDB(ctx)
	queryParams := ListProposalQueryParams{}
	if err := ctx.Bind(&queryParams); err != nil {
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("query params error: %+v", err),
		})
		return
	}
	// Parse pagination
	page := api.ParseAndConvertPageParam(ctx)

	querySql := ListProposalsSQL

	// Build query
	if queryParams.State != "" {
		stateList := strings.Split(queryParams.State, ",")
		var stateVals []string
		for _, stateName := range stateList {
			if stateVal, found := model.ProposalStateIdNameMapping[stateName]; found {
				stateVals = append(stateVals, fmt.Sprintf("%d", stateVal))
			} else {
				sdk.LogUserSideError(ctx, fmt.Errorf("query proposal state %s error", queryParams.State))
				log.Warn().Msgf("query proposal state %s error", queryParams.State)
			}
		}

		querySql += fmt.Sprintf(" AND state IN (%s)", strings.Join(stateVals, ","))
	} else {
		querySql += fmt.Sprintf(" AND state != %d", model.ProposalStatePendingSubmit)
	}

	if queryParams.CategoryId != 0 {
		querySql += fmt.Sprintf(" AND proposal_category_id = %d", queryParams.CategoryId)
	}

	if queryParams.CategoryId != 0 {
		querySql += fmt.Sprintf(" AND proposal_category_id = %d", queryParams.CategoryId)
	}

	if queryParams.Q != "" {
		querySql += fmt.Sprintf(" AND title ilike '%%%s%%'", queryParams.Q)
	}

	listBySip := false
	if queryParams.Sip != "" {
		querySql += fmt.Sprintf(" AND sip != 0")
		listBySip = true
	}

	total, resultRows, err := c.ProposalService.GenerateFrontendProposalRecords(c.Db, querySql, page, listBySip)
	if err != nil {
		log.Error().Msgf("get proposal list error: query sql: %s, err: %+v", querySql, err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get proposal error detail:"+err.Error())))
		return
	}

	// get result rows is vote by me
	// <<
	log.Debug().Msgf("vote query sql check: user: %+v", user)
	if len(resultRows) > 0 && user != nil {
		voteQuerySql := fmt.Sprintf("select * from proposal_user_vote_records where user_wallet = '%s' ", user.Wallet)

		var proposalIdList []string
		for i := 0; i < len(resultRows); i++ {
			proposalIdList = append(proposalIdList, fmt.Sprintf("%d", resultRows[i].ID))
		}

		voteQuerySql += fmt.Sprintf(" AND proposal_id in(%s)", strings.Join(proposalIdList, ","))

		log.Debug().Msgf("vote query sql check: query sql: %s", voteQuerySql)

		var userProposalUserVoteRecord []*model.ProposalUserVoteRecord

		dbErr := c.Db.Raw(voteQuerySql).Find(&userProposalUserVoteRecord).Error
		if dbErr != nil {
			log.Error().Msgf("get proposal list error: query sql: %s, err: %+v", voteQuerySql, dbErr)
		} else {
			resultRows = lo.Map(resultRows, func(r *FrontendProposalListRecord, _ int) *FrontendProposalListRecord {
				for i := 0; i < len(userProposalUserVoteRecord); i++ {
					log.Debug().Msgf("vote query sql check find vote data: ProposalID: %d, ID: %d", userProposalUserVoteRecord[i].ProposalID, r.ID)
					if userProposalUserVoteRecord[i].ProposalID == r.ID {
						r.IsVoted = true
						break
					}
				}

				return r
			})
		}
	}
	// >>

	ctx.JSON(http.StatusOK, api.Success(api.ListReplyData{
		Page:  queryParams.Page,
		Size:  queryParams.Size,
		Total: total,
		Rows:  resultRows,
	}))
}

func (c *ProposalController) Detail(ctx *gin.Context) {
	startPostIdStr := ctx.Query("start_post_id")
	metaforoAccessToken := ctx.Query("access_token")
	startPostId := 0
	var err error
	if startPostIdStr != "" {
		startPostId, err = strconv.Atoi(startPostIdStr)
		if err != nil {
			log.Warn().Msgf("parse ")
		}
	}

	proposalIdStr := ctx.Param("id")

	// db, cfg := api.ForContextDBAndConfig(ctx)

	proposalRecord, err := c.ProposalService.GetProposalFromStringId(c.Db, proposalIdStr)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Warn().Msgf("proposal %s not found", proposalIdStr)
			ctx.JSON(http.StatusNotFound, api.BadRequest(err))
			return
		} else {
			sdk.LogUserSideError(ctx, err)
			log.Error().Msgf("parse proposal id %s error: %+v", ctx.Param("id"), err)
			ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
			return
		}
	}

	responseData, err := c.ProposalService.ConvertProposalToFrontendDetailRecord(c.Db, proposalRecord.ID, startPostId, metaforoAccessToken, c.Cfg.MetaforoData.GroupName)
	if err != nil {
		log.Error().Msgf("convert proposal to frontend format error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("load proposal data error detail:"+err.Error())))
		return
	}

	// Return to frontend
	ctx.JSON(http.StatusOK, api.Success(responseData))
}

func (c *ProposalController) ShowVoteDetail(ctx *gin.Context) {
	voteOptionIdStr := ctx.Param("vote_option_id")
	voteOptionId, err := strconv.Atoi(voteOptionIdStr)
	if err != nil {
		err := fmt.Errorf("parse request data error: %+v", err)
		log.Error().Msgf(err.Error())
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("parse request data error: %+v", err)))
		return
	}

	page := 1
	pageStr := ctx.Query("page")
	if pageStr != "" {
		page, err = strconv.Atoi(pageStr)
		if err != nil {
			err := fmt.Errorf("parse request data error: %+v", err)
			log.Error().Msgf(err.Error())
			sdk.LogUserSideError(ctx, err)
			ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("parse request data error: %+v", err)))
			return
		}
	}

	voterList, err := metaforo.GetVoterList(c.Cfg.MetaforoData.GroupName, voteOptionId, page)
	if err != nil {
		log.Error().Msgf("get vote list error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("get vote list error detail:"+err.Error())))
		return
	}

	// TODO: Update this logic to fetchUserWalletFromMetaforoIds function
	// Update metaforo user record if UID not found in DB
	missingMfUserIds := map[int]*metaforo.UserDetailResponseForProfileAPI{}
	for _, mfVoterRecord := range voterList {
		var rcdCount int64
		if err = c.Db.Model(&model.MetaforoUser{}).Where(&model.MetaforoUser{MetaforoUserId: mfVoterRecord.UserId}).Count(&rcdCount).Error; err != nil {
			log.Error().Msgf("count metaforo user record error: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("get metaforo user record error detail:"+err.Error())))
			return
		}
		if rcdCount == 0 {
			mfUserData, err := metaforo.UserDetail(mfVoterRecord.UserId)
			if err != nil {
				log.Error().Msgf("get metaforo user detail error: %+v", err)
				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("get metaforo user detail error detail:"+err.Error())))
				return
			}
			missingMfUserIds[mfVoterRecord.UserId] = mfUserData
		}
	}

	// Create MetaforoUser and User record from API dat
	err = c.Db.Transaction(func(tx *gorm.DB) error {
		for userId, profileData := range missingMfUserIds {
			metaforoUser := model.MetaforoUser{
				MetaforoUserId: userId,
				UserWallet:     common.FormatUserWallet(profileData.User.Web3PublicKey),
			}
			mfUserTx := tx.Where(&model.MetaforoUser{MetaforoUserId: userId}).Find(&metaforoUser)

			if mfUserTx.Error != nil {
				log.Error().Msgf("get metaforo user record error: %+v", mfUserTx.Error)
				sdk.LogServerErrorToSentry(ctx, mfUserTx.Error)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("get metaforo user record error detail:"+err.Error())))
				return mfUserTx.Error
			} else if mfUserTx.RowsAffected == 0 {
				if err = tx.Create(&metaforoUser).Error; err != nil {
					log.Error().Msgf("create metaforo user record error: %+v", err)
					sdk.LogServerErrorToSentry(ctx, err)
					ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("create metaforo user record error detail:"+err.Error())))
					return err
				}
			} else if mfUserTx.RowsAffected == 1 {
				if err = tx.Updates(&metaforoUser).Error; err != nil {
					log.Error().Msgf("update metaforo user record error: %+v", err)
					sdk.LogServerErrorToSentry(ctx, err)
					ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("update metaforo user record error detail:"+err.Error())))
					return err
				}
			} else {
				err = fmt.Errorf("unexpected rows affected: %d", mfUserTx.RowsAffected)
				log.Error().Msgf(err.Error())
				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("unexpected rows affected: %d", mfUserTx.RowsAffected)))
				return err
			}

			// Create user record if not existing
			userRecord := model.User{Wallet: metaforoUser.UserWallet}
			c.Db.Clauses(clause.OnConflict{DoNothing: true}).Model(&model.User{}).Where(&userRecord).Assign(model.User{
				CreateTs: model.GetCurrentUtcEpochSecond(),
				UpdateTs: model.GetCurrentUtcEpochSecond(),
				Avatar:   profileData.User.PhotoUrl,
				Name:     profileData.User.Username,
			}).FirstOrCreate(&userRecord)
		}
		return nil
	})

	// Extract weight value for each user
	userWeightMap := lo.SliceToMap(voterList, func(item *metaforo.UserPollRecord) (int, int) { return item.Uid, item.Weight })

	metaforoUserIds := lo.Map(voterList, func(item *metaforo.UserPollRecord, index int) int { return item.UserId })
	userRecords, err := c.ProposalService.GetOsUserFromMetaforoUserId(c.Db, metaforoUserIds)
	if err != nil {
		log.Error().Msgf("list user vote detail error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("list user vote detail error detail:"+err.Error())))
		return
	}

	rslt := lo.Map(userRecords, func(u *JointMetaforoAndOsUser, _ int) *userVoteDetailInfo {
		weight, found := userWeightMap[u.MetaforoUserID]
		if !found {
			log.Warn().Msgf("no weight found for user: %d", u.MetaforoUserID)
			weight = 0
		}
		return &userVoteDetailInfo{
			*u,
			weight,
		}
	})
	ctx.JSON(http.StatusOK, api.Success(rslt))

}

func (c *ProposalController) Create(ctx *gin.Context) {
	// Parsing request to create proposal object
	var reqData CreateOrUpdateProposalData
	if err := ctx.BindJSON(&reqData); err != nil {
		log.Error().Msgf("parse request data error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("parse request data error: %+v", err)))
		return
	}

	user, _, _, _ := api.ForContext(ctx)

	// Get proposal template data, if it is a close project proposal, try to get project info and check whether it is in open or closed_failed state
	pTmplType, err := c.ProposalService.GetProposalTemplateType(c.Db, reqData.TemplateId)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		log.Error().Msgf("get proposal template error: %+v", err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error detail:"+err.Error())))
		return
	}

	// In close project proposal, verify whether the project to be closed is in open or closed_failed state,
	// and only set project to closing in those status. For other cases, return error
	if pTmplType == model.ProposalTemplateTypeCloseProject {
		projectCanBeClosed, err := c.ProposalService.VerifyProjectCanBeClosed(c.Db, reqData.CreateProjectProposalId)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			log.Error().Msgf("get proposal created project error: %+v", err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error detail:"+err.Error())))
			return
		}

		if !projectCanBeClosed {
			err := fmt.Errorf("project %d can't be closed", reqData.CreateProjectProposalId)
			sdk.LogUserSideError(ctx, err)
			log.Error().Msgf(err.Error())
			ctx.JSON(http.StatusBadRequest, api.ServerError(errors.New("related project can't be closed")))
			return
		}
	}

	proposalRecord, err := c.ProposalService.SaveProposalRecordToDB(c.Db, &reqData, user.Wallet, 0, c.Cfg)
	if err != nil {
		log.Error().Msgf("create proposal error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error detail:"+err.Error())))
		return
	}

	if reqData.SubmitToMetaforo {
		if reqData.MetaforoAccessToken == "" {
			log.Error().Msgf("metaforo access token is empty")
			sdk.LogUserSideError(ctx, errors.New("metaforo access token is empty"))
			ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("metaforo access token is empty")))
			return
		}

		if err = c.ProposalService.UpdateProposalAssociatedProjectStatusInCloseProjectToClosing(c.Db, reqData); err != nil {
			log.Error().Msgf("associate proposal with project error: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error detail:"+err.Error())))
			return
		}

		if err := c.ProposalService.SaveProposalToMetaforo(c.Db, proposalRecord.ID, proposalRecord.VoteType, reqData.MetaforoAccessToken, reqData.EditorType, reqData.IsMultipleVote, c.Cfg.MetaforoData.GroupName); err != nil {
			log.Error().Msgf("create metaforo proposal error: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error detail:"+err.Error())))
			return
		}

		// If the publicity second is 0, update the db proposal to voting state or pending execution state, and handle SIP data
		if proposalRecord.PublicitySecond == 0 {
			log.Debug().Msgf("proposal has no publicity time, change to approved status directly")
			_, err = c.ProposalService.UpdateProposalStateAndLaunchStateChangeActions(c.Db, user, fmt.Sprintf("%d", proposalRecord.ID), model.ProposalStateApproved, c.Cfg)
		} else if proposalRecord.VoteType == model.ProposalVoteTypeNone {
			if err = c.ProposalService.CreateJobToUpdateNoVoteProposalToNextState(c.Db, proposalRecord.ID, proposalRecord.VoteType, proposalRecord.CreateTs+proposalRecord.PublicitySecond, model.ProposalStateApproved); err != nil {
				log.Error().Msgf("create proposal state change error: %+v", err)
				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error detail:"+err.Error())))
				return
			}
		}

		c.Db.First(&proposalRecord, proposalRecord.ID)
	}

	responseData, err := c.ProposalService.ConvertProposalToFrontendDetailRecord(c.Db, proposalRecord.ID, 0, reqData.MetaforoAccessToken, c.Cfg.MetaforoData.GroupName)
	if err != nil {
		log.Error().Msgf("convert proposal to frontend format error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("load proposal data error detail:"+err.Error())))
		return
	}

	// Return to frontend
	ctx.JSON(http.StatusOK, api.Success(responseData))
}

func (c *ProposalController) Update(ctx *gin.Context) {
	user, _, _, _ := api.ForContext(ctx)
	proposalIdStr := ctx.Param("id")
	proposalRcd, err := c.ProposalService.GetProposalFromStringId(c.Db, proposalIdStr)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Warn().Msgf("proposal %s not found", proposalIdStr)
			ctx.JSON(http.StatusNotFound, nil)
			return
		} else {
			log.Error().Msgf("get proposal id %s error: %+v", proposalIdStr, err)
			sdk.LogUserSideError(ctx, err)
			ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("get proposal error detail:"+err.Error())))
			return
		}
	}

	if !proposalRcd.CanBeUpdatedBy(user.Wallet) {
		log.Error().Msgf("proposal id %s can't be updated by user %s", proposalIdStr, user.Wallet)
		sdk.LogUserSideError(ctx, fmt.Errorf("proposal id %s can't be updated by user %s", proposalIdStr, user.Wallet))
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("proposal can't be updated by current user")))
		return
	}

	if proposalRcd.ProposalRecordId != "" {
		if proposalRcd.VoteStartTs < time.Now().Unix() {
			err = fmt.Errorf("proposal id %s has expired the publicity time, can't be updated by user %+v", proposalIdStr, user.Wallet)
			log.Error().Msg(err.Error())
			sdk.LogUserSideError(ctx, err)
			ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("proposal has expired and can't be updated")))
			return
		}
	} else {
		// No actions should be done for draft proposals
	}

	// Proposal is in updatable state

	// Parsing request to create proposal object
	var reqData CreateOrUpdateProposalData
	if err = ctx.BindJSON(&reqData); err != nil {
		log.Error().Msgf("parse request data error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("parse request data error: %+v", err)))
		return
	}

	// In close project proposal, verify whether the project to be closed is in open or closed_failed state,
	// and only set project to closing in those status. For other cases, return error
	// In update cases, the proposal should already have a template ID, which is not changeable in update action,
	// and frontend request doesn't send template ID in request, so use the proposal data
	pTmplType, err := c.ProposalService.GetProposalTemplateType(c.Db, *proposalRcd.ProposalTemplateID)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		log.Error().Msgf("get proposal template error: %+v", err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error detail:"+err.Error())))
		return
	}
	reqData.TemplateId = *proposalRcd.ProposalTemplateID

	// In close project proposal, verify whether the project to be closed is in open or closed_failed state,
	// and only set project to closing in those status. For other cases, return error
	if pTmplType == model.ProposalTemplateTypeCloseProject {
		projectCanBeClosed, err := c.ProposalService.VerifyProjectCanBeClosed(c.Db, reqData.CreateProjectProposalId)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			log.Error().Msgf("get proposal created project error: %+v", err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error detail:"+err.Error())))
			return
		}

		if !projectCanBeClosed {
			sdk.LogUserSideError(ctx, err)
			log.Error().Msgf("project in status can't be closed")
			ctx.JSON(http.StatusBadRequest, api.ServerError(errors.New("related project can't be closed")))
			return
		}
	}

	proposalRecord, err := c.ProposalService.SaveProposalRecordToDB(c.Db, &reqData, user.Wallet, proposalRcd.ID, c.Cfg)
	if err != nil {
		log.Error().Msgf("create proposal error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error detail:"+err.Error())))
		return
	}

	if reqData.SubmitToMetaforo {
		if reqData.MetaforoAccessToken == "" {
			log.Error().Msgf("metaforo access token is empty")
			sdk.LogUserSideError(ctx, errors.New("metaforo access token is empty"))
			ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("metaforo access token is empty")))
			return
		}

		if err = c.ProposalService.UpdateProposalAssociatedProjectStatusInCloseProjectToClosing(c.Db, reqData); err != nil {
			log.Error().Msgf("associate mushrooms: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error detail:"+err.Error())))
			return
		}

		if err := c.ProposalService.SaveProposalToMetaforo(c.Db, proposalRecord.ID, proposalRecord.VoteType, reqData.MetaforoAccessToken, reqData.EditorType, reqData.IsMultipleVote, c.Cfg.MetaforoData.GroupName); err != nil {
			log.Error().Msgf("create metaforo proposal error: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error detail:"+err.Error())))
			return
		}

		// If the publicity second is 0, update the db proposal to voting state
		if proposalRecord.PublicitySecond == 0 {
			log.Debug().Msgf("proposal has no publicity time, change to approved status directly")
			_, err = c.ProposalService.UpdateProposalStateAndLaunchStateChangeActions(c.Db, user, proposalIdStr, model.ProposalStateApproved, c.Cfg)
		} else if proposalRecord.VoteType == model.ProposalVoteTypeNone {
			if err = c.ProposalService.CreateJobToUpdateNoVoteProposalToNextState(c.Db, proposalRecord.ID, proposalRecord.VoteType, proposalRecord.CreateTs+proposalRecord.PublicitySecond, model.ProposalStateApproved); err != nil {
				log.Error().Msgf("create proposal state change error: %+v", err)
				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error detail:"+err.Error())))
				return
			}
		}
	}

	responseData, err := c.ProposalService.ConvertProposalToFrontendDetailRecord(c.Db, proposalRecord.ID, 0, reqData.MetaforoAccessToken, c.Cfg.MetaforoData.GroupName)
	if err != nil {
		log.Error().Msgf("convert proposal to frontend format error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("load proposal data error detail:"+err.Error())))
		return
	}

	// Return to frontend
	ctx.JSON(http.StatusOK, api.Success(responseData))
}

func (c *ProposalController) AddComment(ctx *gin.Context) {
	addComment := AddCommentData{}
	err := ctx.BindJSON(&addComment)
	if err != nil {
		log.Error().Msgf("bind json error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	if addComment.MetaforoAccessToken == "" {
		log.Error().Msgf("missing metaforo access token")
		sdk.LogUserSideError(ctx, errors.New("missing metaforo access token"))
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("missing metaforo access token")))
		return
	}

	user, _, _, _ := api.ForContext(ctx)
	proposalIdStr := ctx.Param("id")
	proposalRcd, proposalMetaforoData, err := c.ProposalService.GetMetaforoProposalByInternalId(c.Db, proposalIdStr, c.Cfg.MetaforoData.GroupName, addComment.MetaforoAccessToken)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Warn().Msgf("proposal %s not found", proposalIdStr)
			ctx.JSON(http.StatusNotFound, nil)
			return
		} else {
			log.Error().Msgf("get proposal id %s error: %+v", proposalIdStr, err)
			sdk.LogUserSideError(ctx, err)
			ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("get proposal error detail:"+err.Error())))
			return
		}
	}

	replyId := proposalMetaforoData.Thread.FirstPostId
	if addComment.ReplyToMetaforoCommentId != 0 {
		replyId = addComment.ReplyToMetaforoCommentId
	}

	metaforoCommentData, err := metaforo.AddComment(
		addComment.MetaforoAccessToken,
		c.Cfg.MetaforoData.GroupName,
		proposalRcd.GetMetaforoThreadId(),
		addComment.Content,
		fmt.Sprintf("%d", replyId),
		addComment.EditorType,
	)
	if err != nil {
		log.Error().Msgf("add comment to proposal %s error: %+v", proposalIdStr, err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("add comment error detail:"+err.Error())))
		return
	}

	var parentComment model.ProposalComment
	if addComment.ReplyToMetaforoCommentId != 0 {
		err := c.Db.Model(model.ProposalComment{}).Where("metaforo_comment_id = ?", fmt.Sprintf("%d", addComment.ReplyToMetaforoCommentId)).First(&parentComment).Error
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				log.Error().Msgf("get parent comment error: %+v", err)
				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get parent comment error detail:"+err.Error())))
				return
			} else {
				// This branch indicates that the parent comment has been added to metaforo but haven't saved in DB
				// Check whether it can be synced from some API calls
			}
		}
	}

	proposalComment := model.ProposalComment{
		CreateTs:          time.Now().UTC().Unix(),
		ParentID:          parentComment.ID,
		ProposalID:        proposalRcd.ID,
		ProposalRecordID:  proposalRcd.ProposalRecordId,
		Content:           addComment.Content,
		MetaforoCommentId: metaforoCommentData.Id,
		IsRejectComment:   false, // Reject comment is added in other API endpoint
		AuthorWallet:      common.FormatUserWallet(user.Wallet),
	}

	err = c.Db.Create(&proposalComment).Error
	if err != nil {
		log.Error().Msgf("create proposal comment error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal comment error detail:"+err.Error())))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

func (c *ProposalController) EditComment(ctx *gin.Context) {
	editComment := EditCommentData{}
	err := ctx.BindJSON(&editComment)
	if err != nil {
		log.Error().Msgf("bind json error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	if editComment.MetaforoAccessToken == "" {
		log.Error().Msgf("missing metaforo access token")
		sdk.LogUserSideError(ctx, errors.New("missing metaforo access token"))
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("missing metaforo access token")))
		return
	}

	// Check whether the comment modified is reject comment, if yes update the db content
	// db, cfg := api.ForContextDBAndConfig(ctx)
	var rejectComment model.ProposalComment
	err = c.Db.Model(&model.ProposalComment{}).
		Where("metaforo_comment_id = ? AND is_reject_comment = ?", fmt.Sprintf("%d", editComment.MetaforoCommentId), true).
		First(&rejectComment).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Error().Msgf("query reject comment error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("edit comment error detail:"+err.Error())))
		return
	}

	err = metaforo.EditComment(
		editComment.MetaforoAccessToken,
		c.Cfg.MetaforoData.GroupName,
		fmt.Sprintf("%d", editComment.MetaforoCommentId),
		editComment.Content,
		editComment.EditorType,
	)
	if err != nil {
		log.Error().Msgf("edit comment error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("edit comment error detail:"+err.Error())))
		return
	}

	if rejectComment.ID != 0 {
		rejectComment.Content = editComment.Content
		err = c.Db.Save(&rejectComment).Error
		if err != nil {
			log.Error().Msgf("update reject comment error: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("edit comment error detail:"+err.Error())))
			return
		}
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

func (c *ProposalController) DeleteComment(ctx *gin.Context) {
	deleteComment := DeleteCommentData{}
	err := ctx.BindJSON(&deleteComment)
	if err != nil {
		log.Error().Msgf("bind json error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}
	if deleteComment.MetaforoAccessToken == "" {
		log.Error().Msgf("missing metaforo access token")
		sdk.LogUserSideError(ctx, errors.New("missing metaforo access token"))
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("missing metaforo access token")))
		return
	}

	user, _, _, _ := api.ForContext(ctx)
	var rejectComment model.ProposalComment
	err = c.Db.Model(&model.ProposalComment{}).
		Where("metaforo_comment_id = ? AND is_reject_comment = ?", fmt.Sprintf("%d", deleteComment.MetaforoCommentId), true).
		First(&rejectComment).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Reject comment not found, the comment can be deleted
			err = metaforo.DeleteComment(
				deleteComment.MetaforoAccessToken,
				c.Cfg.MetaforoData.GroupName,
				fmt.Sprintf("%d", deleteComment.MetaforoCommentId),
			)
			if err != nil {
				log.Error().Msgf("delete comment error: %+v", err)
				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("delete comment error detail:"+err.Error())))
				return
			}
			ctx.JSON(http.StatusOK, api.Success(nil))
		} else {
			log.Error().Msgf("query reject comment error: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("delete comment error detail:"+err.Error())))
			return
		}
	} else {
		// Reject comment found, the comment can not be deleted
		log.Error().Msgf("try to delete reject comment")
		sdk.LogUserSideError(ctx, fmt.Errorf("user %s try to delete reject comment with comment metaforo id %d", user.Wallet, rejectComment.MetaforoCommentId))
		ctx.JSON(http.StatusBadRequest, api.ServerError(errors.New("reject comment can't be deleted")))
		return
	}
}

func (c *ProposalController) MyList(ctx *gin.Context) {
	user, _, _, _ := api.ForContext(ctx)
	queryParams := ListProposalQueryParams{}
	if err := ctx.Bind(&queryParams); err != nil {
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("query params error: %+v", err),
		})
		return
	}

	// Parse pagination
	page := api.ParseAndConvertPageParam(ctx)

	querySql := fmt.Sprintf("%s WHERE applicant = '%s'", ListProposalsSQL, common.FormatUserWallet(user.Wallet))

	queryPendingSubmitFlag := ctx.Query("pending_submit")
	if queryPendingSubmitFlag == "1" {
		querySql += fmt.Sprintf(" AND state = %d", model.ProposalStatePendingSubmit)
	} else {
		querySql += fmt.Sprintf(" AND state != %d", model.ProposalStatePendingSubmit)
	}

	total, resultRows, err := c.ProposalService.GenerateFrontendProposalRecords(c.Db, querySql, page, false)
	if err != nil {
		log.Error().Msgf("get proposal list error: query sql: %s, err: %+v", querySql, err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get proposal error detail:"+err.Error())))
		return
	}

	// get result rows is vote by me
	// <<
	log.Debug().Msgf("vote query sql check: user: %+v", user)
	if len(resultRows) > 0 && user != nil {
		voteQuerySql := fmt.Sprintf("select * from proposal_user_vote_records where user_wallet = '%s' ", user.Wallet)

		var proposalIdList []string
		for i := 0; i < len(resultRows); i++ {
			proposalIdList = append(proposalIdList, fmt.Sprintf("%d", resultRows[i].ID))
		}

		voteQuerySql += fmt.Sprintf(" AND proposal_id in(%s)", strings.Join(proposalIdList, ","))

		log.Debug().Msgf("vote query sql check: query sql: %s", voteQuerySql)

		var userProposalUserVoteRecord []*model.ProposalUserVoteRecord

		dbErr := c.Db.Raw(voteQuerySql).Find(&userProposalUserVoteRecord).Error
		if dbErr != nil {
			log.Error().Msgf("get proposal list error: query sql: %s, err: %+v", voteQuerySql, dbErr)
		} else {
			resultRows = lo.Map(resultRows, func(r *FrontendProposalListRecord, _ int) *FrontendProposalListRecord {
				for i := 0; i < len(userProposalUserVoteRecord); i++ {
					log.Debug().Msgf("vote query sql check find vote data: ProposalID: %d, ID: %d", userProposalUserVoteRecord[i].ProposalID, r.ID)
					if userProposalUserVoteRecord[i].ProposalID == r.ID {
						r.IsVoted = true
						break
					}
				}

				return r
			})
		}
	}
	// >>

	ctx.JSON(http.StatusOK, api.Success(api.ListReplyData{
		Page:  queryParams.Page,
		Size:  queryParams.Size,
		Total: total,
		Rows:  resultRows,
	}))

}

func (c *ProposalController) GetProposalsUsedForCreatingProjects(ctx *gin.Context) {
	user, _, _, _ := api.ForContext(ctx)
	categoryIdStr := ctx.Query("category_id")

	var newProjectTemplateIds []uint
	tmplQueryParams := model.ProposalTemplate{
		Type: model.ProposalTemplateTypeNewProject,
	}

	queryingCommonProjectsByOwner := false

	// Some proposal templates uses another category ID for closing project proposal.
	if categoryIdStr != "" {
		categoryId, err := strconv.Atoi(categoryIdStr)
		if err != nil {
			log.Error().Msgf("parse category ID %s to int error: %+v", categoryIdStr, err)
			sdk.LogUserSideError(ctx, err)
			ctx.JSON(http.StatusBadRequest, api.ServerError(fmt.Errorf("parse category ID %s to int error: %+v", categoryIdStr, err)))
			return
		}

		// Try to get category data and check whether it is needed to remap the category ID
		var pCategory model.ProposalCategory
		c.Db.Model(&pCategory).Where("id = ?", categoryId).First(&pCategory)
		if pCategory.CategoryIdForCloseProject != 0 {
			tmplQueryParams.ProposalCategoryID = pCategory.CategoryIdForCloseProject
			if pCategory.Name == internal.AutomationCloseCommonProjectCategory {
				// This is a closing project proposal for common project, change to query by project owner
				queryingCommonProjectsByOwner = true
			}
		} else {
			tmplQueryParams.ProposalCategoryID = uint(categoryId)
		}
	}

	querySql := ""

	if queryingCommonProjectsByOwner {
		closableCommonProjects, err := model.ProjectModel.GetClosableProject(c.Db, user.Wallet, []string{
			internal.ManuallyCreatedCommonProjectCategory,
			internal.AutomationCreatedCommonProjectCategory,
		})

		if err != nil {
			log.Error().Msgf("get closable common project error: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get proposal error detail:"+err.Error())))
			return
		}

		var proposalIds []string
		for _, project := range closableCommonProjects {
			proposalIds = append(proposalIds, project.Proposals...)
		}

		err = c.Db.Model(&model.Proposal{}).Where("id IN (?)", proposalIds).Error
		if err != nil {
			log.Error().Msgf("get create project template id error: err: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get proposal error detail:"+err.Error())))
			return
		}

		if len(proposalIds) == 0 {
			log.Debug().Msgf("no proposals of common project found for user %s", user.Wallet)
			ctx.JSON(http.StatusOK, api.Success([]*FrontendProposalListRecord{}))
			return
		}

		querySql = fmt.Sprintf("%s WHERE p.id IN %s order by sip desc, create_ts desc",
			ListProposalsSQLForGettingCreatingProjectProposal,
			fmt.Sprintf("(%s)", strings.Join(proposalIds, ",")),
		)
	} else {
		err := c.Db.Model(&model.ProposalTemplate{}).Where(tmplQueryParams).Pluck("id", &newProjectTemplateIds).Error
		if err != nil {
			log.Error().Msgf("get create project template id error: err: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get proposal error detail:"+err.Error())))
			return
		}

		if len(newProjectTemplateIds) == 0 {
			log.Warn().Msgf("no opening project template found for category: %s", categoryIdStr)
			ctx.JSON(http.StatusOK, api.Success([]*FrontendProposalListRecord{}))
			return
		}

		querySql = fmt.Sprintf("%s WHERE applicant = '%s' AND proposal_template_id IN (%s) AND state = %d AND p.sip != 0 AND projects.status IN (%s) order by sip desc, create_ts desc",
			ListProposalsSQLForGettingCreatingProjectProposal,
			common.FormatUserWallet(user.Wallet),
			strings.Join(lo.Map(newProjectTemplateIds, func(tmpId uint, _ int) string {
				return fmt.Sprintf("%d", tmpId)
			}), ","),
			model.ProposalStateExecuted,
			strings.Join(lo.Map([]model.ProjectStatus{model.ProjectStatusOpen, model.ProjectStatusCloseFailed}, func(projectStatus model.ProjectStatus, _ int) string {
				return fmt.Sprintf("'%s'", projectStatus)
			}), ","),
		)
	}

	// The passed in query seg has already sorted by sip desc, no need to specify it in listBySip param
	_, resultRows, err := c.ProposalService.GenerateFrontendProposalRecords(c.Db, querySql, nil, false)
	if err != nil {
		log.Error().Msgf("get proposal list error: query sql: %s, err: %+v", querySql, err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get proposal error detail:"+err.Error())))
		return
	}

	resultRowsWithSipRecords := lo.Map(resultRows, func(r *FrontendProposalListRecord, _ int) *FrontendProposalListRecord {
		if r.Sip != 0 {
			r.Title = fmt.Sprintf("SIP-%d: %s", r.Sip, r.Title)
		}
		return r
	})

	ctx.JSON(http.StatusOK, api.Success(resultRowsWithSipRecords))
}

func (c *ProposalController) Withdraw(ctx *gin.Context) {
	user, _, _, _ := api.ForContext(ctx)
	proposalIdStr := ctx.Param("id")
	_, err := c.ProposalService.UpdateProposalStateAndLaunchStateChangeActions(c.Db, user, proposalIdStr, model.ProposalStateWithdrawn, c.Cfg)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Warn().Msgf("proposal %s not found", proposalIdStr)
			ctx.JSON(http.StatusNotFound, nil)
			return
		} else {
			sdk.LogUserSideError(ctx, err)
			log.Error().Msgf("parse proposal id %s error: %+v", ctx.Param("id"), err)
			ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
			return
		}
	}
	ctx.JSON(http.StatusOK, api.Success(nil))
}

func (c *ProposalController) Approve(ctx *gin.Context) {
	user, enforcer, _, _ := api.ForContext(ctx)
	formattedWallet := common.FormatUserWallet(user.Wallet)

	//  check permission
	ok, err := enforcer.HasRoleForUser(formattedWallet, internal.RoleHall)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall permission error detail:"+err.Error())))
		return
	}

	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, internal.RoleHall, "access")
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	proposalIdStr := ctx.Param("id")
	_, err = c.ProposalService.UpdateProposalStateAndLaunchStateChangeActions(c.Db, user, proposalIdStr, model.ProposalStateApproved, c.Cfg)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Warn().Msgf("proposal %s not found", proposalIdStr)
			ctx.JSON(http.StatusNotFound, nil)
		} else {
			sdk.LogServerErrorToSentry(ctx, err)
			log.Error().Msgf("update proposal %s state to approved error: %+v", proposalIdStr, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("approve proposal error detail:"+err.Error())))
			return
		}
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

func (c *ProposalController) Reject(ctx *gin.Context) {
	user, enforcer, _, _ := api.ForContext(ctx)
	formattedWallet := common.FormatUserWallet(user.Wallet)

	//  check permission
	ok, err := enforcer.HasRoleForUser(formattedWallet, internal.RoleHall)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall permission error detail:"+err.Error())))
		return
	}

	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, internal.RoleHall, "access")
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	var rejectRequestData RejectProposalData
	if err = ctx.BindJSON(&rejectRequestData); err != nil {
		sdk.LogUserSideError(ctx, err)
		log.Error().Msgf("parse request data error: %+v", err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("parse request data error detail:"+err.Error())))
		return
	}

	if rejectRequestData.MetaforoAccessToken == "" {
		sdk.LogUserSideError(ctx, err)
		log.Error().Msgf("missing metaforo_access_token value")
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("missing metaforo_access_token value")))
		return
	}

	proposalIdStr := ctx.Param("id")
	proposalRecordId, err := c.ProposalService.UpdateProposalStateAndLaunchStateChangeActions(c.Db, user, proposalIdStr, model.ProposalStateRejected, c.Cfg)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Warn().Msgf("proposal %s not found", proposalIdStr)
			ctx.JSON(http.StatusNotFound, nil)
		} else {
			sdk.LogServerErrorToSentry(ctx, err)
			log.Error().Msgf("update proposal %s state to rejected error: %+v", proposalIdStr, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("reject proposal error detail:"+err.Error())))
			return
		}
	}

	rejectComment := model.ProposalComment{
		CreateTs:        time.Now().Unix(),
		UpdateTs:        time.Now().Unix(),
		ProposalID:      proposalRecordId,
		Content:         rejectRequestData.Reason,
		IsHidden:        false,
		IsRejectComment: true,
	}
	c.Db.Save(&rejectComment)

	proposalRecord, err := c.ProposalService.GetProposalFromStringId(c.Db, proposalIdStr)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		log.Error().Msgf("get proposal %s error: %+v", proposalIdStr, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get proposal error detail:"+err.Error())))
		return
	}

	// Add reject comment
	commentData, err := metaforo.AddComment(
		rejectRequestData.MetaforoAccessToken,
		c.Cfg.MetaforoData.GroupName,
		proposalRecord.GetMetaforoThreadId(),
		rejectComment.Content,
		"",
		1)
	if err != nil {
		log.Error().Msgf("add comment to proposal %s error: %+v", proposalIdStr, err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("add comment error detail:"+err.Error())))
		return
	}

	// Comment currently is fetched from Metaforo directly, so only RejectReason is saved in local DB
	c.Db.Model(&rejectComment).Where("id = ?", rejectComment.ID).Update("metaforo_comment_id", fmt.Sprintf("%d", commentData.Id))

	ctx.JSON(http.StatusOK, api.Success(nil))
}

func (c *ProposalController) CheckVotePermission(ctx *gin.Context) {
	user, _, _, _ := api.ForContext(ctx)

	proposalIdString := ctx.Param("id")
	userHasVotePermissionOnThread, err := c.ProposalService.CanUserVoteOnThread(c.Db, user.Wallet, proposalIdString)
	if err != nil {
		log.Error().Msgf("check user vote permission error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("check user permission error detail:"+err.Error())))
		return
	}
	ctx.JSON(http.StatusOK, api.Success(userHasVotePermissionOnThread))
}

func (c *ProposalController) CastVote(ctx *gin.Context) {
	user, _, _, _ := api.ForContext(ctx)

	proposalIdString := ctx.Param("id")
	userHasVotePermissionOnThread, err := c.ProposalService.CanUserVoteOnThread(c.Db, user.Wallet, proposalIdString)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		log.Error().Msgf("check user vote permission error: %+v", err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("check user permission error detail:"+err.Error())))
		return
	}

	if !userHasVotePermissionOnThread {
		err := fmt.Errorf("user %s not met vote gate requirements", user.Wallet)
		log.Err(err)
		sdk.LogForbiddenError(ctx, user.Wallet, fmt.Sprintf("vote in proposal %s", proposalIdString), "cast_vote")
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	reqData := CastVoteData{}
	if err := ctx.BindJSON(&reqData); err != nil {
		log.Error().Msgf("parse request data error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("parse request data error: %+v", err)))
		return
	}

	if err := metaforo.CastVote(
		reqData.MetaforoAccessToken,
		c.Cfg.MetaforoData.GroupName,
		reqData.MetaforoVoteId,
		reqData.MetaforoVoteOptions,
	); err != nil {
		log.Error().Msgf("cast vote error error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("cast vote error detail:"+err.Error())))
		return
	}

	// Get proposal id
	proposalId, _ := strconv.Atoi(proposalIdString)

	// Create proposal user vote record
	err = c.Db.Transaction(func(tx *gorm.DB) error {
		var voteOptions []*model.ProposalVoteOptionRecord
		tx.Model(model.ProposalVoteOptionRecord{}).Where(&model.ProposalVoteOptionRecord{
			ProposalId:     uint(proposalId),
			MetaforoVoteID: reqData.MetaforoVoteId,
		}).Where("metaforo_id IN (?)", reqData.MetaforoVoteOptions).Find(&voteOptions)

		for _, voteOption := range voteOptions {
			userVoteRecordSearchCond := model.ProposalUserVoteRecord{
				ProposalID:                 uint(proposalId),
				ProposalVoteOptionRecordId: voteOption.ID,
				UserWallet:                 common.FormatUserWallet(user.Wallet),
			}
			userVoteRecord := model.ProposalUserVoteRecord{
				ProposalID:                 uint(proposalId),
				ProposalVoteOptionRecordId: voteOption.ID,
				UserWallet:                 common.FormatUserWallet(user.Wallet),
				VoteTs:                     model.GetCurrentUtcEpochSecond(),
			}
			tx.Model(model.ProposalUserVoteRecord{}).
				Where(userVoteRecordSearchCond).
				FirstOrCreate(&userVoteRecord)
		}

		return nil
	})

	if err != nil {
		log.Error().Msgf("create proposal user vote record error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("create proposal user vote record error detail:"+err.Error())))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

func (c *ProposalController) RevokeVote(ctx *gin.Context) {
	reqData := RevokeVoteData{}
	if err := ctx.BindJSON(&reqData); err != nil {
		log.Error().Msgf("parse request data error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("parse request data error: %+v", err)))
		return
	}

	if err := metaforo.RevokeVote(
		reqData.MetaforoAccessToken,
		c.Cfg.MetaforoData.GroupName,
		reqData.MetaforoVoteId,
	); err != nil {
		log.Error().Msgf("revoke vote error error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("revoke vote error detail:"+err.Error())))
		return

	}
	// Remove proposal user vote record
	err := c.Db.Transaction(func(tx *gorm.DB) error {
		// Get proposal user vote record from passed in metaforo vote id
		var proposalVoteOptionRecord *model.ProposalVoteOptionRecord
		err := tx.Model(&model.ProposalVoteOptionRecord{}).Where(&model.ProposalVoteOptionRecord{
			MetaforoVoteID: reqData.MetaforoVoteId,
		}).First(&proposalVoteOptionRecord).Error

		if err != nil {
			log.Error().Msgf("get proposal vote option record error: %+v", err)
			return err
		}

		return tx.Model(&model.ProposalUserVoteRecord{}).Where(&model.ProposalUserVoteRecord{
			ProposalVoteOptionRecordId: proposalVoteOptionRecord.ID,
		}).Delete(&model.ProposalUserVoteRecord{}).Error
	})

	if err != nil {
		log.Error().Msgf("delete proposal user vote record error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("delete proposal user vote record error detail:"+err.Error())))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

func (c *ProposalController) CloseVote(ctx *gin.Context) {
	proposalIdStr := ctx.Param("id")
	reqData := CloseVoteRequest{}
	if err := ctx.BindJSON(&reqData); err != nil {
		log.Error().Msgf("parse request data error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("parse request data error: %+v", err)))
		return
	}

	if err := metaforo.CloseVote(
		reqData.MetaforoAccessToken,
		c.Cfg.MetaforoData.GroupName,
		reqData.MetaforoVoteId,
	); err != nil {
		log.Error().Msgf("close vote error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("close vote error")))
		return
	}

	// update proposal state after getting the vote result
	dbProposal, metaforoProposalResponse, _ := c.ProposalService.GetMetaforoProposalByInternalId(c.Db, proposalIdStr, c.Cfg.MetaforoData.GroupName, reqData.MetaforoAccessToken)
	pollStatusChanged, err := c.ProposalService.UpdateDbVoteOptionRecordsFromMetaforoProposalResponse(c.Db, dbProposal.ID, metaforoProposalResponse)
	if err != nil {
		log.Error().Msgf("update propsal vote option records with metaforo response error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("close vote error detail:"+err.Error())))
		return
	}

	if pollStatusChanged {
		if err = c.ProposalService.HandleProposalPollStatusChange(c.Db, dbProposal.ID, c.Cfg.MetaforoData.GroupName); err != nil {
			log.Error().Msgf("handle proposal poll status change error: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("close vote error detail:"+err.Error())))
			return
		}
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

func (c *ProposalController) ListTemplates(ctx *gin.Context) {
	var dbRcds []*model.ProposalTemplate
	if err := c.Db.Model(&model.ProposalTemplate{}).Preload("Components").Find(&dbRcds).Error; err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list templates failed detail:"+err.Error())))
		return
	}

	respRcds := lo.Map(dbRcds, func(r *model.ProposalTemplate, _ int) *TemplateResponseWithComponents {
		tmplRsp := TemplateResponse{
			ID:               r.ID,
			Name:             r.Name,
			DisplayIndex:     r.DisplayIndex,
			ContentSchema:    r.ContentSchema,
			ScreenshotUri:    r.ScreenshotUri,
			RuleDescription:  r.RuleDesc,
			IsInstantVote:    r.PublicitySecond == 0,
			IsClosingProject: r.Type == model.ProposalTemplateTypeCloseProject,
			VoteType:         r.VoteType,
		}
		return &TemplateResponseWithComponents{
			TemplateResponse: tmplRsp,
			Components:       c.ProposalService.GetTemplateComponents(r),
		}
	})

	ctx.JSON(200, api.Success(respRcds))
}

func (c *ProposalController) ListTemplatesWithPerm(ctx *gin.Context) {
	user, _, _, _ := api.ForContext(ctx)

	sppClient := sdk.GetSppClient()
	userSeepassData, err := api.GetCachedSeepassData(sppClient, user.Wallet, false)

	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		log.Error().Msgf("get user %+v seepass data error: %+v", user, err)
	}

	var rcds []*TemplateResponse
	err = c.Db.Raw(listTemplateWithPermSQL, model.ProposalTemplateTypeCloseProject).Scan(&rcds).Error
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list templates failed detail:"+err.Error())))
		return
	}

	tmplRecords := lo.Map(rcds, func(r *TemplateResponse, _ int) *TemplateResponseWithComponents {
		var tmplDbRcd model.ProposalTemplate
		err = c.Db.Preload("Components").Find(&tmplDbRcd, r.ID).Error
		if err != nil {
			log.Warn().Msgf("try to fetch template record  %d error: %+v", r.ID, err)
			return nil
		}

		var useTemplateVoteGates []*model.ProposalVoteGate
		if err = c.Db.Model(&tmplDbRcd).Association("UseTemplateGates").Find(&useTemplateVoteGates); err != nil {
			log.Warn().Msgf("try to fetch template record  %d error: %+v", r.ID, err)
			return nil
		}

		// Stop using the template if user has no seepass data or template is disabled
		if userSeepassData == nil || tmplDbRcd.IsDisabled {
			r.HasPermToUse = false
		} else {
			permArray := lo.Map(useTemplateVoteGates, func(r *model.ProposalVoteGate, _ int) bool {
				return c.ProposalService.IsUserMetVoteGate(userSeepassData, r)
			})

			r.HasPermToUse = lo.Reduce(permArray, func(rslt bool, r bool, _ int) bool {
				return rslt && r
			}, true)
		}

		components := c.ProposalService.GetTemplateComponents(&tmplDbRcd)

		return &TemplateResponseWithComponents{
			TemplateResponse: *r,
			Components:       components,
		}
	})

	respRcdMap := lo.GroupBy(tmplRecords, func(r *TemplateResponseWithComponents) lo.Tuple3[uint, uint, string] {
		return lo.T3[uint, uint, string](r.CategoryId, r.CategoryDisplayIndex, r.CategoryName)
	})

	respRcds := lo.MapToSlice(respRcdMap, func(categoryIdName lo.Tuple3[uint, uint, string], tmplRcds []*TemplateResponseWithComponents) *TmplWithCategoryNameRecord {
		categoryId, categoryDisplayIndex, categoryName := lo.Unpack3(categoryIdName)
		return &TmplWithCategoryNameRecord{
			CategoryId:           categoryId,
			CategoryDisplayIndex: categoryDisplayIndex,
			CategoryName:         categoryName,
			Templates:            tmplRcds,
		}
	})

	ctx.JSON(200, api.Success(respRcds))
}

func (c *ProposalController) ListAllCategories(ctx *gin.Context) {
	var proposalCategories []*model.ProposalCategory
	err := c.Db.Model(&model.ProposalCategory{}).Where(model.ProposalCategory{IsActive: true}).Find(&proposalCategories).Error
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		log.Error().Msgf("get proposal categories error: %+v", err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get proposal categories error detail:"+err.Error())))
		return
	}

	categoryResp := lo.Map(proposalCategories, func(r *model.ProposalCategory, index int) *FrontendProposalCategory {
		return &FrontendProposalCategory{
			ID:         r.ID,
			ParentID:   r.ParentID,
			Name:       r.Name,
			MetaforoId: r.MetaforoId,
		}
	})

	ctx.JSON(http.StatusOK, api.Reply{
		Data: categoryResp,
	})
}

func (c *ProposalController) ListCategoriesWithPerm(ctx *gin.Context) {
	user, _, _, _ := api.ForContext(ctx)

	sppClient := sdk.GetSppClient()
	userSeepassData, _ := api.GetCachedSeepassData(sppClient, user.Wallet, false)

	var proposalCategories []*model.ProposalCategory
	err := c.Db.Model(&model.ProposalCategory{}).
		Joins("ProposalVoteGate").
		Where(model.ProposalCategory{IsActive: true}).Find(&proposalCategories).Error
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		log.Error().Msgf("get proposal categories error: %+v", err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get proposal categories error detail:"+err.Error())))
		return
	}

	categoryResp := lo.Map(proposalCategories, func(r *model.ProposalCategory, index int) *FrontendProposalCategory {
		return &FrontendProposalCategory{
			ID:         r.ID,
			ParentID:   r.ParentID,
			Name:       r.Name,
			MetaforoId: r.MetaforoId,
			HasPerm:    c.ProposalService.IsUserMetVoteGate(userSeepassData, r.ProposalVoteGate),
		}
	})

	ctx.JSON(http.StatusOK, api.Reply{
		Data: categoryResp,
	})
}
