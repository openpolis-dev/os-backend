package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/casbin/casbin/v2"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/storage"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

type proposalComponentActionRcd struct {
	Command string `yaml:"command" json:"command"`
}

type proposalComponentRcd struct {
	Name          string `yaml:"name" json:"name"`
	Schema        string `yaml:"schema" json:"schema"`
	IsHidden      bool   `yaml:"is_hidden" json:"is_hidden"`
	ApproveAction string `yaml:"approve_action" json:"approve_action"`
	RejectAction  string `yaml:"reject_action" json:"reject_action"`
}

type proposalCategoryRcd struct {
	Name                     string `yaml:"name" json:"name"`
	MetaforoId               uint   `yaml:"metaforo_id" json:"metaforo_id"`
	IsActive                 bool   `yaml:"is_active" json:"is_active"`
	VoteDurationSecond       int64  `yaml:"vote_duration_second" json:"vote_duration_second"`
	PublicitySecond          int64  `yaml:"publicity_second" json:"publicity_second"`
	PendingExecutionSecond   int64  `yaml:"pending_execution_second" json:"pending_execution_second"`
	DisplayIndex             int    `yaml:"display_index" json:"display_index"`
	CanBeVetoed              bool   `yaml:"can_be_vetoed" json:"can_be_vetoed"`
	CloseProjectCategoryName string `yaml:"close_project_category_name" json:"close_project_category_name"`
}

type proposalTmplRcd struct {
	Name                   string `yaml:"name" json:"name"`
	ContentSchema          string `yaml:"content_schema" json:"content_schema"`
	RuleDesc               string `yaml:"rule_desc" json:"rule_desc"`
	IsHidden               bool   `yaml:"is_hidden" json:"is_hidden"`
	CategoryName           string `yaml:"category_name" json:"category_name"`
	VoteDurationSecond     int64  `yaml:"vote_duration_second" json:"vote_duration_second"`
	PublicitySecond        int64  `yaml:"publicity_second" json:"publicity_second"`
	PendingExecutionSecond int64  `yaml:"pending_execution_second" json:"pending_execution_second"`
	VoteType               int    `yaml:"vote_type" json:"vote_type"`
	Type                   int    `yaml:"type" json:"type"`
	DisplayIndex           int    `yaml:"display_index" json:"display_index"`
	IsCustomTemplate       bool   `yaml:"is_custom_template" json:"is_custom_template"`
	ComponentNameList      string `yaml:"component_name_list" json:"component_name_list"`
	ExtraResultCheckRule   string `yaml:"extra_result_check_rule" json:"extra_result_check_rule"`
}

type templateComponentRcd struct {
	TemplateName  string `yaml:"template_name" json:"template_name"`
	ComponentName string `yaml:"component_name" json:"component_name"`
}

type sysVarRcd struct {
	Name     string `yaml:"name" json:"name"`
	NumValue int    `yaml:"num_value" json:"num_value"`
	StrValue string `yaml:"str_value" json:"str_value"`
}

type prjRcd struct {
	Name         string `yaml:"name" json:"name"`
	Status       string `yaml:"status" json:"status"`
	Sponsors     string `yaml:"sponsors" json:"sponsors"`
	Desc         string `yaml:"desc" json:"desc"`
	Sip          string `yaml:"s_ip" json:"s_ip"`
	Category     string `yaml:"category" json:"category"`
	ApprovalLink string `yaml:"approval_link" json:"approval_link"`
	OverLink     string `yaml:"over_link" json:"over_link"`
	ContantWay   string `yaml:"contant_way" json:"contant_way"`
	OfficialLink string `yaml:"official_link" json:"official_link"`
	Label        string `yaml:"label" json:"label"`
}

type guildRcd struct {
	Name         string `yaml:"name" json:"name"`
	Logo         string `yaml:"logo" json:"logo"`
	Status       string `yaml:"status" json:"status"`
	Sponsors     string `yaml:"sponsors" json:"sponsors"`
	Desc         string `yaml:"desc" json:"desc"`
	CreateTs     int64  `yaml:"create_ts" json:"create_ts"`
	ContantWay   string `yaml:"contant_way" json:"contant_way"`
	OfficialLink string `yaml:"official_link" json:"official_link"`
}

