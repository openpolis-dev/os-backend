package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
	"github.com/theseed-labs/os-backend/internal/storage"
	"gorm.io/gorm"
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
	syncCategoryFlag := syncCommand.Bool("sync-category", false, "sync category flag, default false")
	syncGateFlag := syncCommand.Bool("sync-vote-gate", false, "sync gate flag, default false")

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
		storage.InitGormDBWithLoggerLevel(cfg.DataSource.Dsn, cfg.Casbin.DriverName, logger.Error)
		storage.SeedDbRecords()
		db := storage.GetGormDB()

		if *syncCategoryFlag {
			SyncCategories(db, *syncGroup)
		}
		if *syncGateFlag {
			SyncNftGate(db, *syncGroup)
		}

		for i := *syncStartPage; i < *syncEndPage; i++ {
			log.Debug().Msgf("parse page %d", i)
			SyncProposals(db, *syncGroup, i, *syncSize)
			time.Sleep(2 * time.Second)
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

func SyncProposals(db *gorm.DB, grpName string, page int, size int) {
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
			categoryRecord := model.ProposalCategory{
				MetaforoId: thread.CategoryIndexId,
			}
			err := db.Where(categoryRecord).Find(&categoryRecord).Error
			if err != nil {
				panic(err)
			}

			proposalRecordId := fmt.Sprintf("metaforo:%d", thread.Id)

			// Build content
			contentBlock := model.ProposalContentBlock{
				CreateTs:   thread.UpdatedAt.Unix(),
				Title:      "Proposal Content",
				Content:    fmt.Sprintf("%s", thread.FirstPost.Content),
				ProposalID: 0,
			}

			proposalRecord := model.Proposal{
				CreateTs:           thread.UpdatedAt.Unix(),
				State:              int(model.ProposalStateApproved),
				Title:              thread.Title,
				ContentBlocks:      []*model.ProposalContentBlock{&contentBlock},
				ProposalCategoryID: categoryRecord.ID,
				ProposalRecordId:   proposalRecordId,
				Version:            1,
				Applicant:          "",
				IsHidden:           false,
			}

			cond := model.Proposal{ProposalRecordId: proposalRecordId, Version: 1}
			err, createdCount := upsertDbRcd(tx, &cond, &proposalRecord)
			if err != nil {
				panic(err)
			}
			totalCreatedRecordCount += createdCount
		}

		if totalCreatedRecordCount < len(proposals) {
			log.Debug().Msgf("fetched %d records, created %d records", len(proposals), totalCreatedRecordCount)
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
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
			cond := model.ProposalCategory{MetaforoId: category.CategoryId}
			data := model.ProposalCategory{MetaforoId: category.CategoryId, Name: category.Name}
			err, _ = upsertDbRcd(tx, &cond, &data)
			if err != nil {
				log.Error().Msgf("upsert proposal category record error: %+v", err)
				return err
			}

			if len(category.Children) > 0 {
				for _, childCategory := range category.Children {
					childCond := model.ProposalCategory{MetaforoId: childCategory.CategoryId}
					childData := model.ProposalCategory{Name: childCategory.Name, ParentID: dbCategoryRcd.ID, MetaforoId: childCategory.CategoryId}
					err, _ = upsertDbRcd(tx, &childCond, &childData)
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
			nftGateConf := model.ProposalVoteGate{
				ChainType:    int(pollSetting.ChainType),
				TokenType:    int(pollSetting.TokenType),
				TokenAddress: pollSetting.Address,
				TokenId:      fmt.Sprintf("%d", pollSetting.TokenId),
				Name:         pollSetting.Alias,
				MetaforoId:   pollSetting.Id,
			}
			err := db.Where(model.ProposalVoteGate{MetaforoId: pollSetting.Id}).Assign(nftGateConf).FirstOrCreate(&nftGateConf).Error
			if err != nil {
				panic(err)
			}
		}
	}
}

func upsertDbRcd[T any](db *gorm.DB, cond *T, data *T) (error, int) {
	var dbRcd T
	var createdRecord = 0
	tx := db.Where(cond).Limit(1).Find(&dbRcd)
	if tx.Error != nil {
		log.Error().Msgf("upsertDbRcd error: %s", tx.Error)
	} else if tx.RowsAffected == 0 {
		// No record found, create it
		db.Create(data)
		createdRecord = 1
	} else if tx.RowsAffected == 1 {
		// Update existing record
		db.Where(cond).Create(data)
	} else {
		err = fmt.Errorf("upsertDbRcd error: unexpected rows affected: %d", tx.RowsAffected)
		log.Error().Msgf(err.Error())
		return err, 0
	}
	return nil, createdRecord
}
