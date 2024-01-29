package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/storage"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

var err error

type MigrateConfig struct {
	Scheme            string
	Force             bool
	MigrateTableFlag  bool // Execute autoMigrate for all tables
	UpgradeWalletFlag bool // Upgrade wallet address to checksum version
}

type SeedDataConfig struct {
	DataSeedFile string // yaml file used for data seeding
}

type dataSeedRecords struct {
	Records []struct {
		Name       string   `yaml:"name"`
		Category   string   `yaml:"category"`
		Schema     string   `yaml:"schema"`
		Compoments []string `yaml:"components"`
	} `yaml:"records"`
}

func autoMigrateDb(db *gorm.DB) error {
	return storage.MigrateTables(db)
}

func updateWallet(db *gorm.DB, tableName string, walletField string, whereClause string) error {
	if tableName != "projects" && tableName != "guilds" {
		var walletList []string
		err := db.Table(tableName).Where(whereClause).Distinct().Pluck(walletField, &walletList).Error

		if err != nil {
			return err
		}

		return db.Transaction(func(tx *gorm.DB) error {
			for _, origWallet := range walletList {
				// Only processing lower cased wallet address
				if strings.ToLower(origWallet) == origWallet {
					err = tx.Table(tableName).Where(fmt.Sprintf("%s = ?", walletField), origWallet).Update(walletField, common.FormatUserWallet(origWallet)).Error
					if err != nil {
						tx.Rollback()
						return err
					}
				}
			}
			return nil
		})
	} else {
		// Projects and Guilds saves user wallet address in `members` and `sponsors` fields, which are list of JSON object
		var projectList []*model.Project
		err := db.Model(&model.Project{}).Distinct().Select("id", "members", "sponsors").Find(&projectList).Error
		if err != nil {
			return err
		}

		err = db.Transaction(func(tx *gorm.DB) error {
			for _, r := range projectList {
				updatedMembers := lo.Map(r.Members, func(wallet string, _ int) string {
					return common.FormatUserWallet(wallet)
				})
				updatedSponsors := lo.Map(r.Sponsors, func(wallet string, _ int) string {
					return common.FormatUserWallet(wallet)
				})
				err = tx.Model(&model.Project{}).Where("id = ?", r.ID).Updates(model.Project{Members: updatedMembers, Sponsors: updatedSponsors}).Error
				if err != nil {
					tx.Rollback()
					return err
				}
			}
			return nil
		})

		var guildList []*model.Guild
		err = db.Model(&model.Guild{}).Distinct().Select("id", "members", "sponsors").Find(&guildList).Error
		if err != nil {
			return err
		}

		err = db.Transaction(func(tx *gorm.DB) error {
			for _, r := range guildList {
				updatedMembers := lo.Map(r.Members, func(wallet string, _ int) string {
					return common.FormatUserWallet(wallet)
				})
				updatedSponsors := lo.Map(r.Sponsors, func(wallet string, _ int) string {
					return common.FormatUserWallet(wallet)
				})
				err = tx.Model(&model.Guild{}).Where("id = ?", r.ID).Updates(model.Guild{Members: updatedMembers, Sponsors: updatedSponsors}).Error
				if err != nil {
					tx.Rollback()
					return err
				}
			}
			return nil
		})

		return nil
	}
}

func MigrateDB(config *MigrateConfig, db *gorm.DB) error {
	if config.MigrateTableFlag {
		err = autoMigrateDb(db)
		if err != nil {
			return err
		}
	}

	if config.UpgradeWalletFlag {
		if err = updateWallet(db, "user_asset_records", "user_wallet", "1=1"); err != nil {
			return err
		}
		if err = updateWallet(db, "users", "wallet", "1=1"); err != nil {
			return nil
		}
		if err = updateWallet(db, "casbin_rule", "v0", "ptype='g'"); err != nil {
			return err
		}
		if err = updateWallet(db, "app_bundles", "applicant", "applicant is not null"); err != nil {
			return err
		}
		if err = updateWallet(db, "app_bundle_audit_logs", "operator", "operator is not null"); err != nil {
			return err
		}
		if err = updateWallet(db, "applications", "target_user_wallet", "target_user_wallet is not null"); err != nil {
			return err
		}
		if err = updateWallet(db, "applications", "applicant", "applicant not in ('', null)"); err != nil {
			return err
		}
		if err = updateWallet(db, "pushes", "creator_wallet", "creator_wallet is not null"); err != nil {
			return err
		}
		if err = updateWallet(db, "user_nonces", "wallet", "1=1"); err != nil {
			return err
		}

		// The wallet record in Projects and Guilds table are list of string, need to be handled in other way
		if err = updateWallet(db, "projects", "", ""); err != nil {
			return err
		}
	}

	return nil
}

