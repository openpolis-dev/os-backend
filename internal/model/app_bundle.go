package model

import (
	"time"

	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

type AppBundle struct {
	ID uint `gorm:"primaryKey"`

	// Applications belongs to this bundle
	AppRecords []*Application `gorm:"foreignKey:BundleId;references:ID"`

	Comment string

	// Submitter of this bundle
	Submitter string

	// Entity means this app bundle's refer, which maybe project or guild.
	// And the field EntityId is the db record ID for Project or Guild table
	EntityType string `gorm:"index"`
	EntityId   uint   `gorm:"index"`

	// Season information of application
	SeasonId uint `gorm:"index"`
	Season   Season

	State ApplicationState `gorm:"inidex"`

	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

// AppBundleAuditLog saves audit log for app bundles, the data structure is similar with ApplicationAuditLog.
type AppBundleAuditLog struct {
	ID uint `gorm:"primaryKey"`

	AppBundleId uint
	AppBundle   AppBundle

	LogTs time.Time
	// Which operation this log record, which should be in new/approve/reject/process/complete
	Operation AuditActionType `json:"operation"`

	// Who perform this operation
	Operator string `json:"operator"`

	// Application state before this operation
	PreState ApplicationState `json:"pre_state"`

	// Application state after this operation
	PostState ApplicationState `json:"post_state"`

	// ExtraData saves some additional data for the operation, e.g. reject reason
	ExtraData string `json:"extra_data"`
}

func ListAppBundles(db *gorm.DB, state string, page *gormfind.Page) (rcds []*AppBundle, total int64, err error) {
	querySeg := db.Model(&AppBundle{})
	if state != "" {
		querySeg = querySeg.Where("state = ?", state)
	}

	total, err = gormfind.Count(querySeg)
	if err != nil {
		return
	}

	rcds, err = gormfind.Rows[AppBundle](querySeg, page)
	if err != nil {
		return
	}

	return
}