type commonBudgetSourceRcd struct {
	Name  string `yaml:"name" json:"name"`
	State string `yaml:"state" json:"state"`
}

func autoMigrateDb(db *gorm.DB) error {
	return model.MigrateTables(db)
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
	var seedDataBucket map[string][]any
	err = yaml.Unmarshal(yamlFile, &seedDataBucket)
	if err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if compActionRcds, found := seedDataBucket["proposal_component_actions"]; found {
			for _, r := range compActionRcds {
				var rr proposalComponentActionRcd
				dataBytes, err := yaml.Marshal(r)
				if err != nil {
					panic(err)
				}
				err = yaml.Unmarshal(dataBytes, &rr)
				if err != nil {
					panic(err)
				}

				dbRcd := model.ProposalComponentAction{
					CreateTs: model.GetCurrentUtcEpochSecond(),
					UpdateTs: model.GetCurrentUtcEpochSecond(),
					Command:  rr.Command,
				}
				createTx := tx.Where(&model.ProposalComponentAction{Command: rr.Command}).Assign(&dbRcd).FirstOrCreate(&dbRcd)
				if err = createTx.Error; err != nil {
					return err
				} else if createTx.RowsAffected == 0 {
					tx.Updates(&dbRcd)
				}
			}
		}

		if categoryRcds, found := seedDataBucket["proposal_categories"]; found {
			categoryWithCloseProject := map[uint]string{}
			for _, r := range categoryRcds {
				var rr proposalCategoryRcd
				dataBytes, err := yaml.Marshal(r)
				if err != nil {
					panic(err)
				}
				err = yaml.Unmarshal(dataBytes, &rr)
				if err != nil {
					panic(err)
				}

				dbRcd := model.ProposalCategory{
					Name:         rr.Name,
					MetaforoId:   rr.MetaforoId,
					DisplayIndex: rr.DisplayIndex,
					IsActive:     rr.IsActive,
					CanBeVetoed:  rr.CanBeVetoed,
				}
				dbRcd.PublicitySecond = rr.PublicitySecond
				dbRcd.VoteDurationSecond = rr.VoteDurationSecond
				dbRcd.PendingExecutionSecond = rr.PendingExecutionSecond

				createTx := tx.Where(&model.ProposalCategory{Name: rr.Name}).Assign(&dbRcd).FirstOrCreate(&dbRcd)
				if err = createTx.Error; err != nil {
					return err
				} else if createTx.RowsAffected == 0 {
					tx.Updates(&dbRcd)
				}

				if rr.CloseProjectCategoryName != "" {
					categoryWithCloseProject[dbRcd.ID] = rr.CloseProjectCategoryName
				}
			}

			for origId, closeProjectCategoryName := range categoryWithCloseProject {
				var closeProjectCategory model.ProposalCategory
				if err = tx.Where(&model.ProposalCategory{Name: closeProjectCategoryName}).First(&closeProjectCategory).Error; err != nil {
					return err
				}

				if err = tx.Model(&model.ProposalCategory{}).
					Where("id = ?", origId).
					Updates(&model.ProposalCategory{CategoryIdForCloseProject: closeProjectCategory.ID}).Error; err != nil {
					return err
				}
			}
		}

		if componentsRcds, found := seedDataBucket["proposal_components"]; found {
			for _, r := range componentsRcds {
				var rr proposalComponentRcd
				dataBytes, err := yaml.Marshal(r)
				if err != nil {
					panic(err)
				}
				err = yaml.Unmarshal(dataBytes, &rr)
				if err != nil {
					panic(err)
				}

				dbRcd := model.ProposalComponent{
					Schema:   rr.Schema,
					IsHidden: rr.IsHidden,
				}

				if rr.ApproveAction != "" {
					var approveComponentAction model.ProposalComponentAction
					if err = tx.Where(&model.ProposalComponentAction{Command: rr.ApproveAction}).First(&approveComponentAction).Error; err != nil {
						panic(err)
					}
					dbRcd.ApproveActionId = approveComponentAction.ID
				}

				if rr.RejectAction != "" {
					var rejectComponentAction model.ProposalComponentAction
					if err = tx.Where(&model.ProposalComponentAction{Command: rr.RejectAction}).First(&rejectComponentAction).Error; err != nil {
						panic(err)
					}
					dbRcd.RejectActionId = rejectComponentAction.ID
				}

				createTx := tx.Where(&model.ProposalComponent{Name: rr.Name}).Assign(&dbRcd).FirstOrCreate(&dbRcd)
				if err = createTx.Error; err != nil {
					return err
				} else if createTx.RowsAffected == 0 {
					tx.Updates(&dbRcd)
				}
			}
		}

		if tmplRcds, found := seedDataBucket["proposal_templates"]; found {
			for _, r := range tmplRcds {
				var rr proposalTmplRcd
				dataBytes, err := yaml.Marshal(r)
				if err != nil {
					panic(err)
				}
				err = yaml.Unmarshal(dataBytes, &rr)
				if err != nil {
					panic(err)
				}

				var compList []string
				if rr.ComponentNameList != "" {
					err = json.Unmarshal([]byte(rr.ComponentNameList), &compList)
					if err != nil {
						panic(err)
					}
				}

				var xtraCheckRules []*model.ExtraResultCheckRuleData
				if rr.ExtraResultCheckRule != "" {
					if err = json.Unmarshal([]byte(rr.ExtraResultCheckRule), &xtraCheckRules); err != nil {
						panic(err)
					}
				}

				var proposalCategory model.ProposalCategory
				if err = tx.Where(&model.ProposalCategory{Name: rr.CategoryName}).First(&proposalCategory).Error; err != nil {
					panic(err)
				}

				dbRcd := model.ProposalTemplate{
					Name:                 rr.Name,
					RuleDesc:             rr.RuleDesc,
					VoteType:             rr.VoteType,
					Type:                 model.ProposalTemplateType(rr.Type),
					ContentSchema:        rr.ContentSchema,
					Components:           nil,
					ComponentNameList:    compList,
					ProposalCategoryID:   proposalCategory.ID,
					ExtraResultCheckRule: xtraCheckRules,
					VoteTimeProperties: model.VoteTimeProperties{
						PublicitySecond:        rr.PublicitySecond,
						VoteDurationSecond:     rr.VoteDurationSecond,
						PendingExecutionSecond: rr.PendingExecutionSecond,
					},
					DisplayIndex:     rr.DisplayIndex,
					IsCustomTemplate: rr.IsCustomTemplate,
					IsHidden:         rr.IsHidden,
				}
				createTx := tx.Where(&model.ProposalTemplate{Name: dbRcd.Name}).Assign(&dbRcd).FirstOrCreate(&dbRcd)
				if err = createTx.Error; err != nil {
					return err
				} else if createTx.RowsAffected == 0 {
					tx.Updates(&dbRcd)
				}
			}
		}

		if tmplCompRcds, found := seedDataBucket["template_components"]; found {
			for _, r := range tmplCompRcds {
				var rr templateComponentRcd
				dataBytes, err := yaml.Marshal(r)
				if err != nil {
					panic(err)
				}
				err = yaml.Unmarshal(dataBytes, &rr)
				if err != nil {
					panic(err)
				}

				var componentRcd model.ProposalComponent
				if err = tx.Where(&model.ProposalComponent{Name: rr.ComponentName}).First(&componentRcd).Error; err != nil {
					panic(err)
				}

				var tmplRcd model.ProposalTemplate
				if err = tx.Where(&model.ProposalTemplate{Name: rr.TemplateName}).First(&tmplRcd).Error; err != nil {
					panic(err)
				}

				tmplRcd.Components = append(tmplRcd.Components, &componentRcd)

				if err = tx.Updates(&tmplRcd).Error; err != nil {
					panic(err)
				}
			}
		}

		if sysVarRcds, found := seedDataBucket["system_variables"]; found {
			for _, r := range sysVarRcds {
				var rr sysVarRcd
				dataBytes, err := yaml.Marshal(r)
				if err != nil {
					panic(err)
				}
				err = yaml.Unmarshal(dataBytes, &rr)
				if err != nil {
					panic(err)
				}

				if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.SystemVariable{
					Name:     rr.Name,
					NumValue: rr.NumValue,
					StrValue: rr.StrValue,
				}).Error; err != nil {
					panic(err)
				}
			}
		}

		if prjRcds, found := seedDataBucket["projects"]; found {
			for _, r := range prjRcds {
				var rr prjRcd
				dataBytes, err := yaml.Marshal(r)
				if err != nil {
					panic(err)
				}
				err = yaml.Unmarshal(dataBytes, &rr)
				if err != nil {
					panic(err)
				}

				var sponsors []string
				if rr.Sponsors != "" {
					err = json.Unmarshal([]byte(rr.Sponsors), &sponsors)
					if err != nil {
						panic(err)
					}
				}

				dbPrjRcd := model.Project{
					Name:         rr.Name,
					Desc:         rr.Desc,
					Status:       model.ProjectStatus(rr.Status),
					Sponsors:     sponsors,
					CreateTs:     model.GetCurrentUtcEpochSecond(),
					UpdateTs:     model.GetCurrentUtcEpochSecond(),
					Label:        rr.Label,
					SIP:          rr.Sip,
					Category:     rr.Category,
					ApprovalLink: rr.ApprovalLink,
					OverLink:     rr.OverLink,
					ContantWay:   rr.ContantWay,
					OfficialLink: rr.OfficialLink,
				}
				createTx := tx.Where(&model.Project{Name: rr.Name}).FirstOrCreate(&dbPrjRcd)
				if createTx.Error != nil {
					panic(createTx.Error)
				} else if createTx.RowsAffected == 0 {
					tx.Updates(&dbPrjRcd)
				}
			}
		}

		if guildRcds, found := seedDataBucket["guilds"]; found {
			for _, r := range guildRcds {
				var rr guildRcd
				dataBytes, err := yaml.Marshal(r)
				if err != nil {
					panic(err)
				}
				err = yaml.Unmarshal(dataBytes, &rr)
				if err != nil {
					panic(err)
				}

				var sponsors []string
				if rr.Sponsors != "" {
					err = json.Unmarshal([]byte(rr.Sponsors), &sponsors)
					if err != nil {
						panic(err)
					}
				}

				dbRcd := model.Guild{
					Logo:         rr.Logo,
					Name:         rr.Name,
					Desc:         rr.Desc,
					Status:       model.ProjectStatus(rr.Status),
					Sponsors:     sponsors,
					CreateTs:     model.GetCurrentUtcEpochSecond(),
					UpdateTs:     model.GetCurrentUtcEpochSecond(),
					ContantWay:   rr.ContantWay,
					OfficialLink: rr.OfficialLink,
				}
				createTx := tx.Where(&model.Guild{Name: rr.Name}).FirstOrCreate(&dbRcd)
				if createTx.Error != nil {
					panic(createTx.Error)
				} else if createTx.RowsAffected == 0 {
					tx.Updates(&dbRcd)
				}
			}
		}

		if commonBudgetRcds, found := seedDataBucket["common_budget_sources"]; found {
			for _, r := range commonBudgetRcds {
				var rr commonBudgetSourceRcd
				dataBytes, err := yaml.Marshal(r)
				if err != nil {
					panic(err)
				}
				err = yaml.Unmarshal(dataBytes, &rr)
				if err != nil {
					panic(err)
				}

				dbRcd := model.CommonBudgetSource{
					Name:  rr.Name,
					State: rr.State,
				}
				createTx := tx.Where(&model.CommonBudgetSource{Name: rr.Name}).FirstOrCreate(&dbRcd)
				if createTx.Error != nil {
					panic(createTx.Error)
				} else if createTx.RowsAffected == 0 {
					tx.Updates(&dbRcd)
				}
			}
		}

		return nil
	})
}