func SeedData(cfg SeedDataConfig, db *gorm.DB) error {
	seedFile, err := os.Open(cfg.DataSeedFile)
	if err != nil {
		return err
	}
	yamlFile, err := io.ReadAll(seedFile)
	if err != nil {
		return err
	}

	// Unmarshal the YAML data into the Config struct
	var config map[string]dataSeedRecords
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		return err
	}

	for tableName, rcdData := range config {
		db.Transaction(func(tx *gorm.DB) error {
			for _, r := range rcdData.Records {
				switch tableName {
				case "proposal_components":
					dbRcd := model.ProposalComponent{
						CreateTs: model.GetCurrentUtcEpochSecond(),
						UpdateTs: model.GetCurrentUtcEpochSecond(),
						Name:     r.Name,
						Schema:   r.Schema,
					}
					if err := db.Model(&model.ProposalComponent{}).Where("name = ?", r.Name).First(&dbRcd).Error; err != nil {
						if errors.Is(err, gorm.ErrRecordNotFound) {
							db.Create(&dbRcd)
						} else {
							return err
						}
					} else {
						db.Save(&dbRcd)
					}
				case "proposal_templates":
					// TODO: Find category record from given category name
					categoryRcd := model.ProposalCategory{}
					err := tx.First(&categoryRcd).Error
					if err != nil {
						return err
					}

					components := lo.Map(r.Compoments, func(compName string, _ int) *model.ProposalComponent {
						dbR := model.ProposalComponent{
							Name: compName,
						}
						if err := tx.Model(&dbR).Where(&dbR).First(&dbR).Error; err != nil {
							if errors.Is(err, gorm.ErrRecordNotFound) {
								tx.Create(&dbR)
							} else {
								panic(err)
							}
						}
						return &dbR
					})

					dbRcd := model.ProposalTemplate{
						CreateTs:           model.GetCurrentUtcEpochSecond(),
						UpdateTs:           model.GetCurrentUtcEpochSecond(),
						Name:               r.Name,
						ContentSchema:      r.Schema,
						ProposalCategoryID: categoryRcd.ID,
						Components:         components,
					}
					if err := db.Model(&model.ProposalTemplate{}).Where("name = ?", r.Name).First(&dbRcd).Error; err != nil {
						if errors.Is(err, gorm.ErrRecordNotFound) {
							db.Create(&dbRcd)
						} else {
							return err
						}
					} else {
						db.Save(&dbRcd)
					}
				}

			}
			return nil
		})
	}
	return nil
}

func main() {
	cfg := config.LoadConfig("config.yml")
	storage.InitGormDB(cfg.DataSource.Dsn, cfg.Casbin.DriverName)
	db := storage.GetGormDB()

	migrateCmd := flag.NewFlagSet("migrate", flag.ExitOnError)
	var migrateConfig MigrateConfig
	migrateCmd.StringVar(&migrateConfig.Scheme, "scheme", "", "Specify the database scheme")
	migrateCmd.BoolVar(&migrateConfig.Force, "force", false, "Specify the force")
	migrateCmd.BoolVar(&migrateConfig.MigrateTableFlag, "migrate-table", false, "Specify the force")
	migrateCmd.BoolVar(&migrateConfig.UpgradeWalletFlag, "upgrade-wallet", false, "Specify the force")

	seedCmd := flag.NewFlagSet("seed", flag.ExitOnError)
	var seedConfig SeedDataConfig
	seedCmd.StringVar(&seedConfig.DataSeedFile, "seed-file", "seed.yml", "yml file saves records")

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
		err := MigrateDB(&migrateConfig, db)
		if err != nil {
			panic(err)
		}
	case "seed":
		err = seedCmd.Parse(os.Args[2:])
		if err != nil {
			panic(err)
		}
		err := SeedData(seedConfig, db)
		if err != nil {
			panic(err)
		}
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}

}
