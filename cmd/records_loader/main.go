package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/service"
	"github.com/theseed-labs/os-backend/internal/storage"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

const DealDateLayoutFormat1 = "2006/1/2"
const DealDateLayoutFormat2 = "2006-01-02 15:04:05"

const DetailSheetName = "明细"
const SummarizedSheetName = "数据透视"

type DetailRecordSchema struct {
	SeasonName   string
	Username     string
	EntityName   string
	UserWallet   string
	DealDate     time.Time
	AssetName    string
	AssetAmount  decimal.Decimal
	DetailedType string
	Comment      string
	ProposalLink string
}

type SummarizedRecordSchema struct {
	Wallet        string
	SeasonsCredit []decimal.Decimal
	Total         decimal.Decimal
}

type EntityProps struct {
	EntityType string
	Id         uint
	Name       string
}

func parseSeasonParams(db *gorm.DB, seasonParamValue string) ([]*model.Season, error) {
	if seasonParamValue == "" {
		currentSeason, err := service.GetCurrentSeason(db)
		if err != nil {
			return nil, err
		}
		return []*model.Season{currentSeason}, nil
	} else if strings.EqualFold(seasonParamValue, "all") {
		var seasonRcds []*model.Season
		err := db.Model(&model.Season{}).Find(&seasonRcds).Error
		return seasonRcds, err
	} else {
		seasonNames := strings.Split(seasonParamValue, ",")
		seasonRcds, err := service.GetSeasonsByName(db, seasonNames)
		if err != nil {
			return nil, err
		}

		if len(seasonNames) != len(seasonRcds) {
			log.Warn().Msgf("queried season count does not equals to given season id, please check input. queried seasons: %+v, record returned: %+v", seasonNames, seasonRcds)
		}

		return seasonRcds, nil
	}
}

func loadRows(filePath string, sheetName string) ([][]string, error) {
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return nil, err
	}

	defer func() {
		// Close the spreadsheet.
		if err := f.Close(); err != nil {
			fmt.Println(err)
		}
	}()

	rows, err := f.GetRows(sheetName)
	if err != nil {
		log.Error().Msgf("Failed to get rows from sheet '%s': %v", sheetName, err)
		return nil, err
	}

	return rows, nil
}

func loadDetailSheet(filePath string) ([]*DetailRecordSchema, error) {
	rows, err := loadRows(filePath, DetailSheetName)
	if err != nil {
		return nil, err
	}

	detailRecords := lo.Map(rows[2:], func(r []string, _ int) *DetailRecordSchema {
		var dealDate time.Time
		dealDate, err = time.Parse(DealDateLayoutFormat1, r[4])
		if err != nil {
			// Try parse date time with another format
			dealDate, err = time.Parse(DealDateLayoutFormat2, r[4])
			if err != nil {
				panic(err)
			}
		}
		assetAmount, err := decimal.NewFromString(strings.ReplaceAll(r[6], ",", ""))
		if err != nil {
			panic(err)
		}

		proposalLink := ""
		comment := ""
		detailedType := ""

		if len(r) > 7 {
			detailedType = r[7]
		}
		if len(r) > 8 {
			comment = r[8]
		}

		if len(r) > 9 {
			proposalLink = r[9]
		}

		return &DetailRecordSchema{
			SeasonName:   r[0],
			Username:     r[1],
			EntityName:   r[2],
			UserWallet:   model.FormatUserWallet(r[3]),
			DealDate:     dealDate.In(internal.ProjectTimezone),
			AssetName:    r[5],
			AssetAmount:  assetAmount,
			DetailedType: detailedType,
			Comment:      comment,
			ProposalLink: proposalLink,
		}
	})

	return detailRecords, nil
}

func loadSummarizedSheet(filePath string) ([]*SummarizedRecordSchema, error) {
	rows, err := loadRows(filePath, SummarizedSheetName)
	if err != nil {
		return nil, err
	}
	log.Debug().Msgf("summarized sheet rows length: %d", len(rows))

	summarizedRcds := lo.Map(rows[4:], func(r []string, _ int) *SummarizedRecordSchema {
		lineColumnCount := len(r)
		var err error
		var seasonCredits []decimal.Decimal

		for _, creditStr := range r[1 : lineColumnCount-1] {
			var credit decimal.Decimal
			if creditStr == "" {
				credit = decimal.Zero
			} else {
				credit, err = decimal.NewFromString(creditStr)
				if err != nil {
					log.Error().Msgf("parse credit %s error: %+v", creditStr, err)
					return nil
				}
			}
			seasonCredits = append(seasonCredits, credit)
		}

		totalCredit, err := decimal.NewFromString(r[lineColumnCount-1])
		if err != nil {
			log.Error().Msgf("parse total credit error: %+v", err)
			return nil
		}

		return &SummarizedRecordSchema{
			Wallet:        model.FormatUserWallet(r[0]),
			SeasonsCredit: seasonCredits,
			Total:         totalCredit,
		}
	})

	return summarizedRcds, nil
}