func updateEntityCasbinPermission(db *gorm.DB) {
	adapter, err := gormadapter.NewAdapterByDB(db)
	if err != nil {
		panic(err)
	}
	enforcer, err := casbin.NewSyncedEnforcer("rbac_model.conf", adapter)
	if err != nil {
		panic(err)
	}

	// Update project casbin policies
	var projects []*model.Project
	err = db.Model(projects).Find(&projects).Error
	if err != nil {
		panic(err)
	}

	// Update guild casbin policies
	var guilds []*model.Guild
	err = db.Model(guilds).Find(&guilds).Error
	if err != nil {
		panic(err)
	}

	for i := range projects {
		p := projects[i]

		_, err = enforcer.AddPolicies(model.GenerateCasbinPoliciesForProject(p.ID))
		if err != nil {
			panic(err)
		}

		if p.Sponsors != nil && len(p.Sponsors) > 0 {
			_, err = enforcer.AddGroupingPolicies(model.GenerateGroupingPoliciesForProject(p.ID, p.Sponsors, nil))
			if err != nil {
				panic(err)
			}
		}
	}

	for i := range guilds {
		g := guilds[i]
		_, err = enforcer.AddPolicies(model.GenerateCasbinPoliciesForGuild(g.ID))
		if err != nil {
			panic(err)
		}

		if g.Sponsors != nil && len(g.Sponsors) > 0 {
			_, err = enforcer.AddGroupingPolicies(model.GenerateGroupingPoliciesForGuild(g.ID, g.Sponsors))
			if err != nil {
				panic(err)
			}
		}
	}

	if err != nil {
		panic(err)
	}
}

