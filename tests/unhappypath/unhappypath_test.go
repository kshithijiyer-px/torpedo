package tests

import (
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
	"github.com/portworx/sched-ops/k8s/stork"
	"github.com/portworx/torpedo/drivers/node"
	"github.com/portworx/torpedo/drivers/scheduler"
	"github.com/portworx/torpedo/pkg/kvdbutils"
	"github.com/portworx/torpedo/pkg/snapshotutils"
	"github.com/portworx/torpedo/pkg/testrailuttils"
	. "github.com/portworx/torpedo/tests"
	"github.com/sirupsen/logrus"
)

var (
	storkops = stork.Instance()
)

const (
	dropPercentage      = 20
	delayInMilliseconds = 250
	//24 hours
	totalTimeInHours              = 24
	errorPersistTimeInMinutes     = 60 * time.Minute
	snapshotScheduleRetryInterval = 10 * time.Second
	snapshotScheduleRetryTimeout  = 5 * time.Minute
	waitTimeForPXAfterError       = 20 * time.Minute
)

func TestBasic(t *testing.T) {
	RegisterFailHandler(Fail)

	var specReporters []Reporter
	junitReporter := reporters.NewJUnitReporter("/testresults/junit_basic.xml")
	specReporters = append(specReporters, junitReporter)
	RunSpecsWithDefaultAndCustomReporters(t, "Torpedo : Basic", specReporters)
}

var _ = BeforeSuite(func() {
	InitInstance()
})

// This test is to verify stability of the system when there  is  a network error on the system.
var _ = Describe("{NetworkErrorInjection}", func() {
	var testrailID = 3526435
	injectionType := "drop"
	//TODO need to fix this issue later.
	// testrailID corresponds to: https://portworx.testrail.net/index.php?/cases/view/35264
	var runID int
	JustBeforeEach(func() {
		runID = testrailuttils.AddRunsToMilestone(testrailID)
	})
	var contexts []*scheduler.Context
	It("Inject network error while applications are running", func() {
		contexts = make([]*scheduler.Context, 0)

		for i := 0; i < Inst().GlobalScaleFactor; i++ {
			logrus.Infof("Iteration number %d", i)
			contexts = append(contexts, ScheduleApplications(fmt.Sprintf("applicationscaleup-%d", i))...)
		}
		currentTime := time.Now()
		timeToExecuteTest := time.Now().Local().Add(time.Hour * time.Duration(totalTimeInHours))

		// Set Autofs trim
		currNode := node.GetWorkerNodes()[0]
		err := Inst().V.SetClusterOpts(currNode, map[string]string{
			"--auto-fstrim": "on",
		})
		if err != nil {
			err = fmt.Errorf("error while enabling auto fstrim, Error:%v", err)
			Expect(err).NotTo(HaveOccurred())
		}

		Step("Verify applications after deployment", func() {
			for _, ctx := range contexts {
				ValidateContext(ctx)
			}
		})
		Step("Create snapshot schedule policy", func() {
			createSnapshotSchedule(contexts)
		})

		for int64(timeToExecuteTest.Sub(currentTime).Seconds()) > 0 {
			// TODO core check
			logrus.Infof("Remaining time to test in minutes : %d ", int64(timeToExecuteTest.Sub(currentTime).Seconds()/60))
			Step("Set packet loss on random nodes ", func() {
				//Get all nodes and set eth0
				nodes := node.GetWorkerNodes()
				numberOfNodes := int(math.Ceil(float64(0.40) * float64(len(nodes))))
				selectedNodes := nodes[:numberOfNodes]
				//nodes []Node, errorInjectionType string, operationType string,
				//dropPercentage int, delayInMilliseconds int
				logrus.Infof("Set network error injection")
				Inst().N.InjectNetworkError(selectedNodes, injectionType, "add", dropPercentage, delayInMilliseconds)
				logrus.Infof("Wait %d minutes before checking px status ", errorPersistTimeInMinutes/(time.Minute))
				time.Sleep(errorPersistTimeInMinutes)
				hasPXUp := true
				for _, n := range nodes {
					logrus.Infof("Check PX status on %v", n.Name)
					err := Inst().V.WaitForPxPodsToBeUp(n)
					if err != nil {
						hasPXUp = false
						logrus.Errorf("PX failed to be in ready state  %v %s ", n.Name, err)
					}
				}
				if !hasPXUp {
					Expect(fmt.Errorf("PX is not ready on on or more nodes ")).NotTo(HaveOccurred())
				}
				logrus.Infof("Clear network error injection ")
				Inst().N.InjectNetworkError(selectedNodes, injectionType, "delete", 0, 0)
				//Get kvdb members and
				if injectionType == "drop" {
					injectionType = "delay"
				} else {
					injectionType = "drop"
				}
			})
			logrus.Infof("Wait %d minutes before checking application status ", waitTimeForPXAfterError/(time.Minute))
			time.Sleep(waitTimeForPXAfterError)
			Step("Verify application after clearing error", func() {
				for _, ctx := range contexts {
					ValidateContext(ctx)
				}
			})
			Step("Check KVDB memebers health", func() {
				nodes := node.GetWorkerNodes()
				kvdbMembers, err := Inst().V.GetKvdbMembers(nodes[0])
				if err != nil {
					err = fmt.Errorf("Error getting kvdb members using node %v. cause: %v", nodes[0].Name, err)
					Expect(err).NotTo(HaveOccurred())
				}
				err = kvdbutils.ValidateKVDBMembers(kvdbMembers)
				Expect(err).NotTo(HaveOccurred())
			})
			Step("Check Cloudsnap status ", func() {
				verifyCloudSnaps(contexts)
			})
			Step("Check for crash and verify crash was found before ", func() {
				//TODO need to add this method in future.
			})
			logrus.Infof("Wait  %d minutes before starting next iteration ", errorPersistTimeInMinutes/(time.Minute))
			time.Sleep(errorPersistTimeInMinutes)
			currentTime = time.Now()
		}
		Step("teardown all apps", func() {
			for _, ctx := range contexts {
				TearDownContext(ctx, nil)
			}
		})
	})
	JustAfterEach(func() {
		AfterEachTest(contexts, testrailID, runID)
	})
})

