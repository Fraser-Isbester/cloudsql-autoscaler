package cloudsql

import (
	"context"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	sqladmin "google.golang.org/api/sqladmin/v1"

	"github.com/fraser-isbester/cloudsql-autoscaler/internal/fakegcp"
	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/config"
)

const testProject = "test-project"

func newInstance(name, tier, edition string) *sqladmin.DatabaseInstance {
	return &sqladmin.DatabaseInstance{
		Name:            name,
		DatabaseVersion: "POSTGRES_16",
		State:           "RUNNABLE",
		Region:          "us-central1",
		GceZone:         "us-central1-a",
		Settings: &sqladmin.Settings{
			Tier:                tier,
			Edition:             edition,
			AvailabilityType:    "ZONAL",
			SettingsVersion:     7,
			BackupConfiguration: &sqladmin.BackupConfiguration{Enabled: true},
			DatabaseFlags:       []*sqladmin.DatabaseFlags{{Name: "max_connections", Value: "500"}},
		},
	}
}

var _ = Describe("Client (against fake Cloud SQL Admin API)", func() {
	var (
		ctx    context.Context
		fake   *fakegcp.SQLAdmin
		client *Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		fake = fakegcp.NewSQLAdmin()
		DeferCleanup(fake.Close)
		var err error
		client, err = NewClient(ctx, testProject, fake.ClientOptions()...)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("GetInstance", func() {
		It("maps the API resource onto InstanceInfo", func() {
			inst := newInstance("db1", "db-n2-standard-4", "ENTERPRISE_PLUS")
			inst.Settings.AvailabilityType = "REGIONAL"
			fake.AddInstance(inst)

			info, err := client.GetInstance(ctx, "db1")

			Expect(err).NotTo(HaveOccurred())
			Expect(info.Name).To(Equal("db1"))
			Expect(info.Project).To(Equal(testProject))
			Expect(info.MachineType).To(Equal("db-n2-standard-4"))
			Expect(info.CurrentCPU).To(Equal(4))
			Expect(info.CurrentMemoryGB).To(Equal(16.0))
			Expect(info.Edition).To(Equal(config.EditionEnterprisePlus))
			Expect(info.HighAvailability).To(BeTrue())
			Expect(info.BackupEnabled).To(BeTrue())
			Expect(info.State).To(Equal("RUNNABLE"))
			Expect(info.Region).To(Equal("us-central1"))
			Expect(info.Zone).To(Equal("us-central1-a"))
		})

		It("treats any non-ENTERPRISE_PLUS edition (including unset) as Enterprise", func() {
			fake.AddInstance(newInstance("db1", "db-n2-standard-4", ""))

			info, err := client.GetInstance(ctx, "db1")

			Expect(err).NotTo(HaveOccurred())
			Expect(info.Edition).To(Equal(config.EditionEnterprise))
		})

		It("rejects an instance whose tier it does not understand", func() {
			fake.AddInstance(newInstance("db1", "db-mystery-9", "ENTERPRISE"))

			_, err := client.GetInstance(ctx, "db1")

			Expect(err).To(MatchError(ContainSubstring("unknown machine type")))
		})

		It("surfaces API errors", func() {
			_, err := client.GetInstance(ctx, "missing")

			Expect(err).To(MatchError(ContainSubstring("failed to get instance missing")))
		})

		It("does not panic when the API omits settings.backupConfiguration", func() {
			Skip("BUG: GetInstance dereferences Settings.BackupConfiguration without a nil check; " +
				"an instance returned without that block panics and aborts the whole analysis cycle")
			inst := newInstance("replica", "db-n2-standard-4", "ENTERPRISE")
			inst.Settings.BackupConfiguration = nil
			fake.AddInstance(inst)

			Expect(func() { _, _ = client.GetInstance(ctx, "replica") }).NotTo(Panic())
		})
	})

	Describe("ListInstances", func() {
		It("skips instances that cannot be described and returns the rest", func() {
			fake.AddInstance(newInstance("good", "db-n2-standard-4", "ENTERPRISE"))
			fake.AddInstance(newInstance("weird", "db-mystery-9", "ENTERPRISE"))

			infos, err := client.ListInstances(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(infos).To(HaveLen(1))
			Expect(infos[0].Name).To(Equal("good"))
		})

		It("returns an error when listing fails", func() {
			fake.FailWith("GET", "/projects/"+testProject+"/instances", http.StatusForbidden)

			_, err := client.ListInstances(ctx)

			Expect(err).To(HaveOccurred())
		})

		It("follows pagination so no instance is silently ignored", func() {
			Skip("BUG: ListInstances reads only the first page of instances.list (NextPageToken is ignored); " +
				"projects with more instances than one page are partially analyzed")
			fake.ListPageSize = 2
			for _, n := range []string{"a", "b", "c", "d", "e"} {
				fake.AddInstance(newInstance(n, "db-n2-standard-4", "ENTERPRISE"))
			}

			infos, err := client.ListInstances(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(infos).To(HaveLen(5))
		})
	})

	Describe("UpdateMachineType", func() {
		BeforeEach(func() {
			fake.AddInstance(newInstance("db1", "db-n2-standard-4", "ENTERPRISE_PLUS"))
		})

		It("changes only the tier and preserves the rest of the settings", func() {
			Expect(client.UpdateMachineType(ctx, "db1", "db-n2-standard-8")).To(Succeed())

			updates := fake.Updates()
			Expect(updates).To(HaveLen(1))
			Expect(updates[0].Instance).To(Equal("db1"))
			body := updates[0].Body
			Expect(body.Settings.Tier).To(Equal("db-n2-standard-8"))
			Expect(body.Settings.SettingsVersion).To(Equal(int64(7)), "settingsVersion guards against lost updates")
			Expect(body.Settings.AvailabilityType).To(Equal("ZONAL"))
			Expect(body.Settings.Edition).To(Equal("ENTERPRISE_PLUS"))
			Expect(body.Settings.DatabaseFlags).To(HaveLen(1))
			Expect(body.Settings.BackupConfiguration.Enabled).To(BeTrue())
		})

		It("returns an error when the long-running operation finishes with an error", func() {
			fake.SetOperationResult(&sqladmin.Operation{
				Status: "DONE",
				Error:  &sqladmin.OperationErrors{Errors: []*sqladmin.OperationError{{Code: "QUOTA", Message: "no"}}},
			})

			err := client.UpdateMachineType(ctx, "db1", "db-n2-standard-8")

			Expect(err).To(MatchError(ContainSubstring("operation failed")))
		})

		It("returns an error when the update is rejected", func() {
			fake.FailWith("PUT", "/instances/db1", http.StatusConflict)

			err := client.UpdateMachineType(ctx, "db1", "db-n2-standard-8")

			Expect(err).To(MatchError(ContainSubstring("failed to update instance machine type")))
		})

		It("does not send an update when the instance cannot be read", func() {
			err := client.UpdateMachineType(ctx, "missing", "db-n2-standard-8")

			Expect(err).To(HaveOccurred())
			Expect(fake.Updates()).To(BeEmpty())
		})

		It("stops waiting on a pending operation when the context is canceled", func() {
			fake.SetOperationResult(&sqladmin.Operation{Status: "RUNNING"})
			cctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
			defer cancel()

			start := time.Now()
			err := client.UpdateMachineType(cctx, "db1", "db-n2-standard-8")

			Expect(err).To(MatchError(context.DeadlineExceeded))
			Expect(time.Since(start)).To(BeNumerically("<", 3*time.Second))
		})
	})

	Describe("GetLastScalingTime", func() {
		ts := func(d time.Duration) string { return time.Now().Add(-d).UTC().Format(time.RFC3339) }

		It("returns the most recent completed UPDATE for the instance", func() {
			fake.AddOperation(&sqladmin.Operation{TargetId: "db1", OperationType: "UPDATE", Status: "RUNNING", InsertTime: ts(time.Minute)})
			fake.AddOperation(&sqladmin.Operation{TargetId: "other", OperationType: "UPDATE", Status: "DONE", InsertTime: ts(2 * time.Minute)})
			fake.AddOperation(&sqladmin.Operation{TargetId: "db1", OperationType: "BACKUP_VOLUME", Status: "DONE", InsertTime: ts(3 * time.Minute)})
			fake.AddOperation(&sqladmin.Operation{TargetId: "db1", OperationType: "UPDATE", Status: "DONE", InsertTime: ts(10 * time.Minute)})
			fake.AddOperation(&sqladmin.Operation{TargetId: "db1", OperationType: "UPDATE", Status: "DONE", InsertTime: ts(5 * time.Hour)})

			last, err := client.GetLastScalingTime(ctx, "db1")

			Expect(err).NotTo(HaveOccurred())
			Expect(time.Since(last)).To(BeNumerically("~", 10*time.Minute, time.Minute))
		})

		It("errors when the instance has no completed updates", func() {
			fake.AddOperation(&sqladmin.Operation{TargetId: "other", OperationType: "UPDATE", Status: "DONE", InsertTime: ts(time.Minute)})

			_, err := client.GetLastScalingTime(ctx, "db1")

			Expect(err).To(HaveOccurred())
		})

		It("finds the instance's last update even when the project has many newer operations", func() {
			Skip("BUG: GetRecentOperations lists the 50 newest operations project-wide and filters client-side " +
				"instead of passing ?instance=; in a busy project a recent scale is missed, which disables " +
				"the Enterprise Plus 30m/3h interval check and can trigger a downtime-causing re-scale")
			for range 60 {
				fake.AddOperation(&sqladmin.Operation{TargetId: "noisy", OperationType: "BACKUP_VOLUME", Status: "DONE", InsertTime: ts(time.Minute)})
			}
			fake.AddOperation(&sqladmin.Operation{TargetId: "db1", OperationType: "UPDATE", Status: "DONE", InsertTime: ts(10 * time.Minute)})

			last, err := client.GetLastScalingTime(ctx, "db1")

			Expect(err).NotTo(HaveOccurred())
			Expect(time.Since(last)).To(BeNumerically("~", 10*time.Minute, time.Minute))
		})
	})
})
