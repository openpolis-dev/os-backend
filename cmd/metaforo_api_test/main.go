package main

import (
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
	"github.com/theseed-labs/os-backend/internal/storage"
	"gorm.io/gorm"
)

const groupName = "seedao"

func main() {
	cfg := config.LoadConfig("config.yml")
	storage.InitGormDB(cfg.DataSource.Dsn, cfg.Casbin.DriverName)
	storage.SeedDbRecords()
	db := storage.GetGormDB()

	SyncCategoriesFromMetaforo(db, groupName)

	metaforoProposals := fetchProposalData(groupName)
	for _, thread := range metaforoProposals {
		log.Error().Msgf("TTT: update  at: %+v", thread.UpdatedAt)
		categoryRecord := model.ProposalCategory{
			MetaforoId: thread.CategoryId,
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

	//metaforo.GetTags(groupName, "21826|C16zyP8o10wY0easORsNiCa1KTxp0AZwICUAXp6W")
	//metaforo.NewCategory(groupName, "test_category", 0, "21826|C16zyP8o10wY0easORsNiCa1KTxp0AZwICUAXp6W")
	//metaforo.GetCategories(groupName)
	//groupInfo, err := metaforo.GetGroupInfo(groupName)
	//if err != nil {
	//	panic(err)
	//}
	//
	//jsonStr, _ := json.MarshalIndent(groupInfo, "  ", "  ")

}

func fetchProposalData(grpName string) []*metaforo.Thread {
	proposals, _ := metaforo.ListProposals(&metaforo.PaginationParams{
		Page:            1,
		PerPage:         10,
		CategoryIndexId: 0,
		TagId:           0,
		Sort:            "",
		GroupName:       grpName,
	})

	//jsonStr, _ := json.MarshalIndent(proposals[0], "  ", "  ")
	//fmt.Printf("TTT: proposals: %s", jsonStr)

	return proposals
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
		err := db.Where(model.ProposalCategory{MetaforoId: category.Id}).
			Assign(model.ProposalCategory{Name: category.Name}).
			FirstOrCreate(&dbCategoryRcd).Error
		if err != nil {
			panic(err)
		}

		if len(category.Children) > 0 {
			for _, childCategory := range category.Children {
				var dbChildCategoryRcd model.ProposalCategory
				err := db.Where(model.ProposalCategory{MetaforoId: childCategory.Id}).
					Assign(model.ProposalCategory{Name: childCategory.Name, ParentID: dbCategoryRcd.ID}).
					FirstOrCreate(&dbChildCategoryRcd).Error
				if err != nil {
					panic(err)
				}
			}
		}
	}
}