// createSnapshotSchedule creating snapshot schedule
func createSnapshotSchedule(contexts []*scheduler.Context) {
	//Create snapshot schedule
	policyName := "intervalpolicy"
	interval := 30
	for _, ctx := range contexts {
		err := SchedulePolicy(ctx, policyName, interval)
		Expect(err).NotTo(HaveOccurred())
		if strings.Contains(ctx.App.Key, "cloudsnap") {
			appVolumes, err := Inst().S.GetVolumes(ctx)
			if err != nil {
				Expect(err).NotTo(HaveOccurred())
			}
			if len(appVolumes) == 0 {
				err = fmt.Errorf("found no volumes for app %s", ctx.App.Key)
				logrus.Warnf("No appvolumes found")
				Expect(err).NotTo(HaveOccurred())
			}
		}
	}
}

// verifyCloudSnaps check cloudsnaps are taken on scheduled time.
func verifyCloudSnaps(contexts []*scheduler.Context) {
	for _, ctx := range contexts {
		appVolumes, err := Inst().S.GetVolumes(ctx)
		if err != nil {
			logrus.Warnf("Error found while getting volumes %s ", err)
		}
		if len(appVolumes) == 0 {
			err = fmt.Errorf("found no volumes for app %s", ctx.App.Key)
			logrus.Warnf("No appvolumes found")
		}
		//Verify cloudsnap is continuing
		for _, v := range appVolumes {
			if strings.Contains(ctx.App.Key, "cloudsnap") == false {
				logrus.Warningf("Apps are not cloudsnap supported %s ", v.Name)
				continue
			}
			// Skip cloud snapshot trigger for Pure DA volumes
			isPureVol, err := Inst().V.IsPureVolume(v)
			if err != nil {
				logrus.Warnf("No pure volumes found in %s ", ctx.App.Key)
			}
			if isPureVol {
				logrus.Warnf("Cloud snapshot is not supported for Pure DA volumes: [%s]", v.Name)
				continue
			}
			snapshotScheduleName := v.Name + "-interval-schedule"
			logrus.Infof("snapshotScheduleName : %v for volume: %s", snapshotScheduleName, v.Name)
			appNamespace := ctx.App.Key + "-" + ctx.UID
			logrus.Infof("Namespace : %v", appNamespace)

			err = snapshotutils.ValidateSnapshotSchedule(snapshotScheduleName, appNamespace)
			Expect(err).NotTo(HaveOccurred())
		}
	}
}

// SchedulePolicy
func SchedulePolicy(ctx *scheduler.Context, policyName string, interval int) error {
	if strings.Contains(ctx.App.Key, "cloudsnap") {
		logrus.Infof("APP with cloudsnap key available %v ", ctx.App.Key)
		schedPolicy, err := storkops.GetSchedulePolicy(policyName)
		if err == nil {
			logrus.Infof("schedPolicy is %v already exists", schedPolicy.Name)
		} else {
			err = snapshotutils.SchedulePolicyInDefaultNamespace(policyName, interval, 2)
			Expect(err).NotTo(HaveOccurred())
		}
		logrus.Infof("Waiting for 10 mins for Snapshots to be completed")
		time.Sleep(10 * time.Minute)
	}
	return nil
}

var _ = AfterSuite(func() {
	PerformSystemCheck()
	ValidateCleanup()
})

func TestMain(m *testing.M) {
	// call flag.Parse() here if TestMain uses flags
	ParseFlags()
	os.Exit(m.Run())
}