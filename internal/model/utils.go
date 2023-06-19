package model

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/samber/lo"
	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

func GenerateFrontendApplicationRecords(db *gorm.DB, queryParams *ListApplicationQueryParams) ([]*FrontendApplicationRecord, int64, error) {
	if !lo.Contains([]string{"close_project", "new_reward"}, strings.ToLower(strings.TrimSpace(queryParams.Type))) {
		return nil, 0, fmt.Errorf("unknown application type %s", queryParams.Type)
	}

	if !lo.Contains([]string{"project", "guild"}, strings.ToLower(strings.TrimSpace(queryParams.Entity))) {
		return nil, 0, fmt.Errorf("unknown entity type %s", queryParams.Entity)
	}

	appType := MustParseApplicationType(queryParams.Type)
	querySeg := db.Preload(queryParams.Entity).Model(&Application{}).Where("type = ?", appType)

	if len(strings.TrimSpace(queryParams.EntityId)) != 0 {
		entityId, err := strconv.Atoi(queryParams.EntityId)
		if err != nil {
			return nil, 0, err
		}
		querySeg = querySeg.Where(fmt.Sprintf("`%s.id = ?`", queryParams.Entity), entityId)
	}

	fmt.Printf("query seg: %+v\n", querySeg)

	gormFindPage := gormfind.Page{
		Page:      queryParams.Page,
		Size:      queryParams.Size,
		SortField: &queryParams.SortField,
		Order:     &queryParams.SortOrder,
	}

	total, err := gormfind.Count(querySeg)
	if err != nil {
		return nil, 0, err
	}

	rcds, err := gormfind.RowsJoin[FrontendApplicationRecord](querySeg, "application", &gormFindPage)
	if err != nil {
		return nil, 0, err
	}

	return rcds, total, nil
}
