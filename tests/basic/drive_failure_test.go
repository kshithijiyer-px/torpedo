package tests

import (
	"fmt"
	"time"

	"github.com/portworx/torpedo/pkg/testrailuttils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/portworx/torpedo/drivers/node"
	"github.com/portworx/torpedo/drivers/scheduler"
	. "github.com/portworx/torpedo/tests"
)

const (
	dfDefaultTimeout       = 1 * time.Minute
	driveFailTimeout       = 2 * time.Minute
	dfDefaultRetryInterval = 5 * time.Second
)

var _ = Describe("{DriveFailure}", func() {
	var testrailID = 35265
	// testrailID corresponds to: https://portworx.testrail.net/index.php?/cases/view/35265
	var runID int
	JustBeforeEach(func() {
		runID = testrailuttils.AddRunsToMilestone(testrailID)
	})
	var contexts []*scheduler.Context

	testName := "drivefailure"
	It("has to schedule apps and induce a drive failure on one of the nodes", func() {
		var err error
		contexts = make([]*scheduler.Context, 0)

		for i := 0; i < Inst().GlobalScaleFactor; i++ {
			contexts = append(contexts, ScheduleApplications(fmt.Sprintf("%s-%d", testName, i))...)
		}

		ValidateApplications(contexts)

		Step("get nodes for all apps in test and induce drive failure on one of the nodes", func() {
			for _, ctx := range contexts {
				var (
					drives        []string
					appNodes      []node.Node
					nodeWithDrive node.Node
				)

				Step(fmt.Sprintf("get nodes where %s app is running", ctx.App.Key), func() {
					appNodes, err = Inst().S.GetNodesForApp(ctx)
					Expect(err).NotTo(HaveOccurred())
					Expect(appNodes).NotTo(BeEmpty())
					nodeWithDrive = appNodes[0]
				})

				Step(fmt.Sprintf("get drive from node %v", nodeWithDrive), func() {
					drives, err = Inst().V.GetStorageDevices(nodeWithDrive)
					Expect(err).NotTo(HaveOccurred())
					Expect(drives).NotTo(BeEmpty())
				})

				busInfoMap := make(map[string]string)
				Step(fmt.Sprintf("induce a failure on all drives on the node %v", nodeWithDrive), func() {
					for _, driveToFail := range drives {
						busID, err := Inst().N.YankDrive(nodeWithDrive, driveToFail, node.ConnectionOpts{
							Timeout:         dfDefaultTimeout,
							TimeBeforeRetry: dfDefaultRetryInterval,
						})
						busInfoMap[driveToFail] = busID
						Expect(err).NotTo(HaveOccurred())
					}
					Step("wait for the drives to fail", func() {
						time.Sleep(30 * time.Second)
					})

					Step(fmt.Sprintf("check if apps are running"), func() {
						ValidateContext(ctx)
					})

				})

				Step(fmt.Sprintf("recover all drives and the storage driver"), func() {
					for _, driveToFail := range drives {
						err = Inst().N.RecoverDrive(nodeWithDrive, driveToFail, busInfoMap[driveToFail], node.ConnectionOpts{
							Timeout:         driveFailTimeout,
							TimeBeforeRetry: dfDefaultRetryInterval,
						})
						Expect(err).NotTo(HaveOccurred())
					}
					Step("wait for the drives to recover", func() {
						time.Sleep(30 * time.Second)
					})

					err = Inst().V.RecoverDriver(nodeWithDrive)
					Expect(err).NotTo(HaveOccurred())
				})

				Step(fmt.Sprintf("check if volume driver is up"), func() {
					err = Inst().V.WaitDriverUpOnNode(nodeWithDrive, Inst().DriverStartTimeout)
					Expect(err).NotTo(HaveOccurred())
				})
			}
		})

		ValidateAndDestroy(contexts, nil)
	})
	JustAfterEach(func() {
		AfterEachTest(contexts, testrailID, runID)
	})
})