func verifyEntityCasbinPermission(db *gorm.DB) {
	adapter, err := gormadapter.NewAdapterByDB(db)
	if err != nil {
		panic(err)
	}
	enforcer, err := casbin.NewSyncedEnforcer("rbac_model.conf", adapter)
	if err != nil {
		panic(err)
	}

	var projects []*model.Project
	var guilds []*model.Guild

	err = db.Model(projects).Find(&projects).Error
	if err != nil {
		panic(err)
	}

	err = db.Model(guilds).Find(&guilds).Error
	if err != nil {
		panic(err)
	}

	missingPermAccounts := make(map[string]bool)
	for _, prj := range projects {
		for _, sponsor := range prj.Sponsors {
			obj := fmt.Sprintf("%s%d", internal.ObjProjPrefix, prj.ID)
			ok, err := enforcer.Enforce(common.FormatUserWallet(sponsor), obj, internal.ActCreateApplication)
			if err != nil {
				panic(err)
			}

			if !ok {
				missingPermAccounts[fmt.Sprintf("%s|project.%d", common.FormatUserWallet(sponsor), prj.ID)] = true
			}
		}
	}

	for _, guild := range guilds {
		for _, sponsor := range guild.Sponsors {
			obj := fmt.Sprintf("%s%d", internal.ObjGuildPrefix, guild.ID)
			ok, err := enforcer.Enforce(common.FormatUserWallet(sponsor), obj, internal.ActCreateApplication)
			if err != nil {
				panic(err)
			}

			if !ok {
				missingPermAccounts[fmt.Sprintf("%s|guild.%d", common.FormatUserWallet(sponsor), guild.ID)] = true
			}
		}
	}

	if len(missingPermAccounts) > 0 {
		api.PrintStructAsJson(lo.Keys(missingPermAccounts), "Missing permission accounts")
	}
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
		fmt.Println("  seed")
		fmt.Println("  fixperm")
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
	case "fixperm":
		updateEntityCasbinPermission(db)
	case "verifyperm":
		verifyEntityCasbinPermission(db)
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}

}
