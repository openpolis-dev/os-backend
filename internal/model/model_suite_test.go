package model_test

import (
	"testing"

	"github.com/glebarez/sqlite"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

var db *gorm.DB

var _ = BeforeSuite(func() {
	db, _ = gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
})

func TestModel(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Model Suite")
}
