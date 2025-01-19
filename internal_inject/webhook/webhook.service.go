package webhook_inject

import (
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WebhookService struct {
	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`
}

func (s *WebhookService) RefreshNodeSbtNum() error {

	computNodeSbt, err := sdk.GetIndexerClient().GetComputeNodeSbt()
	if err != nil {
		return err
	}

	node := &model.SystemVariable{
		Name:     "compute_node_num",
		NumValue: computNodeSbt.Node,
	}
	err = s.Db.Model(&model.SystemVariable{}).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}},
		DoUpdates: clause.AssignmentColumns([]string{"num_value"}),
	}).Create(node).Error
	if err != nil {
		return err
	}

	sbt := &model.SystemVariable{
		Name:     "compute_sbt_num",
		NumValue: computNodeSbt.Node,
	}
	err = s.Db.Model(&model.SystemVariable{}).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}},
		DoUpdates: clause.AssignmentColumns([]string{"num_value"}),
	}).Create(sbt).Error
	if err != nil {
		return err
	}

	return nil
}
