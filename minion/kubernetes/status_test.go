package kubernetes

import (
	"errors"
	"testing"

	"github.com/kelda/kelda/blueprint"
	"github.com/kelda/kelda/db"
	"github.com/kelda/kelda/minion/kubernetes/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
)

// Test that if the list fails, nothing changes.
func TestStatusFailedList(t *testing.T) {
	t.Parallel()
	conn := db.New()
	podsClient := &mocks.PodInterface{}

	containerToStatus := map[string]string{
		"host1": "foo",
		"host2": "bar",
	}
	conn.Txn(db.ContainerTable).Run(func(view db.Database) error {
		for host, status := range containerToStatus {
			dbc := view.InsertContainer()
			dbc.Hostname = host
			dbc.Status = status
			view.Commit(dbc)
		}
		return nil
	})

	podsClient.On("List", mock.Anything).Return(nil, assert.AnError).Once()
	updateContainerStatuses(conn, podsClient, nil)

	conn.Txn(db.ContainerTable).Run(func(view db.Database) error {
		dbcs := view.SelectFromContainer(nil)
		assert.Len(t, dbcs, len(containerToStatus))
		for _, dbc := range dbcs {
			expStatus, ok := containerToStatus[dbc.Hostname]
			assert.True(t, ok)
			assert.Equal(t, expStatus, dbc.Status)
		}
		return nil
	})
}

func TestStatusContainer(t *testing.T) {
	t.Parallel()

	checkUpdateStatus(t, updateStatusTest{
		containers: []db.Container{
			{Hostname: "host1"},
			{Hostname: "host2", Status: "waiting"},
			{Hostname: "host3", Status: "running"},
			{Hostname: "noinfo", Status: "noinfo"},
		},
		expStatuses: map[string]string{
			"host1":  "waiting: pulling image",
			"host2":  "running",
			"host3":  "terminated: exited",
			"noinfo": "noinfo",
		},
		pods: map[string]corev1.PodStatus{
			"host1": {
				ContainerStatuses: []corev1.ContainerStatus{
					{
						State: corev1.ContainerState{
							Waiting: &corev1.ContainerStateWaiting{
								Reason: "pulling image",
							},
						},
					},
				},
			},
			"host2": {
				ContainerStatuses: []corev1.ContainerStatus{
					{
						State: corev1.ContainerState{
							Running: &corev1.ContainerStateRunning{},
						},
					},
				},
			},
			"host3": {
				ContainerStatuses: []corev1.ContainerStatus{
					{
						State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								Reason: "exited",
							},
						},
					},
				},
			},
			"ignoreme": {
				ContainerStatuses: []corev1.ContainerStatus{
					{
						State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								Reason: "exited",
							},
						},
					},
				},
			},
		},
	})
}

func TestStatusScheduled(t *testing.T) {
	t.Parallel()

	checkUpdateStatus(t, updateStatusTest{
		containers: []db.Container{
			{Hostname: "host1", Status: ""},
			{Hostname: "host2", Status: ""},
		},
		expStatuses: map[string]string{
			"host1": "scheduled",
			"host2": "running",
		},
		pods: map[string]corev1.PodStatus{
			"host1": {
				Conditions: []corev1.PodCondition{
					{
						Status: corev1.ConditionTrue,
						Type:   corev1.PodScheduled,
					},
				},
			},
			// "Running" should supersede "scheduled".
			"host2": {
				Conditions: []corev1.PodCondition{
					{
						Status: corev1.ConditionTrue,
						Type:   corev1.PodScheduled,
					},
				},
				ContainerStatuses: []corev1.ContainerStatus{
					{
						State: corev1.ContainerState{
							Running: &corev1.ContainerStateRunning{},
						},
					},
				},
			},
		},
	})
}

