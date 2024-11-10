package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/api"
	proposal "github.com/theseed-labs/os-backend/internal/api/proposal"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
	"github.com/theseed-labs/os-backend/internal/storage"
	"github.com/valyala/fasthttp"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

var err error

func main() {
	syncCommand := flag.NewFlagSet("sync", flag.ExitOnError)
	voteCommand := flag.NewFlagSet("vote", flag.ExitOnError)
	showCommand := flag.NewFlagSet("show", flag.ExitOnError)

	// Define flags for sync command
	syncStartPage := syncCommand.Int("start-page", 1, "Start page number")
	syncEndPage := syncCommand.Int("end-page", 1, "End page number")
	syncSize := syncCommand.Int("size", 10, "Page size")
	syncGroup := syncCommand.String("group", "testttt", "Group name")
	syncQuillToMdService := syncCommand.String("quill-service", "https://delta2html-api-0x2t.hello-what.workers.dev", "service to convert quill to markdown")
	syncCategoryFlag := syncCommand.Bool("sync-category", false, "sync category flag, default false")
	syncGateFlag := syncCommand.Bool("sync-vote-gate", false, "sync gate flag, default false")

	syncProposalVoteGate := syncCommand.Bool("proposal-vote-gate", false, "sync proposal voting gate, default false")
	syncUserVoteFlag := syncCommand.Bool("user-vote", false, "sync user vote flag, default false")
	syncWaitSeconds := syncCommand.Int("wait-seconds", 10, "wait seconds between each proposal")

	// vote related
	voteAccessToken := voteCommand.String("access-token", "", "Access token")
	voteGroup := voteCommand.String("group", "testttt", "Group name")
	voteId := voteCommand.Int("id", 0, "Vote id")
	voteStartTime := voteCommand.String("start", time.Now().UTC().Format(time.RFC3339), "Vote start time, default is now")
	voteEndTime := voteCommand.String("end", time.Now().UTC().Format(time.RFC3339), "Vote end time, default is now")

	showThreadId := showCommand.Int("id", 0, "Thread id")
	showGroup := showCommand.String("group", "testttt", "group name")

	// Parse the command-line arguments
	if len(os.Args) < 2 {
		fmt.Println("Subcommand is required")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "sync":
		syncCommand.Parse(os.Args[2:])
		cfg := config.LoadConfig("config.yml")
		storage.InitGormDBWithLoggerLevel(cfg.DataSource.Dsn, cfg.Casbin.DriverName, logger.Warn)
		storage.SeedDbRecords()
		db := storage.GetGormDB()

		if *syncCategoryFlag {
			SyncCategories(db, *syncGroup)
		}

		if *syncGateFlag {
			SyncNftGate(db, *syncGroup)
		}

		if *syncUserVoteFlag {
			SyncUserVote(db, *syncGroup, *syncWaitSeconds)
		}

		if *syncProposalVoteGate {
			SyncProposalVoteGate(db, cfg, *syncGroup, *syncWaitSeconds)
		}

		for i := *syncStartPage; i < *syncEndPage; i++ {
			log.Debug().Msgf("parse page %d", i)
			SyncProposalList(db, *syncGroup, i, *syncSize, *syncQuillToMdService)
			time.Sleep(30 * time.Second)
		}

	case "vote":
		voteCommand.Parse(os.Args[2:])
		startTs, err := time.Parse(time.RFC3339, *voteStartTime)
		if err != nil {
			panic(err)
		}
		endTs, err := time.Parse(time.RFC3339, *voteEndTime)
		if err != nil {
			panic(err)
		}
		fmt.Printf("StartTs: %s, endTs: %s\n", startTs, endTs)
		metaforo.UpdateVoteTime(*voteAccessToken, *voteGroup, *voteId, startTs.Unix(), endTs.Unix())
	case "show":
		showCommand.Parse(os.Args[2:])
		proposal, _ := metaforo.GetProposal(*showThreadId, *showGroup, "", 0)
		api.PrintStructAsJson(proposal, "")
	default:
		fmt.Println("Unknown subcommand:", os.Args[1])
		os.Exit(1)
	}
}

