package global_object

import (
	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/config"
	"gorm.io/gorm"
)

type GlobalObject struct {
	// GlobalObject is a global object

	Gin *gin.Engine

	Db *gorm.DB

	Cfg *config.Config
}

var gObjSingle *GlobalObject

func NewGlobalObject(gin *gin.Engine, db *gorm.DB, cfg *config.Config) {
	if gObjSingle == nil {
		gObjSingle = &GlobalObject{
			Gin: gin,
			Db:  db,
			Cfg: cfg,
		}
	}
}

func GetGlobalObject() *GlobalObject {
	return gObjSingle
}