func saveToDatabase(db *gorm.DB, rcds []*DetailRecordSchema, seasonRcds []*model.Season, cleanDbFlag bool) error {
	var err error
	// Prepare season data
	var seasonIds []uint
	seasonNames := make(map[string]uint)

	for _, r := range seasonRcds {
		seasonIds = append(seasonIds, r.ID)
		seasonNames[r.Name] = r.ID
	}

	// clear application and related audit log records with specified seasons if set cleanDbFlag to true
	if cleanDbFlag {
		var appRcdIds []uint
		db.Model(&model.Application{}).Where("season_id IN ?", seasonIds).Select("id").Find(&appRcdIds)
		err = db.Model(&model.ApplicationAuditLog{}).Where("application_id IN ?", appRcdIds).Delete(&model.ApplicationAuditLog{}).Error
		if err != nil {
			panic(err)
		}
		err = db.Model(&model.Application{}).Where("season_id IN ?", seasonIds).Delete(&model.Application{}).Error
		if err != nil {
			panic(err)
		}
	}

	// Collect all entities in database
	var dbEntities []EntityProps
	dbEntityMap := make(map[string]EntityProps)

	db.Raw("? UNION ?",
		db.Select("id, name, 'project' as 'entity_type'").Model(&model.Project{}),
		db.Select("id, name, 'guild' as 'entity_type'").Model(&model.Guild{}),
	).Find(&dbEntities)

	// TODO: check whether there are records with same name but different entity_type
	for _, entity := range dbEntities {
		dbEntityMap[entity.Name] = entity
	}

	// Filter out xsls records with specified seasons
	userWallets := make(map[string]bool)
	var recordsWillBeImported []*DetailRecordSchema
	entityInfo := make(map[string]EntityProps) // Entity data

	missingEntity := make(map[string]bool)

	for _, xslxRcd := range rcds {
		if _, exists := seasonNames[xslxRcd.SeasonName]; exists {
			userWallets[xslxRcd.UserWallet] = true
			recordsWillBeImported = append(recordsWillBeImported, xslxRcd)
			if dbEntityMap[xslxRcd.EntityName].Id == 0 {
				missingEntity[xslxRcd.EntityName] = true
			}

			entityInfo[xslxRcd.EntityName] = dbEntityMap[xslxRcd.EntityName]
		}
	}

	if len(missingEntity) > 0 {
		panic(fmt.Errorf("some entites are missing in DB: %+v", missingEntity))
	}

	// DB tasks
	// Create user record if not existing
	err = db.Transaction(func(tx *gorm.DB) error {
		for wallet, _ := range userWallets {
			err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.User{Wallet: model.FormatUserWallet(wallet)}).Error

			if err != nil {
				log.Error().Msgf("find or create user error: %+v", err)
				return err
			}
		}
		return nil
	})

	if err != nil {
		panic(err)
	}

	appBundle := model.AppBundle{
		Type:     "NEW_REWARD",
		SeasonId: seasonIds[0],
		State:    model.ApplicationStateApproved,
	}
	err = db.Save(&appBundle).Error
	if err != nil {
		panic(err)
	}

	return db.Transaction(func(tx *gorm.DB) error {
		for i, r := range recordsWillBeImported {
			if i%10 == 0 {
				fmt.Printf("%d", i)
			} else {
				fmt.Print(".")
			}

			seasonId, _ := seasonNames[r.SeasonName]
			entityInfo := entityInfo[r.EntityName]

			application := model.Application{
				Type:             model.ApplicationNewReward,
				Applicant:        "",
				State:            model.ApplicationStateCompleted,
				CreatedAt:        r.DealDate,
				UpdatedAt:        r.DealDate,
				DetailedType:     r.DetailedType,
				TargetUserWallet: model.FormatUserWallet(r.UserWallet),
				AssetName:        r.AssetName,
				AssetAmount:      r.AssetAmount,
				EntityType:       entityInfo.EntityType,
				Comment:          r.Comment,
				EntityId:         entityInfo.Id,
				SeasonId:         seasonId,
				BundleId:         appBundle.ID,
			}

			err = tx.Save(&application).Error
			if err != nil {
				return err
			}

			auditLogs := []*model.ApplicationAuditLog{
				{ApplicationID: application.ID, LogTs: time.Now().In(internal.ProjectTimezone), PostState: model.ApplicationStateApproved},
				{ApplicationID: application.ID, LogTs: time.Now().In(internal.ProjectTimezone), PreState: model.ApplicationStateOpen, PostState: model.ApplicationStateApproved},
				{ApplicationID: application.ID, LogTs: time.Now().In(internal.ProjectTimezone), PreState: model.ApplicationStateApproved, PostState: model.ApplicationStateProcessing},
				{ApplicationID: application.ID, LogTs: time.Now().In(internal.ProjectTimezone), PreState: model.ApplicationStateProcessing, PostState: model.ApplicationStateCompleted},
			}

			err = tx.Save(auditLogs).Error
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func main() {
	// Define the command-line flags
	dsn := flag.String("dsn", "", "Database connect string")
	mode := flag.String("mode", "load", "load data mode or verify data")
	seasonName := flag.String("season", "", "Specify seasons the application will import, multiple seasons can be split by comma. If not given, the current season will be used. And pass `all` for processing all season records")
	cleanDBFlag := flag.Bool("clean-db", false, "Clean the database with specified seasons before importing.")
	//logLevelFlag := flag.Int("v", 0, "Log level: 0 for no logs, 1 for normal logs, 2 for verbose logs, 3 for very verbose logs.")
	inputFile := flag.String("input", "summary.xsls", "Specify input xslx file")

	// Parse the command-line flags
	flag.Parse()

	// Prepare db connection
	dbDsn := *dsn
	if dbDsn == "" {
		envDbUrl := os.Getenv("DATABASE_URL")
		if envDbUrl != "" {
			dbDsn = envDbUrl
		} else {
			dbDsn = "localhost:3306/mysql"
		}
	}
	storage.InitGormDB(dbDsn)
	db := storage.GetGormDB()
	db.Logger = logger.Default.LogMode(logger.Silent)

	// parse season data
	seasons, err := parseSeasonParams(db, *seasonName)
	if err != nil {
		panic(err)
	}

	switch *mode {
	case "load":
		// Read and parse xslx file
		// The sheet used for loading is sheet with DetailSheetName.
		detailedRecords, err := loadDetailSheet(*inputFile)
		if err != nil {
			panic(err)
		}

		err = saveToDatabase(db, detailedRecords, seasons, *cleanDBFlag)
		if err != nil {
			panic(err)
		}
	case "verify":
		// verify uses first sheet
		// Get summarized records from Excel worksheet
		log.Warn().Msgf("Note: Please verify the summary sheet name and structure")
		summarizedRecords, err := loadSummarizedSheet(*inputFile)
		if err != nil {
			panic(err)
		}
		totalSummaryMap := make(map[string]decimal.Decimal)
		for _, record := range summarizedRecords {
			totalSummaryMap[record.Wallet] = record.Total
		}

		// Calculated summarized records from parsed detail worksheet
		detailedRecords, err := loadDetailSheet(*inputFile)
		if err != nil {
			panic(err)
		}

		aggrUserTotal := make(map[string]decimal.Decimal)

		// aggregate detailed records
		for _, detailedRcd := range detailedRecords {
			model.SetDefaultMapValue(aggrUserTotal, model.FormatUserWallet(detailedRcd.UserWallet), decimal.Zero)
			aggrUserTotal[model.FormatUserWallet(detailedRcd.UserWallet)] = aggrUserTotal[detailedRcd.UserWallet].Add(detailedRcd.AssetAmount)
		}

		// Verify whether calculated result is same with Excel result
		for wallet, amount := range aggrUserTotal {
			_wallet := model.FormatUserWallet(wallet)
			if !totalSummaryMap[_wallet].Equal(amount) {
				log.Error().Msgf("record not equal, wallet: %s, excel amount: %s, calc amount: %s", _wallet, totalSummaryMap[_wallet], amount)
			}
		}

	default:
		panic(fmt.Errorf("unknown mode, should be `load` or `verify`"))
	}
}