// SyncProposalList syncs proposal from list API, this API returns category, title, brief content and poll status.
// Some detailed data like poll detail, comments, etc. should be fetched from detailed API.
func SyncProposalList(db *gorm.DB, grpName string, page int, size int, quillServiceUrl string) {
	proposals, _ := metaforo.ListProposals(&metaforo.PaginationParams{
		Page:            page,
		PerPage:         size,
		CategoryIndexId: 0,
		TagId:           0,
		Sort:            "",
		GroupName:       grpName,
	})

	err = db.Transaction(func(tx *gorm.DB) error {
		var totalCreatedRecordCount = 0
		for _, thread := range proposals {
			if thread.PollStatus == "on" {
				log.Debug().Msgf("skip voting thread: %+v", thread)
				continue
			}

			if thread.CategoryIndexId == -1 {
				log.Warn().Msgf("skip category index id is -1: %+v", thread)
				continue
			}

			categoryRecord := model.ProposalCategory{
				MetaforoId: uint(thread.CategoryIndexId),
			}
			err := db.Where(categoryRecord).Find(&categoryRecord).Error
			if err != nil {
				api.PrintStructAsJson(thread, "Thread causes the error")
				panic(err)
			}

			proposalRecordId := fmt.Sprintf("metaforo:%d", thread.Id)

			mfUserRcd := model.MetaforoUser{MetaforoUserId: thread.User.Id}
			err, _ = upsertDbRcd(tx, map[string]any{"metaforo_user_id": thread.User.Id}, &mfUserRcd)
			if err != nil {
				log.Error().Msgf("upsert metaforo user record error: %+v", err)
				return err
			}

			applicantAddr := mfUserRcd.UserWallet
			if mfUserRcd.UserWallet == "" {
				log.Debug().Msgf("Try to get user detail info for userId: %d", thread.User.Id)
				userDetailResp, err := metaforo.UserDetail(thread.User.Id)
				if err != nil {
					log.Error().Msgf("get user detail error: %+v", err)
					return err
				}
				applicantAddr = common.FormatUserWallet(userDetailResp.User.Web3PublicKey)

				// Update metaforo user record with fetched user wallet
				err, _ = upsertDbRcd(tx, map[string]any{"metaforo_user_id": userDetailResp.User.Id}, &model.MetaforoUser{UserWallet: applicantAddr})
				if err != nil {
					log.Error().Msgf("upsert metaforo user record error: %+v", err)
					return err
				}
			}

			if applicantAddr == "" {
				log.Warn().Msgf("fetch user wallet error for user id: %d", thread.User.Id)
			}

			applicantAddr = common.FormatUserWallet(applicantAddr)
			// Create user from given wallet if not existing
			err, createdUserCount := upsertDbRcd(db, map[string]any{"wallet": applicantAddr}, &model.User{
				Wallet: applicantAddr,
			})
			if createdUserCount > 1 {
				log.Debug().Msgf("new user created: %s", applicantAddr)
			}

			pollStatus := thread.PollStatus
			pState := int(model.ProposalStateExecuted)
			if pollStatus == "on" {
				pState = int(model.ProposalStateVoting)
			}

			proposalRecord := model.Proposal{
				CreateTs: thread.UpdatedAt.Unix(),
				Title:    thread.Title,
				State:    pState,
				//ContentBlocks:      []*model.ProposalContentBlock{&contentBlock},
				ProposalCategoryID: categoryRecord.ID,
				ProposalRecordId:   proposalRecordId,
				Version:            1,
				Applicant:          applicantAddr,
				IsHidden:           false,
				//IsImported:         true,
			}

			err, createdCount := upsertDbRcd(tx, map[string]any{"proposal_record_id": proposalRecordId, "version": 1}, &proposalRecord)
			if err != nil {
				log.Error().Msgf("upsert proposal record error: %+v", err)
				panic(err)
			}

			// Build content
			contentBlock := model.ProposalContentBlock{
				CreateTs:   thread.UpdatedAt.Unix(),
				Title:      "Proposal Content",
				ProposalID: proposalRecord.ID,
			}
			contentStr := fmt.Sprintf("%s", thread.FirstPost.Content)
			if thread.FirstPost.EditorType == 0 {
				// Quill format data, convert to html and back to markdown
				req := fasthttp.AcquireRequest()
				resp := fasthttp.AcquireResponse()
				defer fasthttp.ReleaseRequest(req)
				defer fasthttp.ReleaseResponse(resp)

				req.Header.SetMethod("POST")
				req.SetRequestURI(quillServiceUrl)
				req.Header.SetContentType("application/json")

				reqData := map[string]string{"data": contentStr}
				reqDataBytes, err := json.Marshal(reqData)
				if err != nil {
					panic(err)
				}
				req.SetBody(reqDataBytes)
				if err = fasthttp.Do(req, resp); err != nil {
					log.Error().Msgf("quill service error: %+v", err)
					panic(err)
				}

				mdBytes := resp.Body()
				contentBlock.Content = string(mdBytes)
			} else {
				contentBlock.Content = contentStr
			}

			err, _ = upsertDbRcd(tx, map[string]any{"proposal_id": proposalRecord.ID}, &contentBlock)
			if err != nil {
				log.Error().Msgf("upsert proposal content block error: %+v", err)
				panic(err)
			}

			// Create fake proposal vote record
			err, _ = upsertDbRcd(tx, map[string]any{"proposal_id": proposalRecord.ID}, &model.ProposalVoteRecord{
				Title:      "",
				State:      "",
				ProposalID: proposalRecord.ID,
			})

			totalCreatedRecordCount += createdCount
		}

		if totalCreatedRecordCount < len(proposals) {
			log.Debug().Msgf("fetched %d records, created %d records", len(proposals), totalCreatedRecordCount)
		}
		return nil
	})
	if err != nil {
		log.Error().Msgf("transaction error: %+v", err)
		panic(err)
	}
}

