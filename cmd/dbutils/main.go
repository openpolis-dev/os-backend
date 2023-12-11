package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"

	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/storage"
	"gorm.io/gorm"
)

var err error

type MigrateConfig struct {
	Dsn               string
	Scheme            string
	Force             bool
	MigrateTableFlag  bool // Execute autoMigrate for all tables
	UpgradeWalletFlag bool // Upgrade wallet address to checksum version
	UpdateSeasonFlag  bool // Update season record to correct values
}

var seasons = []*model.Season{
	{Idx: 0, StartAt: 0, EndAt: 0},
	{Idx: 1, StartAt: 0, EndAt: 0},
	{Idx: 2, StartAt: 0, EndAt: 0},
	{Idx: 3, StartAt: 0, EndAt: 0},
	{Idx: 4, StartAt: 0, EndAt: 0},
	{Idx: 5, StartAt: 0, EndAt: 0},
}

func autoMigrateDb(db *gorm.DB) error {
	return db.AutoMigrate(
		&model.User{},
		&model.UserNonce{},
		&model.UserAssetRecord{},
		&model.Project{},
		&model.ProjectBudget{},
		&model.Guild{},
		&model.GuildBudget{},
		&model.AppBundle{},
		&model.AppBundleAuditLog{},
		&model.Season{},
		&model.Application{},
		&model.ApplicationAuditLog{},
		&model.TreasuryAsset{},
		&model.TreasuryDetailedRecord{},
		&model.TreasuryAuditLog{},
		&model.Event{},
		&model.Push{},
	)
}

func updateWallet(db *gorm.DB, tableName string, walletField string, whereClause string) error {
	var walletList []string
	err := db.Table(tableName).Where(whereClause).Distinct().Pluck(walletField, &walletList).Error

	if err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		for _, origWallet := range walletList {
			err = tx.Table(tableName).Where(fmt.Sprintf("%s = ?", walletField), origWallet).Update(walletField, common.FormatUserWallet(origWallet)).Error
			if err != nil {
				tx.Rollback()
				return err
			}
		}
		return nil
	})
}

func MigrateDB(config *MigrateConfig) error {
	var err error
	var gormDB *gorm.DB
	scheme := config.Scheme
	fmt.Printf("migrate config %+v\n", config)
	if scheme != "" {
		gormDB, err = storage.BuildGormClient(config.Scheme, config.Dsn)
		if err != nil {
			return err
		}
	} else {
		dbUrl, err := url.Parse(config.Dsn)
		scheme = dbUrl.Scheme
		gormDB, err = storage.BuildGormClient(dbUrl.Scheme, config.Dsn)
		if err != nil {
			return err
		}
	}

	if config.MigrateTableFlag {
		err = autoMigrateDb(gormDB)
		if err != nil {
			return err
		}
	}

	if config.UpgradeWalletFlag {
		if scheme == "mysql" {
			err = gormDB.Exec("SET foreign_key_checks = 0;").Error
			if err != nil {
				return err
			}
		}

		if err = updateWallet(gormDB, "user_asset_records", "user_wallet", "1=1"); err != nil {
			return err
		}
		if err = updateWallet(gormDB, "users", "wallet", "1=1"); err != nil {
			return nil
		}
		if err = updateWallet(gormDB, "casbin_rule", "v0", "ptype='g'"); err != nil {
			return err
		}
		if err = updateWallet(gormDB, "app_bundles", "applicant", "applicant is not null"); err != nil {
			return err
		}
		if err = updateWallet(gormDB, "app_bundle_audit_logs", "operator", "operator is not null"); err != nil {
			return err
		}
		if err = updateWallet(gormDB, "applications", "target_user_wallet", "target_user_wallet is not null"); err != nil {
			return err
		}
		if err = updateWallet(gormDB, "application_audit_logs", "target_user_wallet", "target_user_wallet is not null"); err != nil {
			return err
		}

		if scheme == "mysql" {
			err = gormDB.Exec("SET foreign_key_checks = 1;").Error
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func main() {
	migrateCmd := flag.NewFlagSet("migrate", flag.ExitOnError)
	var migrateConfig MigrateConfig
	migrateCmd.StringVar(&migrateConfig.Dsn, "dsn", "", "Specify the database")
	migrateCmd.StringVar(&migrateConfig.Scheme, "scheme", "", "Specify the database scheme")
	migrateCmd.BoolVar(&migrateConfig.Force, "force", false, "Specify the force")
	migrateCmd.BoolVar(&migrateConfig.MigrateTableFlag, "migrate-table", false, "Specify the force")
	migrateCmd.BoolVar(&migrateConfig.UpgradeWalletFlag, "upgrade-wallet", false, "Specify the force")
	migrateCmd.BoolVar(&migrateConfig.UpdateSeasonFlag, "update-season", false, "Specify the force")

	if len(os.Args) < 2 {
		fmt.Println("Usage: dbutils <command> [arguments]")
		fmt.Println("Available commands:")
		fmt.Println("  migrate")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "migrate":
		err = migrateCmd.Parse(os.Args[2:])
		if err != nil {
			panic(err)
		}
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}

	err := MigrateDB(&migrateConfig)
	if err != nil {
		panic(err)
	}
}
