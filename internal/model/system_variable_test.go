package model_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/storage"
)

var _ = Describe("Application", func() {

	// Before each `It` execution, create the table and init project data
	BeforeEach(func() {
		_ = db.Migrator().AutoMigrate(&model.SystemVariable{})
	})

	// After each `It` execution, drop tables
	AfterEach(func() {
		_ = db.Migrator().DropTable(&model.SystemVariable{})
	})

	Describe("Fetch metaforo access token from DB", func() {
		When("no record with metaforo access token created", func() {
			It("should returns error", func() {
				_, err := model.GetMetaforoAccessToken(db)
				Expect(err).ToNot(BeNil())
			})
		})
		When("record with metaforo access token created", func() {
			BeforeEach(func() {
				err := db.Create(&model.SystemVariable{Name: model.MetaforoAccessTokenVariableName, StrValue: "test"}).Error
				Expect(err).To(BeNil())
			})

			It("should returns metaforo access token", func() {
				token, err := model.GetMetaforoAccessToken(db)
				Expect(err).To(BeNil())
				Expect(token).To(Equal("test"))
			})

			It("should store metaforo access token to cache", func() {
				tokenVal, err := storage.GetCachedData(model.MetaforoAccessTokenVariableName)
				Expect(err).To(BeNil())
				Expect(string(tokenVal)).To(Equal("test"))
			})
		})
	})
})
