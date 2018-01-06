package kubernetes

import (
	"crypto/sha1"
	"fmt"
	"sort"

	"github.com/kelda/kelda/blueprint"
	"github.com/kelda/kelda/db"
	"github.com/kelda/kelda/join"

	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appsclient "k8s.io/client-go/kubernetes/typed/apps/v1"
	"k8s.io/client-go/util/retry"
)

var filepathToContentMode = int32(0444)

func updateDeployments(conn db.Conn, deploymentsClient appsclient.DeploymentInterface,
	secretClient SecretClient) {

	currentDeployments, err := deploymentsClient.List(metav1.ListOptions{})
	if err != nil {
		log.WithError(err).Error("Failed to list current deployments")
		return
	}

	key := func(intf interface{}) interface{} {
		return intf.(appsv1.Deployment).Name
	}
	pairs, toCreate, toDelete := join.HashJoin(
		deploymentSlice(makeDesiredDeployments(secretClient, conn)),
		deploymentSlice(currentDeployments.Items),
		key, key)

	for _, pair := range pairs {
		// TODO: Counters throughout.
		// Retry updating the deployment if the apiserver reports that there's
		// a conflict. Conflicts are benign -- for example, there might be a
		// conflict if Kubernetes updated the deployment to change the pod
		// status.
		deployment := pair.L.(appsv1.Deployment)
		err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			_, err := deploymentsClient.Update(&deployment)
			return err
		})
		if err != nil {
			log.WithError(err).WithField("deployment", deployment.Name).
				Error("Failed to update deployment")
		}
	}

	for _, intf := range toCreate {
		deployment := intf.(appsv1.Deployment)
		log.WithField("deployment", deployment.Name).
			Info("Creating deployment")
		if _, err := deploymentsClient.Create(&deployment); err != nil {
			log.WithError(err).WithField("deployment", deployment.Name).
				Error("Failed to create deployment")
		}
	}

	for _, intf := range toDelete {
		deployment := intf.(appsv1.Deployment)
		log.WithField("deployment", deployment.Name).
			Info("Deleting deployment")
		err := deploymentsClient.Delete(deployment.Name, &metav1.DeleteOptions{})
		if err != nil {
			log.WithError(err).WithField("deployment", deployment.Name).
				Error("Failed to delete deployment")
		}
	}
}

const hostnameKey = "hostname"

func makeDesiredDeployments(secretClient SecretClient, conn db.Conn) (
	deployments []appsv1.Deployment) {

	var containers []db.Container
	var pb podBuilder
	conn.Txn(db.ContainerTable, db.ImageTable, db.PlacementTable).Run(func(view db.Database) error {
		containers = view.SelectFromContainer(func(dbc db.Container) bool {
			return dbc.IP != ""
		})
		pb = newPodBuilder(secretClient, view.SelectFromImage(nil), view.SelectFromPlacement(nil))
		return nil
	})

	for _, dbc := range containers {
		pod, ok := pb.makePodForContainer(dbc)
		if !ok {
			continue
		}

		labels := map[string]string{
			hostnameKey: dbc.Hostname,
			"keldaIP":   dbc.IP,
		}
		deployments = append(deployments, appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				// TODO: Kubernetes has pretty strict constraints for its
				// names, such as no underscores, and no uppercase letters.
				// We could hash the name to get around this.
				Name: dbc.Hostname,
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name:   dbc.Hostname,
						Labels: labels,
					},
					Spec: pod,
				},
				Selector: &metav1.LabelSelector{
					MatchLabels: labels,
				},
			},
		})
	}
	return deployments
}

type podBuilder struct {
	customImageMap map[db.Image]db.Image
	idToAffinity   map[string]*corev1.Affinity
	secretClient   SecretClient
}

func newPodBuilder(secretClient SecretClient, images []db.Image, placements []db.Placement) podBuilder {
	imageMap := map[db.Image]db.Image{}
	for _, img := range images {
		imageMap[db.Image{
			Name:       img.Name,
			Dockerfile: img.Dockerfile,
		}] = img
	}

	return podBuilder{imageMap, toAffinities(placements), secretClient}
}

