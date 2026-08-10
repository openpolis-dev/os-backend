package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/storage"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type proposalV2Row struct {
	ID       uint   `gorm:"column:id"`
	LegacyID uint   `gorm:"column:legacy_id"`
	Title    string `gorm:"column:title"`
}

func (proposalV2Row) TableName() string { return "proposal_v2" }

type contentBlock struct {
	ID            uint            `json:"id,omitempty"`
	Title         string          `json:"title,omitempty"`
	Content       string          `json:"content,omitempty"`
	Type          string          `json:"type,omitempty"`
	ComponentList string          `json:"component_list,omitempty"`
	Component     string          `json:"component,omitempty"`
	Data          json.RawMessage `json:"data,omitempty"`
	IsAutoGen     bool            `json:"is_auto_generated,omitempty"`
}

type proposalContent struct {
	Blocks    []contentBlock `json:"blocks"`
	PlainText string         `json:"plain_text"`
}

func main() {
	cfgPath := flag.String("config", "config.yml", "config file path")
	dsnOverride := flag.String("dsn", "", "override dataSource.dsn from config")
	dbSchema := flag.String("db", "postgres", "database driver: postgres or mysql")
	step := flag.String("step", "content", "migration step (only content supported)")
	dryRun := flag.Bool("dry-run", false, "build content but do not write to DB")
	flag.Parse()

	if *step != "content" {
		log.Fatal().Str("step", *step).Msg("unsupported step, use -step content")
	}

	dsn := strings.TrimSpace(*dsnOverride)
	if dsn == "" {
		cfg := config.LoadConfig(*cfgPath)
		dsn = cfg.DataSource.Dsn
		if *dbSchema == "postgres" && cfg.Casbin.DriverName != "" {
			*dbSchema = cfg.Casbin.DriverName
		}
	}
	if dsn == "" {
		log.Fatal().Msg("empty DSN: set -dsn or config dataSource.dsn")
	}

	storage.InitGormDBWithLoggerLevel(dsn, *dbSchema, logger.Warn)
	db := storage.GetGormDB()

	updated, skipped, err := backfillProposalContent(db, *dryRun)
	if err != nil {
		log.Fatal().Err(err).Msg("backfill proposal content failed")
	}
	log.Info().
		Int("updated", updated).
		Int("skipped", skipped).
		Bool("dry_run", *dryRun).
		Msg("migrate_proposal_v2 content step done")
}

func backfillProposalContent(db *gorm.DB, dryRun bool) (updated, skipped int, err error) {
	var rows []proposalV2Row
	if err = db.Where("legacy_id IS NOT NULL AND legacy_id > 0").Order("id").Find(&rows).Error; err != nil {
		return 0, 0, err
	}

	componentNames, err := loadComponentNameMap(db)
	if err != nil {
		return 0, 0, err
	}

	for _, row := range rows {
		content, err := buildContentForProposal(db, row.LegacyID, row.Title, componentNames)
		if err != nil {
			return updated, skipped, fmt.Errorf("proposal_v2 id=%d legacy_id=%d: %w", row.ID, row.LegacyID, err)
		}
		if len(content.Blocks) == 0 && content.PlainText == "" {
			skipped++
			continue
		}

		payload, err := json.Marshal(content)
		if err != nil {
			return updated, skipped, err
		}

		if dryRun {
			log.Info().Uint("proposal_v2_id", row.ID).Uint("legacy_id", row.LegacyID).Int("blocks", len(content.Blocks)).Msg("dry-run")
			updated++
			continue
		}

		if err = db.Table("proposal_v2").Where("id = ?", row.ID).
			Update("content", datatypes.JSON(payload)).Error; err != nil {
			return updated, skipped, fmt.Errorf("update proposal_v2 id=%d: %w", row.ID, err)
		}
		updated++
	}
	return updated, skipped, nil
}

func loadComponentNameMap(db *gorm.DB) (map[uint]string, error) {
	var components []model.ProposalComponent
	if err := db.Find(&components).Error; err != nil {
		return nil, err
	}
	out := make(map[uint]string, len(components))
	for _, c := range components {
		out[c.ID] = c.Name
	}
	return out, nil
}

func buildContentForProposal(db *gorm.DB, legacyProposalID uint, title string, componentNames map[uint]string) (*proposalContent, error) {
	var textBlocks []model.ProposalContentBlock
	if err := db.Where("proposal_id = ?", legacyProposalID).Order("id").Find(&textBlocks).Error; err != nil {
		return nil, err
	}

	var compRecords []model.ProposalComponentRecord
	if err := db.Where("proposal_id = ? AND component_id <> 0", legacyProposalID).Order("id").Find(&compRecords).Error; err != nil {
		return nil, err
	}

	blocks := make([]contentBlock, 0, len(textBlocks)+len(compRecords))
	var plainParts []string

	for _, b := range textBlocks {
		blocks = append(blocks, contentBlock{
			ID:            b.ID,
			Title:         b.Title,
			Content:       b.Content,
			Type:          b.Type,
			ComponentList: b.ComponentList,
		})
		if part := formatPlainSection(b.Title, b.Content); part != "" {
			plainParts = append(plainParts, part)
		}
	}

	for _, cr := range compRecords {
		name := componentNames[cr.ComponentID]
		var data json.RawMessage
		if strings.TrimSpace(cr.Data) != "" {
			if !json.Valid([]byte(cr.Data)) {
				data = json.RawMessage(fmt.Sprintf("%q", cr.Data))
			} else {
				data = json.RawMessage(cr.Data)
			}
		} else {
			data = json.RawMessage("null")
		}
		blocks = append(blocks, contentBlock{
			ID:        cr.ID,
			Type:      "component",
			Component: name,
			Data:      data,
			IsAutoGen: cr.IsAutoGenerated,
		})
		if name != "" {
			plainParts = append(plainParts, fmt.Sprintf("[%s]", name))
		}
	}

	plainText := strings.Join(plainParts, "\n\n")
	if plainText == "" {
		plainText = title
	}

	return &proposalContent{Blocks: blocks, PlainText: plainText}, nil
}

func formatPlainSection(title, content string) string {
	title = strings.TrimSpace(title)
	content = strings.TrimSpace(content)
	switch {
	case title != "" && content != "":
		return title + "\n" + content
	case title != "":
		return title
	default:
		return content
	}
}