func TestStatusSecret(t *testing.T) {
	t.Parallel()

	checkUpdateStatus(t, updateStatusTest{
		containers: []db.Container{
			{
				Hostname: "host1",
				FilepathToContent: map[string]blueprint.ContainerValue{
					"foo": blueprint.NewSecret("undefined"),
					"bar": blueprint.NewSecret("undefined2"),
					"baz": blueprint.NewSecret("defined"),
				},
			},
			{
				Hostname: "host2",
				FilepathToContent: map[string]blueprint.ContainerValue{
					"foo": blueprint.NewSecret("defined"),
				},
			},
		},
		expStatuses: map[string]string{
			"host1": "Waiting for secrets: [undefined undefined2]",
			"host2": "Secrets ready",
		},
		secrets: map[string]string{
			"defined": "val",
		},
	})
}

func TestStatusCustomImage(t *testing.T) {
	t.Parallel()

	buildingDockerfile := "buildingDockerfile"
	buildingImage := "buildingImage"
	builtDockerfile := "builtDockerfile"
	builtImage := "builtImage"
	checkUpdateStatus(t, updateStatusTest{
		containers: []db.Container{
			{
				Hostname: "host1",
				FilepathToContent: map[string]blueprint.ContainerValue{
					"foo": blueprint.NewSecret("defined"),
				},
				Dockerfile: buildingDockerfile,
				Image:      buildingImage,
			},
			{
				Hostname:   "host2",
				Dockerfile: buildingDockerfile,
				Image:      buildingImage,
			},
			{
				Hostname:   "host3",
				Dockerfile: builtDockerfile,
				Image:      builtImage,
			},
		},
		expStatuses: map[string]string{
			"host1": "building",
			"host2": "building",
			"host3": "built",
		},
		secrets: map[string]string{
			"defined": "val",
		},
		images: []db.Image{
			{Name: buildingImage, Dockerfile: buildingDockerfile, Status: db.Building},
			{Name: builtImage, Dockerfile: builtDockerfile, Status: db.Built},
		},
	})
}

type updateStatusTest struct {
	containers  []db.Container
	images      []db.Image
	pods        map[string]corev1.PodStatus
	secrets     map[string]string
	expStatuses map[string]string
}

func checkUpdateStatus(t *testing.T, test updateStatusTest) {
	conn := db.New()
	conn.Txn(db.ContainerTable, db.ImageTable).Run(func(view db.Database) error {
		for _, dbc := range test.containers {
			inserted := view.InsertContainer()
			dbc.ID = inserted.ID
			view.Commit(dbc)
		}
		for _, dbi := range test.images {
			inserted := view.InsertImage()
			dbi.ID = inserted.ID
			view.Commit(dbi)
		}
		return nil
	})

	podsClient := &mocks.PodInterface{}
	var pods []corev1.Pod
	for host, status := range test.pods {
		pods = append(pods, corev1.Pod{
			Spec: corev1.PodSpec{
				Hostname: host,
			},
			Status: status,
		})
	}
	podsClient.On("List", mock.Anything).
		Return(&corev1.PodList{Items: pods}, nil).Once()

	secretClient := &mocks.SecretClient{}
	testSecretUndefined := func(secretName string) bool {
		if test.secrets == nil {
			return true
		}
		_, exists := test.secrets[secretName]
		return !exists
	}
	secretClient.On("Get", mock.MatchedBy(testSecretUndefined)).
		Return("", errors.New("secret does not exist"))
	for name, val := range test.secrets {
		secretClient.On("Get", name).Return(val, nil)
	}

	updateContainerStatuses(conn, podsClient, secretClient)

	conn.Txn(db.ContainerTable).Run(func(view db.Database) error {
		dbcs := view.SelectFromContainer(nil)
		assert.Len(t, dbcs, len(test.expStatuses))
		for _, dbc := range dbcs {
			expStatus, ok := test.expStatuses[dbc.Hostname]
			assert.True(t, ok)
			assert.Equal(t, expStatus, dbc.Status)
		}
		return nil
	})
}