func (pb podBuilder) makePodForContainer(dbc db.Container) (corev1.PodSpec, bool) {
	// If the container isn't built by Kelda, the image doesn't have to be
	// rewritten to the version hosted by the local registry.
	image := dbc.Image
	if dbc.Dockerfile != "" {
		img, ok := pb.customImageMap[db.Image{
			Name:       dbc.Image,
			Dockerfile: dbc.Dockerfile,
		}]
		if !ok || img.Status != db.Built {
			return corev1.PodSpec{}, false
		}
		image = img.RepoDigest
	}

	volumes, volumeMounts := toPodVolumes(dbc.FilepathToContent)
	env, missing := toSecretHashEnvVars(pb.secretClient, dbc.GetReferencedSecrets())
	if len(missing) != 0 {
		return corev1.PodSpec{}, false
	}
	env = append(env, toPodEnvVars(dbc.Env)...)

	// Sort the volumes and volume mounts so that the pod config is
	// consistent. Otherwise, Kubernetes will treat differences in
	// orderings as a reason to restart the pod.
	sort.Sort(volumeMountSlice(volumeMounts))
	sort.Sort(volumeSlice(volumes))
	sort.Sort(envVarSlice(env))

	return corev1.PodSpec{
		Hostname: dbc.Hostname,
		Containers: []corev1.Container{
			{
				Name:         dbc.Hostname,
				Image:        image,
				Env:          env,
				Args:         dbc.Command,
				VolumeMounts: volumeMounts,
			},
		},
		Affinity:  pb.idToAffinity[dbc.Hostname],
		DNSPolicy: corev1.DNSDefault,
		Volumes:   volumes,
	}, true
}

func toSecretHashEnvVars(secretClient SecretClient, secretNames []string) (
	envVars []corev1.EnvVar, missing []string) {
	for _, name := range secretNames {
		val, err := secretClient.Get(name)
		if err != nil {
			missing = append(missing, name)
			continue
		}

		envVars = append(envVars, corev1.EnvVar{
			Name:  "SECRET_HASH_" + name,
			Value: fmt.Sprintf("%x", sha1.Sum([]byte(val))),
		})
	}
	return
}

func toPodEnvVars(dbcEnv map[string]blueprint.ContainerValue) (envVars []corev1.EnvVar) {
	rawStrings, secrets := blueprint.SortContainerValues(dbcEnv)
	for key, val := range rawStrings {
		envVars = append(envVars, corev1.EnvVar{
			Name:  key,
			Value: val,
		})
	}

	for key, secret := range secrets {
		kubeName, subpath := secretRef(secret)
		envVars = append(envVars, corev1.EnvVar{
			Name: key,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: kubeName,
					},
					Key: subpath,
				},
			},
		})
	}
	return envVars
}

func toPodVolumes(filepathToContent map[string]blueprint.ContainerValue) (
	volumes []corev1.Volume, mounts []corev1.VolumeMount) {

	rawStrings, secrets := blueprint.SortContainerValues(filepathToContent)
	mountedSecretVolumes := map[string]struct{}{}
	for path, secret := range secrets {
		kubeName, key := secretRef(secret)
		volumeName := "secret-volume-" + kubeName

		// If there are multiple references to the same secret, only mount its
		// secret volume once to avoid two references to the exact same volume.
		if _, ok := mountedSecretVolumes[volumeName]; !ok {
			volumes = append(volumes, corev1.Volume{
				Name: volumeName,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: kubeName,
					},
				},
			})
			mountedSecretVolumes[volumeName] = struct{}{}
		}

		mounts = append(mounts, corev1.VolumeMount{
			Name:      volumeName,
			MountPath: path,
			ReadOnly:  true,
			SubPath:   key,
		})
	}

	// Mount the raw string values by mounting the ConfigMap corresponding to
	// the filepathToContent.
	const filesVolumeName = "filepath-to-content"
	if len(rawStrings) != 0 {
		volumes = append(volumes, corev1.Volume{
			Name: filesVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: configMapName(rawStrings),
					},
					DefaultMode: &filepathToContentMode,
				},
			},
		})
	}

	for path := range rawStrings {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      filesVolumeName,
			ReadOnly:  true,
			MountPath: path,
			SubPath:   configMapKey(path),
		})
	}
	return
}

type deploymentSlice []appsv1.Deployment

func (slc deploymentSlice) Get(ii int) interface{} {
	return slc[ii]
}

func (slc deploymentSlice) Len() int {
	return len(slc)
}

type volumeMountSlice []corev1.VolumeMount

func (slc volumeMountSlice) Len() int {
	return len(slc)
}

func (slc volumeMountSlice) Swap(i, j int) {
	slc[i], slc[j] = slc[j], slc[i]
}

func (slc volumeMountSlice) Less(i, j int) bool {
	return slc[i].MountPath < slc[j].MountPath
}

type volumeSlice []corev1.Volume

func (slc volumeSlice) Len() int {
	return len(slc)
}

func (slc volumeSlice) Swap(i, j int) {
	slc[i], slc[j] = slc[j], slc[i]
}

func (slc volumeSlice) Less(i, j int) bool {
	return slc[i].Name < slc[j].Name
}

type envVarSlice []corev1.EnvVar

func (slc envVarSlice) Len() int {
	return len(slc)
}

func (slc envVarSlice) Swap(i, j int) {
	slc[i], slc[j] = slc[j], slc[i]
}

func (slc envVarSlice) Less(i, j int) bool {
	return slc[i].Name < slc[j].Name
}
