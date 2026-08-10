package sdk

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/rs/zerolog/log"
)

type SnsRegistryRecord struct {
	Wallet  string
	SnsName string
}

type indexerSnsRecord struct {
	ID      int    `json:"id"`
	TokenID string `json:"tokenId"`
	Sns     string `json:"sns"`
	Owner   string `json:"owner"`
	Wallet  string `json:"wallet"`
	SnsName string `json:"sns_name"`
	Name    string `json:"name"`
}

func (r indexerSnsRecord) wallet() string {
	if w := strings.TrimSpace(r.Owner); w != "" {
		return w
	}
	return strings.TrimSpace(r.Wallet)
}

func (r indexerSnsRecord) snsName() string {
	if n := strings.TrimSpace(r.Sns); n != "" {
		return n
	}
	if n := strings.TrimSpace(r.SnsName); n != "" {
		return n
	}
	return strings.TrimSpace(r.Name)
}

// GetAllSnsRegistryRecords loads wallet ↔ sns_name mappings from spp-indexer.
// It tries GET /sns/all first, then optional SQLite fallback (indexer data.db).
func (c *IndexerClient) GetAllSnsRegistryRecords(indexerDataDbPath, indexerDataDbQuery string) ([]SnsRegistryRecord, error) {
	records, err := c.fetchSnsRegistryFromIndexerAPI()
	if err != nil {
		return nil, err
	}
	if len(records) > 0 {
		return records, nil
	}

	if strings.TrimSpace(indexerDataDbPath) == "" {
		log.Warn().Msg("indexer /sns/all returned no records and indexer sqlite path is not configured")
		return nil, nil
	}

	return loadSnsRegistryFromSQLite(indexerDataDbPath, indexerDataDbQuery)
}

func (c *IndexerClient) fetchSnsRegistryFromIndexerAPI() ([]SnsRegistryRecord, error) {
	endpoint := fmt.Sprintf("%s/sns/all", c.ApiBase)
	log.Debug().Msgf("fetch SNS registry from indexer, endpoint=%s", endpoint)

	resp, err := c.httpClient.Get(endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("indexer /sns/all failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	return parseIndexerSnsRegistryPayload(body)
}

func parseIndexerSnsRegistryPayload(body []byte) ([]SnsRegistryRecord, error) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" || trimmed == "{}" || trimmed == "null" {
		return nil, nil
	}

	var asArray []indexerSnsRecord
	if err := json.Unmarshal(body, &asArray); err == nil {
		return normalizeIndexerSnsRecords(asArray), nil
	}

	var wrapped struct {
		Data    []indexerSnsRecord          `json:"data"`
		Records []indexerSnsRecord          `json:"records"`
		Items   []indexerSnsRecord          `json:"items"`
		List    []indexerSnsRecord          `json:"list"`
		Rows    []indexerSnsRecord          `json:"rows"`
		Map     map[string]indexerSnsRecord `json:"-"`
	}
	if err := json.Unmarshal(body, &wrapped); err == nil {
		for _, candidate := range [][]indexerSnsRecord{wrapped.Data, wrapped.Records, wrapped.Items, wrapped.List, wrapped.Rows} {
			if len(candidate) > 0 {
				return normalizeIndexerSnsRecords(candidate), nil
			}
		}
	}

	var asMap map[string]indexerSnsRecord
	if err := json.Unmarshal(body, &asMap); err == nil && len(asMap) > 0 {
		rows := make([]indexerSnsRecord, 0, len(asMap))
		for _, row := range asMap {
			rows = append(rows, row)
		}
		return normalizeIndexerSnsRecords(rows), nil
	}

	var asObject map[string]json.RawMessage
	if err := json.Unmarshal(body, &asObject); err == nil {
		for _, value := range asObject {
			var rows []indexerSnsRecord
			if err := json.Unmarshal(value, &rows); err == nil && len(rows) > 0 {
				return normalizeIndexerSnsRecords(rows), nil
			}
		}
	}

	return nil, fmt.Errorf("unsupported /sns/all payload: %s", trimmed)
}

