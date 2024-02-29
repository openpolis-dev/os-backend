package task_manager_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/task_manager"
)

var _ = Describe("Internal/TaskManager/MetaforoTask", func() {
	BeforeEach(func() {
		_ = db.AutoMigrate(tables...)
	})

	// After each `It` execution, drop tables
	AfterEach(func() {
		_ = db.Migrator().DropTable(tables...)
	})

	Describe("Refresh metaforo admin token and save to DB", func() {
		When("no record with metaforo admin pk created", func() {
			It("should not update metaforo_admin_token field", func() {
				_, err := model.GetMetaforoData(db)
				Expect(err).NotTo(BeNil())
				task_manager.RefreshMetaforoAdminToken()
				_, err = model.GetMetaforoData(db)
				Expect(err).NotTo(BeNil())
			})
		})

		When("correct admin pk and address set", func() {
			BeforeEach(func() {
				db.Model(&model.SystemVariable{}).Create(
					&model.SystemVariable{
						Name:     internal.SysVarMfAdminWalletAddr,
						StrValue: "0x70997970C51812dc3A010C7d01b50e0d17dc79C8",
					},
				)
				db.Model(&model.SystemVariable{}).Create(
					&model.SystemVariable{
						Name:     internal.SysVarMfAdminWalletPk,
						StrValue: "59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d",
					},
				)
				db.Model(&model.SystemVariable{}).Create(
					&model.SystemVariable{
						Name:     internal.SysVarMfAdminToken,
						StrValue: "",
					},
				)
				db.Model(&model.SystemVariable{}).Create(
					&model.SystemVariable{
						Name:     internal.SysVarSeeAuthPk,
						StrValue: "59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d",
					},
				)
			})

			It("should update metaforo_admin_token field and save to DB", func() {
				mfData, err := model.GetMetaforoData(db)
				Expect(err).To(BeNil())
				Expect(mfData[internal.SysVarMfAdminToken]).To(BeEmpty())

				task_manager.RefreshMetaforoAdminToken()

				mfData, err = model.GetMetaforoData(db)
				Expect(err).To(BeNil())
				Expect(mfData[internal.SysVarMfAdminToken]).NotTo(BeEmpty())
			})
		})
	})
})
