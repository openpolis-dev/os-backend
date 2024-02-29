package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	ethereumCommon "github.com/ethereum/go-ethereum/common"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/storage"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

const DealDateLayoutFormat1 = "2006/1/2"
const DealDateLayoutFormat2 = "2006-01-02 15:04:05"
const DealDateLayoutFormat3 = "2006-01-02"
const DealDateLayoutFormat4 = "2006-1-2"

const DefaultDetailSheetName = "明细"
const SummarizedSheetName = "数据透视"

type LoaderConfig struct {
	Dsn             string
	Scheme          string
	Mode            string
	SeasonName      string
	AssetName       string
	DetailSheetName string
	CleanDBFlag     bool
	LogLevel        int
	InputFile       string
	CreateMissing   bool
}

type DetailRecordSchema struct {
	SeasonName      string
	Username        string
	EntityName      string
	UserWallet      string
	DealDate        time.Time
	DealTs          int64
	AssetName       string
	AssetAmount     decimal.Decimal
	DetailedType    string
	Comment         string
	ProposalLink    string
	AppState        string
	AddressVerified bool
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
		currentSeason, err := model.GetCurrentSeason(db)
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
		seasonRcds, err := model.GetSeasonsByName(db, seasonNames)
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
	log.Debug().Msgf("rows length: %d", len(rows))

	return rows, nil
}

func tryParseDatetime(dateStr string) (time.Time, error) {
	parsedDate, err := time.ParseInLocation(DealDateLayoutFormat1, dateStr, internal.ProjectTimezone)
	if err != nil {
		// Try parse date time with another format
		parsedDate, err = time.ParseInLocation(DealDateLayoutFormat2, dateStr, internal.ProjectTimezone)
		if err != nil {
			parsedDate, err = time.ParseInLocation(DealDateLayoutFormat3, dateStr, internal.ProjectTimezone)
			if err != nil {
				parsedDate, err = time.ParseInLocation(DealDateLayoutFormat4, dateStr, internal.ProjectTimezone)
				if err != nil {
					return time.Time{}, err
				}
			}
		}
	}
	return parsedDate, nil
}