func normalizeIndexerSnsRecords(rows []indexerSnsRecord) []SnsRegistryRecord {
	out := make([]SnsRegistryRecord, 0, len(rows))
	seenWallet := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		wallet := strings.ToLower(strings.TrimSpace(row.wallet()))
		snsName := strings.TrimSpace(row.snsName())
		if wallet == "" || snsName == "" {
			continue
		}
		if _, ok := seenWallet[wallet]; ok {
			continue
		}
		seenWallet[wallet] = struct{}{}
		out = append(out, SnsRegistryRecord{Wallet: wallet, SnsName: snsName})
	}
	return out
}

func loadSnsRegistryFromSQLite(dbPath, query string) ([]SnsRegistryRecord, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	if strings.TrimSpace(query) == "" {
		query, err = discoverSnsRegistryQuery(db)
		if err != nil {
			return nil, err
		}
	}

	log.Debug().Msgf("load SNS registry from sqlite: path=%s query=%s", dbPath, query)

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	walletIdx, snsIdx := findWalletAndSnsColumnIndex(cols)
	if walletIdx < 0 || snsIdx < 0 {
		return nil, fmt.Errorf("sqlite query must return wallet and sns columns, got: %v", cols)
	}

	out := make([]SnsRegistryRecord, 0)
	seenWallet := make(map[string]struct{})
	for rows.Next() {
		values := make([]any, len(cols))
		scanTargets := make([]any, len(cols))
		for i := range values {
			scanTargets[i] = &values[i]
		}
		if err := rows.Scan(scanTargets...); err != nil {
			return nil, err
		}

		wallet := strings.ToLower(strings.TrimSpace(fmt.Sprint(values[walletIdx])))
		snsName := strings.TrimSpace(fmt.Sprint(values[snsIdx]))
		if wallet == "" || snsName == "" || wallet == "<nil>" || snsName == "<nil>" {
			continue
		}
		if _, ok := seenWallet[wallet]; ok {
			continue
		}
		seenWallet[wallet] = struct{}{}
		out = append(out, SnsRegistryRecord{Wallet: wallet, SnsName: snsName})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func discoverSnsRegistryQuery(db *sql.DB) (string, error) {
	tableRows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return "", err
	}
	defer tableRows.Close()

	var tables []string
	for tableRows.Next() {
		var name string
		if err := tableRows.Scan(&name); err != nil {
			return "", err
		}
		tables = append(tables, name)
	}
	if err := tableRows.Err(); err != nil {
		return "", err
	}

	for _, table := range tables {
		colRows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%q)", table))
		if err != nil {
			continue
		}

		var cols []string
		for colRows.Next() {
			var cid int
			var name, colType string
			var notNull, pk int
			var dflt sql.NullString
			if err := colRows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
				colRows.Close()
				break
			}
			cols = append(cols, name)
		}
		colRows.Close()

		walletCol := pickColumn(cols, "owner", "wallet", "holder", "address")
		snsCol := pickColumn(cols, "sns", "sns_name", "name", "label")
		if walletCol == "" || snsCol == "" {
			continue
		}

		return fmt.Sprintf(
			`SELECT %q AS wallet, %q AS sns_name FROM %q WHERE %q IS NOT NULL AND %q != '' AND %q IS NOT NULL AND %q != ''`,
			walletCol, snsCol, table, walletCol, walletCol, snsCol, snsCol,
		), nil
	}

	return "", fmt.Errorf("unable to discover sns registry table in sqlite db")
}

func pickColumn(cols []string, candidates ...string) string {
	lowerCols := make(map[string]string, len(cols))
	for _, col := range cols {
		lowerCols[strings.ToLower(col)] = col
	}
	for _, candidate := range candidates {
		if col, ok := lowerCols[candidate]; ok {
			return col
		}
	}
	return ""
}

func findWalletAndSnsColumnIndex(cols []string) (walletIdx, snsIdx int) {
	walletIdx, snsIdx = -1, -1
	for i, col := range cols {
		switch strings.ToLower(col) {
		case "wallet", "owner", "holder", "address":
			walletIdx = i
		case "sns", "sns_name", "name", "label":
			snsIdx = i
		}
	}
	return walletIdx, snsIdx
}
