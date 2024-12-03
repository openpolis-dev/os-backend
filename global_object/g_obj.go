package global_object

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type GlobalObject struct {
	// GlobalObject is a global object

	Gin *gin.Engine

	Db *gorm.DB
}

var gObjSingle *GlobalObject

func NewGlobalObject(gin *gin.Engine, db *gorm.DB) {
	if gObjSingle == nil {
		gObjSingle = &GlobalObject{
			Gin: gin,
			Db:  db,
		}
	}
}

func GetGlobalObject() *GlobalObject {
	return gObjSingle
}
