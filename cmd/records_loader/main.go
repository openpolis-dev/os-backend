package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal/service"
	"github.com/theseed-labs/os-backend/internal/storage"
)

func main() {
	// Define the command-line flags
	dsn := flag.String("dsn", "", "Database connect string")
	seasonFlag := flag.String("season", "", "Specify seasons the application will import, multiple seasons can be split by comma. If not given, the current season will be used.")
	cleanDBFlag := flag.Bool("clean-db", false, "Clean the database with specified seasons before importing.")
	logLevelFlag := flag.Int("v", 0, "Log level: 0 for no logs, 1 for normal logs, 2 for verbose logs, 3 for very verbose logs.")
	outputSQLFlag := flag.String("output-sql", "", "Specify the output SQL.")

	// Parse the command-line flags
	flag.Parse()

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

	var seasonIds []uint

	if *seasonFlag == "" {
		currentSeason, err := service.GetCurrentSeason(db)
		if err != nil {
			panic("get current season error")
		}
		seasonIds = []uint{currentSeason.ID}
	} else {
		seasonIds = lo.Map(strings.Split(*seasonFlag, ","), func(seasonIdStr string, _ int) uint {
			intVal, err := strconv.Atoi(seasonIdStr)
			if err != nil {
				panic(err)
			}

			return uint(intVal)
		})
	}

	log.Error().Msgf("TTT: seasion ids: %+v", seasonIds)

	// Access the flag values
	season := *seasonFlag
	cleanDB := *cleanDBFlag
	logLevel := *logLevelFlag
	outputSQL := *outputSQLFlag

	// Print the flag values
	fmt.Printf("Season: %s\n", season)
	fmt.Printf("CleanDB: %v\n", cleanDB)
	fmt.Printf("Log level: %d\n", logLevel)
	fmt.Printf("Output SQL: %s\n", outputSQL)

	// Your application logic goes here
}