func SyncProposalDetail(db *gorm.DB, grpName string, page int, size int) {
	metaforo.GetProposal(0, grpName, "", 0)
}

func SyncCategories(db *gorm.DB, grpName string) {
	categories, _ := metaforo.GetCategories(grpName)
	err = db.Transaction(func(tx *gorm.DB) error {
		for _, category := range categories {
			var dbCategoryRcd model.ProposalCategory

			// TODO: ParentId is not handled here, can be updated manually after sync
			if category.ParentId != 0 {
				log.Warn().Msgf("category record %+v has parent id, please update manually", category)
			}
			data := model.ProposalCategory{MetaforoId: category.CategoryId, Name: category.Name}
			err, _ = upsertDbRcd(tx, map[string]any{"metaforo_id": category.CategoryId}, &data)
			if err != nil {
				log.Error().Msgf("upsert proposal category record error: %+v", err)
				return err
			}

			if len(category.Children) > 0 {
				for _, childCategory := range category.Children {
					childData := model.ProposalCategory{Name: childCategory.Name, ParentID: dbCategoryRcd.ID, MetaforoId: childCategory.CategoryId}
					err, _ = upsertDbRcd(tx, map[string]any{"metaforo_id": childCategory.CategoryId}, &childData)
					if err != nil {
						log.Error().Msgf("upsert child proposal category record error: %+v", err)
						panic(err)
					}
				}
			}
		}
		return nil
	})
}

func SyncNftGate(db *gorm.DB, grpName string) {
	groupInfo, _ := metaforo.GetGroupInfo(grpName)
	_ = db.AutoMigrate(&model.ProposalVoteGate{})

	if groupInfo != nil {
		for _, pollSetting := range groupInfo.PollSetting {
			tokenIdStr := fmt.Sprintf("%d", pollSetting.TokenId)
			nftGateConf := model.ProposalVoteGate{
				ChainType:    int(pollSetting.ChainType),
				TokenType:    int(pollSetting.TokenType),
				TokenAddress: common.FormatUserWallet(pollSetting.Address),
				TokenId:      tokenIdStr,
				Name:         pollSetting.Alias,
				MetaforoId:   pollSetting.Id,
			}
			err, createdCnt := upsertDbRcd(db, map[string]any{
				"chain_type":    int(pollSetting.ChainType),
				"token_type":    int(pollSetting.TokenType),
				"token_address": common.FormatUserWallet(pollSetting.Address),
				"token_id":      tokenIdStr,
			}, &nftGateConf)
			if err != nil {
				panic(err)
			}
			log.Error().Msgf("upsert nft gate record: %+v, created %d records", nftGateConf, createdCnt)
		}
	}
}

