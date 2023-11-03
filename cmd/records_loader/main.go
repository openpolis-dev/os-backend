package main

import (
	"flag"
	"fmt"
	"os"
	"reflect"
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
)

const DealDateLayoutFormat1 = "2006/1/2"
const DealDateLayoutFormat2 = "2006-01-02 15:04:05"

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

func loadXslsFile(filePath string) ([]DetailRecordSchema, error) {
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

	sheetName := "明细"
	rows, err := f.GetRows(sheetName)
	if err != nil {
		log.Error().Msgf("Failed to get rows from sheet '%s': %v", sheetName, err)
		return nil, err
	}

	header := rows[1]
	log.Error().Msgf("TTT: Table header: %+v, type: %+v", header, reflect.TypeOf(header))

	detailRecords := lo.Map(rows[2:], func(r []string, _ int) DetailRecordSchema {
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

		return DetailRecordSchema{
			SeasonName:   r[0],
			Username:     r[1],
			EntityName:   r[2],
			UserWallet:   r[3],
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

func saveToDatabase(db *gorm.DB, rcds []DetailRecordSchema, seasonRcds []*model.Season, cleanDbFlag bool) error {
	seasonIds := lo.Map(seasonRcds, func(r *model.Season, _ int) uint {
		return r.ID
	})

	// clear application and related audit log records with specified seasons if set cleanDbFlag to true
	if cleanDbFlag {
		//applications := db.Model(&model.Application{}).Where("season_id IN ?", seasonIds)
		var auditLogs []model.ApplicationAuditLog
		db.Model(&model.ApplicationAuditLog{}).Where("application_id IN (select ID from applications where season_id IN ?)", seasonIds).Find(&auditLogs)
		log.Error().Msgf("TTT: got audit logs count: %d", len(auditLogs))
	}
	// Filter out records with specified seasons

	return nil
}

func main() {
	// Define the command-line flags
	dsn := flag.String("dsn", "", "Database connect string")
	seasonName := flag.String("season", "", "Specify seasons the application will import, multiple seasons can be split by comma. If not given, the current season will be used. And pass `all` for processing all season records")
	cleanDBFlag := flag.Bool("clean-db", false, "Clean the database with specified seasons before importing.")
	//logLevelFlag := flag.Int("v", 0, "Log level: 0 for no logs, 1 for normal logs, 2 for verbose logs, 3 for very verbose logs.")
	//outputSQLFlag := flag.String("output-sql", "", "Specify the output SQL.")
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

	// parse season data
	seasons, err := parseSeasonParams(db, *seasonName)
	if err != nil {
		panic(err)
	}

	// read and parse xslx file
	detailedRecords, err := loadXslsFile(*inputFile)
	if err != nil {
		panic(err)
	}

	err = saveToDatabase(db, detailedRecords, seasons, *cleanDBFlag)

	// Your application logic goes here
}
