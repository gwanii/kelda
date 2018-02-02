package kubernetes

import (
	"fmt"
	"sort"

	"github.com/kelda/kelda/db"
	"github.com/kelda/kelda/join"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientv1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

func updateContainerStatuses(conn db.Conn, podsClient clientv1.PodInterface,
	secretClient SecretClient) {

	pods, err := podsClient.List(metav1.ListOptions{})
	if err != nil {
		log.WithError(err).Error("Failed to list current pods")
		return
	}

	conn.Txn(db.ImageTable, db.ContainerTable).Run(func(view db.Database) error {
		dbcKey := func(intf interface{}) interface{} {
			return intf.(db.Container).Hostname
		}
		podKey := func(intf interface{}) interface{} {
			return intf.(corev1.Pod).Spec.Hostname
		}
		pairs, noInfoContainers, _ := join.HashJoin(
			db.ContainerSlice(view.SelectFromContainer(nil)),
			podSlice(pods.Items), dbcKey, podKey)

		for _, pair := range pairs {
			dbc := pair.L.(db.Container)
			pod := pair.R.(corev1.Pod)
			if len(pod.Status.ContainerStatuses) == 1 {
				// Get the status of the actual container.
				status := pod.Status.ContainerStatuses[0]
				switch {
				case status.State.Running != nil:
					dbc.Status = "running"
					dbc.Created = status.State.Running.StartedAt.Time
				case status.State.Waiting != nil:
					dbc.Status = "waiting: " + status.State.Waiting.Reason
				case status.State.Terminated != nil:
					dbc.Status = "terminated: " + status.State.Terminated.Reason
				}
			} else {
				// Check if the pod is scheduled.
				for _, status := range pod.Status.Conditions {
					if status.Status == corev1.ConditionTrue &&
						status.Type == corev1.PodScheduled {
						dbc.Status = "scheduled"
					}
				}
			}
			dbc.PodName = pod.GetName()
			dbc.Minion = pod.Status.HostIP
			view.Commit(dbc)
		}

		// For the containers without a matching pod, check if they are waiting
		// on a secret, or if their image has a status.
		for i, intf := range noInfoContainers {
			dbc := intf.(db.Container)
			secrets := dbc.GetReferencedSecrets()
			if len(secrets) == 0 {
				continue
			}

			_, missing := toSecretHashEnvVars(secretClient, secrets)
			if len(missing) != 0 {
				sort.Strings(missing)
				dbc.Status = fmt.Sprintf("Waiting for secrets: %v", missing)
				view.Commit(dbc)
				noInfoContainers = append(noInfoContainers[:i],
					noInfoContainers[i+1:]...)
			} else {
				// Set the container's status to signify that the container
				// isn't waiting on secrets, but don't remove it from the list
				// of noInfoContainers so that if there is an image status, the
				// status will be overwritten.
				dbc.Status = "Secrets ready"
				view.Commit(dbc)
			}
		}

		imageMap := map[db.Image]db.Image{}
		for _, img := range view.SelectFromImage(nil) {
			imageMap[db.Image{
				Name:       img.Name,
				Dockerfile: img.Dockerfile,
			}] = img
		}
		for _, intf := range noInfoContainers {
			dbc := intf.(db.Container)
			if dbc.Dockerfile == "" {
				continue
			}

			img, ok := imageMap[db.Image{
				Name:       dbc.Image,
				Dockerfile: dbc.Dockerfile,
			}]
			if ok {
				dbc.Status = img.Status
				view.Commit(dbc)
			}
		}
		return nil
	})
}

type podSlice []corev1.Pod

func (slc podSlice) Get(ii int) interface{} {
	return slc[ii]
}

func (slc podSlice) Len() int {
	return len(slc)
}