func loadDetailSheet(filePath string, sheetName string, assets map[string]bool) ([]*DetailRecordSchema, error) {
	rows, err := loadRows(filePath, sheetName)
	if err != nil {
		return nil, err
	}

	detailRecords := lo.Map(rows[2:], func(r []string, idx int) *DetailRecordSchema {
		dealDate, err := tryParseDatetime(r[4])
		if err != nil {
			panic(err)
		}
		assetAmount, err := decimal.NewFromString(strings.ReplaceAll(r[6], ",", ""))
		if err != nil {
			panic(err)
		}

		proposalLink := ""
		comment := ""
		detailedType := ""

		if len(r) > 7 {
			detailedType = strings.TrimSpace(r[7])
		}
		if len(r) > 8 {
			comment = strings.TrimSpace(r[8])
		}

		if len(r) > 9 {
			proposalLink = strings.TrimSpace(r[9])
		}

		appState := "completed"
		if len(r) == 11 {
			if r[10] == "未发放" {
				appState = "processing"
			}
		}

		stripedWallet := strings.TrimSpace(r[3])
		if !ethereumCommon.IsHexAddress(stripedWallet) {
			fmt.Printf("invalid user wallet: %d: |%s|\n", idx, stripedWallet)
		}

		userWallet := common.ToChecksumAddress(stripedWallet)

		return &DetailRecordSchema{
			SeasonName:      r[0],
			Username:        r[1],
			EntityName:      r[2],
			UserWallet:      userWallet,
			DealDate:        dealDate.In(internal.ProjectTimezone),
			DealTs:          dealDate.In(internal.ProjectTimezone).UTC().Unix(),
			AssetName:       r[5],
			AssetAmount:     assetAmount,
			DetailedType:    detailedType,
			Comment:         comment,
			ProposalLink:    proposalLink,
			AppState:        appState,
			AddressVerified: ethereumCommon.IsHexAddress(userWallet),
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
			Wallet:        common.ToChecksumAddress(r[0]),
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
		log.Debug().Msgf("%d applications will be removed", len(appRcdIds))
		err = db.Delete(&model.ApplicationAuditLog{}, "application_id IN ?", appRcdIds).Error
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

	err = db.Raw("? UNION ? UNION ?",
		db.Model(&model.Project{}).Select("id, name, 'project' as entity_type"),
		db.Model(&model.Guild{}).Select("id, name, 'guild' as entity_type"),
		db.Model(&model.CommonBudgetSource{}).Select("id, name, 'common_budget_source' as entity_type"),
	).Find(&dbEntities).Error

	if err != nil {
		panic(err)
	}

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
			if dbEntityMap[strings.TrimSpace(xslxRcd.EntityName)].Id == 0 {
				missingEntity[strings.TrimSpace(xslxRcd.EntityName)] = true
			}

			entityInfo[strings.TrimSpace(xslxRcd.EntityName)] = dbEntityMap[strings.TrimSpace(xslxRcd.EntityName)]
		}
	}

	if len(missingEntity) > 0 {
		panic(fmt.Errorf("some entites are missing in DB:\n %+v", strings.Join(lo.Keys(missingEntity), "\n")))
	}

	// DB tasks
	// Create user record if not existing
	err = db.Transaction(func(tx *gorm.DB) error {
		for wallet := range userWallets {
			userRcd := model.User{
				Wallet:    common.ToChecksumAddress(wallet),
				CreatedAt: time.Now().In(internal.ProjectTimezone),
				UpdatedAt: time.Now().In(internal.ProjectTimezone),
				CreateTs:  model.GetCurrentUtcEpochSecond(),
				UpdateTs:  model.GetCurrentUtcEpochSecond(),
			}

			err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&userRcd).Error

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
		Type:         "NEW_REWARD",
		SeasonId:     seasonIds[0],
		State:        model.ApplicationStateApproved,
		ShadowRecord: true,
	}
	err = db.Save(&appBundle).Error
	if err != nil {
		panic(err)
	}

	return db.Transaction(func(tx *gorm.DB) error {
		for i, r := range recordsWillBeImported {
			if !r.AddressVerified {
				fmt.Printf("Skipping record %+v due to address verification failure\n", r)
				continue
			}

			if i%10 == 0 {
				fmt.Printf("%d", i)
			} else {
				fmt.Print(".")
			}

			seasonId, _ := seasonNames[r.SeasonName]
			entityInfo := entityInfo[strings.TrimSpace(r.EntityName)]

			appState := model.ApplicationStateApproved
			if r.AppState == "completed" {
				appState = model.ApplicationStateCompleted
			}

			application := model.Application{
				Type:             model.ApplicationNewReward,
				Applicant:        "",
				State:            model.ApplicationState(appState),
				CreatedAt:        r.DealDate,
				UpdatedAt:        r.DealDate,
				CreateTs:         r.DealTs,
				UpdateTs:         r.DealTs,
				DetailedType:     r.DetailedType,
				TargetUserWallet: common.ToChecksumAddress(r.UserWallet),
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
				{ApplicationID: application.ID, Operation: model.AuditActionNew, LogTs: r.DealTs, PostState: model.ApplicationStateOpen},
				{ApplicationID: application.ID, Operation: model.AuditActionApprove, LogTs: r.DealTs, PreState: model.ApplicationStateOpen, PostState: model.ApplicationStateApproved},
			}

			if r.AppState == "completed" {
				auditLogs = append(auditLogs, &model.ApplicationAuditLog{ApplicationID: application.ID, Operation: model.AuditActionProcess, LogTs: r.DealTs, PreState: model.ApplicationStateApproved, PostState: model.ApplicationStateProcessing})
				auditLogs = append(auditLogs, &model.ApplicationAuditLog{ApplicationID: application.ID, Operation: model.AuditActionComplete, LogTs: r.DealTs, PreState: model.ApplicationStateProcessing, PostState: model.ApplicationStateCompleted})
			}

			err = tx.Save(auditLogs).Error
			if err != nil {
				log.Error().Msgf("Insert audit log for application %d error, audit logs: %+v, err: %+v", application.ID, auditLogs, err)
				return err
			}
		}
		return nil
	})
}

func main() {
	// Define the command-line flags
	var config LoaderConfig
	flag.StringVar(&config.Dsn, "dsn", "", "Database connect string")
	flag.StringVar(&config.Scheme, "scheme", "", "Database scheme, used for mysql connection string")
	flag.StringVar(&config.Mode, "mode", "load", "load data mode or verify data")
	flag.StringVar(&config.SeasonName, "season", "", "Specify seasons the application will import, multiple seasons can be split by comma. If not given, the current season will be used. And pass `all` for processing all season records")
	flag.StringVar(&config.AssetName, "asset", "", "Specify assets the application will import, multiple assets can be split by comma. If not given, all assets will be imported")
	flag.StringVar(&config.DetailSheetName, "detail-sheet", DefaultDetailSheetName, "Specify detail sheet name in the Excel file, the default value will be used if not given")
	flag.BoolVar(&config.CleanDBFlag, "clean-db", false, "Clean the database with specified seasons before importing.")
	flag.IntVar(&config.LogLevel, "v", 0, "Log level: 0 for no logs, 1 for normal logs, 2 for verbose logs, 3 for very verbose logs.")
	flag.StringVar(&config.InputFile, "input", "summary.xsls", "Specify input xslx file")

	// Parse the command-line flags
	flag.Parse()

	// Prepare db connection
	dbDsn := config.Dsn
	if dbDsn == "" {
		envDbUrl := os.Getenv("DATABASE_URL")
		if envDbUrl != "" {
			dbDsn = envDbUrl
		} else {
			dbDsn = "sqlite://./os-backend.db"
		}
	}

	if config.Scheme != "" {
		storage.InitGormDBWithLoggerLevel(dbDsn, config.Scheme, logger.Error)
	} else {
		parsedURI, err := url.Parse(dbDsn)
		if err != nil {
			panic(fmt.Errorf("parse database URI %s error, please confirm", dbDsn))
		}
		storage.InitGormDBWithLoggerLevel(dbDsn, parsedURI.Scheme, logger.Error)
	}
	db := storage.GetGormDB()
	err := model.MigrateTables(db)
	if err != nil {
		panic(err)
	}
	db.Logger = logger.Default.LogMode(logger.Silent)

	// parse season data
	seasons, err := parseSeasonParams(db, config.SeasonName)
	if err != nil {
		panic(err)
	}

	importAssets := make(map[string]bool)
	for _, name := range strings.Split(config.AssetName, ",") {
		importAssets[name] = true
	}

	switch config.Mode {
	case "load":
		// Read and parse xslx file
		// The sheet used for loading is sheet with DetailSheetName.
		detailedRecords, err := loadDetailSheet(config.InputFile, config.DetailSheetName, importAssets)
		if err != nil {
			panic(err)
		}

		err = saveToDatabase(db, detailedRecords, seasons, config.CleanDBFlag)
		if err != nil {
			panic(err)
		}
	case "verify":
		// verify uses first sheet
		// Get summarized records from Excel worksheet
		log.Warn().Msgf("Note: Please verify the summary sheet name and structure")
		summarizedRecords, err := loadSummarizedSheet(config.InputFile)
		if err != nil {
			panic(err)
		}
		totalSummaryMap := make(map[string]decimal.Decimal)
		for _, record := range summarizedRecords {
			totalSummaryMap[record.Wallet] = record.Total
		}

		// Calculated summarized records from parsed detail worksheet
		detailedRecords, err := loadDetailSheet(config.InputFile, config.DetailSheetName, importAssets)
		if err != nil {
			panic(err)
		}

		aggrUserTotal := make(map[string]decimal.Decimal)

		// aggregate detailed records
		for _, detailedRcd := range detailedRecords {
			model.SetDefaultMapValue(aggrUserTotal, common.ToChecksumAddress(detailedRcd.UserWallet), decimal.Zero)
			aggrUserTotal[common.ToChecksumAddress(detailedRcd.UserWallet)] = aggrUserTotal[detailedRcd.UserWallet].Add(detailedRcd.AssetAmount)
		}

		// Verify whether calculated result is same with Excel result
		for wallet, amount := range aggrUserTotal {
			_wallet := common.ToChecksumAddress(wallet)
			if !totalSummaryMap[_wallet].Equal(amount) {
				log.Error().Msgf("record not equal, wallet: %s, excel amount: %s, calc amount: %s", _wallet, totalSummaryMap[_wallet], amount)
			}
		}

	default:
		panic(fmt.Errorf("unknown mode, should be `load` or `verify`"))
	}
}
