package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
	"github.com/theseed-labs/os-backend/internal/storage"
	"gorm.io/gorm"
)

func main() {
	syncCommand := flag.NewFlagSet("sync", flag.ExitOnError)
	createCommand := flag.NewFlagSet("create", flag.ExitOnError)

	// Define flags for sync command
	syncPage := syncCommand.Int("page", 1, "Page number")
	syncSize := syncCommand.Int("size", 10, "Page size")
	syncGroup := syncCommand.String("group", "testttt", "Group name")

	// Define flags for create command
	createAccessToken := createCommand.String("access-token", "", "Access token")
	createGroup := createCommand.String("group", "testttt", "Group name")

	// Parse the command-line arguments
	if len(os.Args) < 2 {
		fmt.Println("Subcommand is required")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "sync":
		syncCommand.Parse(os.Args[2:])
		cfg := config.LoadConfig("config.yml")
		storage.InitGormDB(cfg.DataSource.Dsn, cfg.Casbin.DriverName)
		storage.SeedDbRecords()
		db := storage.GetGormDB()

		SyncCategoriesFromMetaforo(db, *syncGroup)
		SyncProposals(db, *syncGroup, *syncPage, *syncSize)
	case "create":
		createCommand.Parse(os.Args[2:])
		resp, err := metaforo.CreateProposal(*createAccessToken, *createGroup, "1", "test from metaforo API", "# Test content\n## TEST", nil, nil)
		if err != nil {
			panic(err)
		}

		fmt.Sprintf("response: %+v", resp)
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

	//jsonStr, _ := json.MarshalIndent(proposals[0], "  ", "  ")
	//fmt.Printf("TTT: proposals: %s", jsonStr)

	for _, thread := range proposals {
		log.Error().Msgf("TTT: update  at: %+v", thread.UpdatedAt)
		categoryRecord := model.ProposalCategory{
			MetaforoId: thread.CategoryIndexId,
		}
		err := db.Where(categoryRecord).Find(&categoryRecord).Error
		if err != nil {
			panic(err)
		}

		proposalRecordId := fmt.Sprintf("metaforo:%d", thread.FirstPostId)

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
			IsVoted:            false,
		}

		err = db.Where(model.Proposal{ProposalRecordId: proposalRecordId, Version: 1}).Assign(&proposalRecord).FirstOrCreate(&proposalRecord).Error
		if err != nil {
			panic(err)
		}
	}
}

func SyncCategoriesFromMetaforo(db *gorm.DB, grpName string) {
	categories, _ := metaforo.GetCategories(grpName)

	//jsonStr, _ := json.MarshalIndent(categories, "  ", "  ")
	//fmt.Printf("TTT: categories: %s", jsonStr)

	for _, category := range categories {
		var dbCategoryRcd model.ProposalCategory
		// TODO: ParentId is not handled here, can be updated manually after sync
		if category.ParentId != 0 {
			log.Warn().Msgf("category record %+v has parent id, please update manually", category)
		}
		err := db.Where(model.ProposalCategory{MetaforoId: category.CategoryId}).
			Assign(model.ProposalCategory{Name: category.Name}).
			FirstOrCreate(&dbCategoryRcd).Error
		if err != nil {
			panic(err)
		}

		if len(category.Children) > 0 {
			for _, childCategory := range category.Children {
				var dbChildCategoryRcd model.ProposalCategory
				err := db.Where(model.ProposalCategory{MetaforoId: childCategory.CategoryId}).
					Assign(model.ProposalCategory{Name: childCategory.Name, ParentID: dbCategoryRcd.ID}).
					FirstOrCreate(&dbChildCategoryRcd).Error
				if err != nil {
					panic(err)
				}
			}
		}
	}
}
