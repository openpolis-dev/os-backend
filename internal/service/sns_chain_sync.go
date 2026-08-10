package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"gorm.io/gorm"
)

const snsNameSuffix = ".seedao"

// SyncSnsChainRegistry pulls wallet ↔ sns_name mappings from spp-indexer and upserts local snapshot table.
func SyncSnsChainRegistry(cfg *config.Config, db *gorm.DB) error {
	indexerClient := sdk.GetIndexerClient()
	if indexerClient == nil {
		return fmt.Errorf("indexer client is not initialized")
	}

	records, err := indexerClient.GetAllSnsRegistryRecords(cfg.SnsChainSync.IndexerDataDbPath, cfg.SnsChainSync.IndexerDataDbQuery)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		log.Warn().Msg("SyncSnsChainRegistry: no records fetched from indexer")
		return nil
	}

	now := time.Now().UTC()
	rows := make([]*model.SnsChainRegistry, 0, len(records))
	for _, record := range records {
		wallet := common.FormatUserWallet(record.Wallet)
		snsName, ok := normalizeSnsRegistryName(record.SnsName)
		if !ok {
			continue
		}
		rows = append(rows, &model.SnsChainRegistry{
			Wallet:   wallet,
			SnsName:  snsName,
			Claimed:  false,
			SyncedAt: now,
		})
	}

	affected, err := model.SnsChainRegistryModel.UpsertBatch(db, rows)
	if err != nil {
		return err
	}

	total, err := model.SnsChainRegistryModel.Count(db)
	if err != nil {
		return err
	}

	log.Info().
		Int("fetched", len(records)).
		Int("upserted", affected).
		Int64("total", total).
		Msg("SyncSnsChainRegistry completed")

	exportPath := strings.TrimSpace(cfg.SnsChainSync.ExportPath)
	if exportPath != "" {
		if err := ExportSnsChainRegistrySQL(db, exportPath); err != nil {
			return fmt.Errorf("export sns registry failed: %w", err)
		}
	}

	return nil
}

// ExportSnsChainRegistrySQL writes PostgreSQL-compatible UPSERT SQL for offline import into seedao-api-server.
func ExportSnsChainRegistrySQL(db *gorm.DB, outputPath string) error {
	rows, err := model.SnsChainRegistryModel.FindAll(db)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}

	content := strings.Builder{}
	content.WriteString("-- seedao-api-server sns_chain_registry import\n")
	content.WriteString("-- generated_at: " + time.Now().UTC().Format(time.RFC3339) + "\n\n")
	content.WriteString("BEGIN;\n\n")

	for _, row := range rows {
		content.WriteString(fmt.Sprintf(
			"INSERT INTO sns_chain_registry (wallet, sns_name, claimed, synced_at, created_at, updated_at)\nVALUES ('%s', '%s', %t, '%s', '%s', '%s')\nON CONFLICT (wallet) DO UPDATE SET sns_name = EXCLUDED.sns_name, synced_at = EXCLUDED.synced_at, updated_at = EXCLUDED.updated_at;\n\n",
			escapeSQL(row.Wallet),
			escapeSQL(row.SnsName),
			row.Claimed,
			row.SyncedAt.UTC().Format(time.RFC3339),
			row.CreatedAt.UTC().Format(time.RFC3339),
			row.UpdatedAt.UTC().Format(time.RFC3339),
		))
	}

	content.WriteString("COMMIT;\n")

	if err := os.WriteFile(outputPath, []byte(content.String()), 0o644); err != nil {
		return err
	}

	log.Info().Int("rows", len(rows)).Str("path", outputPath).Msg("exported sns_chain_registry sql")
	return nil
}

func normalizeSnsRegistryName(raw string) (string, bool) {
	name := strings.TrimSpace(strings.ToLower(raw))
	name = strings.TrimSuffix(name, snsNameSuffix)
	name = strings.TrimSpace(name)
	if len(name) < 3 || len(name) > 20 {
		return "", false
	}
	for _, ch := range name {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' {
			continue
		}
		return "", false
	}
	return name, true
}

func escapeSQL(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}
