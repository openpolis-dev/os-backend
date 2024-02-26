package model_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/storage"
)

const (
	TestMfAccessToken = "test_token"
	TestMfGrpId       = "12345"
	TestMfGrpName     = "test_grp"
)

var _ = Describe("SystemVariable", func() {

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
				_, err := model.GetMetaforoData(db)
				Expect(err).ToNot(BeNil())
			})
		})
		When("record with metaforo access token created", func() {
			BeforeEach(func() {
				mfSysVariableRcds := []*model.SystemVariable{
					{Name: internal.SysVarMfAdminToken, StrValue: TestMfAccessToken},
					{Name: internal.SysVarMfGroupId, StrValue: TestMfGrpId},
					{Name: internal.SysVarMfGroupName, StrValue: TestMfGrpName},
				}
				err := db.Create(&mfSysVariableRcds).Error
				Expect(err).To(BeNil())
			})

			It("should returns metaforo access token", func() {
				mfInfo, err := model.GetMetaforoData(db)
				Expect(err).To(BeNil())
				Expect(mfInfo[internal.SysVarMfAdminToken]).To(Equal(TestMfAccessToken))
				Expect(mfInfo[internal.SysVarMfGroupId]).To(Equal(TestMfGrpId))
				Expect(mfInfo[internal.SysVarMfGroupName]).To(Equal(TestMfGrpName))
			})

			It("should store metaforo access token to cache", func() {
				mfInfoBytes, err := storage.GetCachedData(model.MetaforoInfoVariableName)
				Expect(err).To(BeNil())
				var rslt map[string]string
				err = json.Unmarshal(mfInfoBytes, &rslt)
				Expect(err).To(BeNil())
				Expect(rslt[internal.SysVarMfAdminToken]).To(Equal(TestMfAccessToken))
				Expect(rslt[internal.SysVarMfGroupId]).To(Equal(TestMfGrpId))
				Expect(rslt[internal.SysVarMfGroupName]).To(Equal(TestMfGrpName))
			})
		})
	})
})