func SyncUserVote(db *gorm.DB, grpName string, waitSeconds int) {
	var proposalRcd []*model.Proposal
	var query = db.Model(&model.Proposal{}).Where("not user_vote_record_saved AND proposal_record_id is not null AND proposal_record_id != ''").Order("id desc")

	err := query.Find(&proposalRcd).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Debug().Msgf("no proposal not updated")
			return
		} else {
			log.Error().Msgf("get proposal records error: %+v", err)
			return
		}
	}

	for _, p := range proposalRcd {
		log.Debug().Msgf("update user vote record for proposal: %d, group: %s", p.ID, grpName)
		err = proposal.UpdateUserVoteRecordViaMetaforo(db, grpName, p.ID)
		if err != nil {
			if strings.Contains(err.Error(), "thread not found") {
				log.Warn().Msgf("proposal record %d is not found in metaforo", p.ID)
				continue
			} else {
				log.Error().Msgf("update proposalRcd user vote record error: %+v", err)
				return
			}
		}
		time.Sleep(time.Duration(waitSeconds) * time.Second)
	}
	// Get proposal list from metaforo
}

func SyncProposalVoteGate(db *gorm.DB, cfg *config.Config, grpName string, waitSeconds int) {
	var proposalRcd []*model.Proposal

	err := db.Model(&model.Proposal{}).Where("proposal_record_id is not null AND proposal_record_id != '' AND vote_gate_id is null").Order("id desc").Find(&proposalRcd).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Debug().Msgf("no proposal not updated")
			return
		} else {
			log.Error().Msgf("get proposal records error: %+v", err)
			return
		}
	}

	var voteGateRcds []*model.ProposalVoteGate
	err = db.Model(&model.ProposalVoteGate{}).Find(&voteGateRcds).Error
	if err != nil {
		log.Error().Msgf("get vote gate records error: %+v", err)
		return
	}

	vgMfDbIdMapping := make(map[string]uint)
	for _, v := range voteGateRcds {
		vgMfDbIdMapping[fmt.Sprintf("%s:%s", v.TokenAddress, v.TokenId)] = v.ID
	}

	for _, p := range proposalRcd {
		log.Debug().Msgf("update vote gate for proposal: %d", p.ID)
		resp, err := metaforo.GetProposal(p.GetMetaforoThreadId(), grpName, cfg.MetaforoData.AccessToken, 0)
		if err != nil {
			if strings.Contains(err.Error(), "thread not found") {
				log.Warn().Msgf("proposal record %d is not found in metaforo", p.ID)
			} else {
				log.Error().Msgf("update proposalRcd vote gate error: %+v", err)
			}
			continue
		}
		if len(resp.Thread.Polls) == 0 {
			log.Error().Msgf("no poll found for proposal: %d, title: %s, mf id: %d", p.ID, p.Title, p.GetMetaforoThreadId())
			continue
		}

		mfVoteGateTokenAddr := resp.Thread.Polls[0].TokenAddress
		mfVoteGateTokenId := resp.Thread.Polls[0].TokenId

		if mfVoteGateTokenAddr == "" {
			log.Error().Msgf("no vote gate found for proposal: %d", p.ID)
			continue
		}

		mfVgTokenKey := fmt.Sprintf("%s:%d", mfVoteGateTokenAddr, mfVoteGateTokenId)

		if vgDbId, found := vgMfDbIdMapping[mfVgTokenKey]; !found {
			log.Error().Msgf("vote gate not found in db: %d", mfVgTokenKey)
			continue
		} else {
			log.Debug().Msgf("update vote gate for proposal: %d, mf vote gate id: %s, db vote gate id: %d", p.ID, mfVgTokenKey, vgDbId)
			err = db.Model(&p).Update("vote_gate_id", vgDbId).Error
			if err != nil {
				log.Error().Msgf("update proposalRcd vote gate error: %+v", err)
				continue
			}
		}

		time.Sleep(time.Duration(waitSeconds) * time.Second)
	}
}

func upsertDbRcd[T any](db *gorm.DB, cond map[string]any, data T) (error, int) {
	var dbRcd T
	var createdRecord = 0
	tx := db.Where(cond).Limit(1).Find(&dbRcd)

	if tx.Error != nil {
		log.Error().Msgf("upsertDbRcd error: %s", tx.Error)
	} else if tx.RowsAffected == 0 {
		// No record found, create it
		db.Clauses(clause.Returning{}).Create(&data)
		createdRecord = 1
	} else if tx.RowsAffected == 1 {
		// Update existing record
		db.Clauses(clause.Returning{}).Where(cond).Updates(&data)
	} else {
		err = fmt.Errorf("upsertDbRcd error: unexpected rows affected: %d", tx.RowsAffected)
		log.Error().Msgf(err.Error())
		return err, 0
	}
	return nil, createdRecord
}
