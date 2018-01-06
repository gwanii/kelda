package kubernetes

import (
	"testing"

	"github.com/kelda/kelda/blueprint"
	"github.com/kelda/kelda/db"
	"github.com/kelda/kelda/minion/kubernetes/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestUpdateDeployments(t *testing.T) {
	t.Parallel()
	conn := db.New()
	deploymentsClient := &mocks.DeploymentInterface{}

	// No actions should be taken if we were unable to list the current
	// deployments.
	deploymentsClient.On("List", mock.Anything).Return(nil, assert.AnError).Once()
	updateDeployments(conn, deploymentsClient, nil)
	deploymentsClient.AssertExpectations(t)

	// Test creating a deployment.
	labels := map[string]string{
		hostnameKey: "hostname",
		"keldaIP":   "ip",
	}
	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "hostname",
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "hostname",
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Hostname: "hostname",
					Containers: []corev1.Container{
						{
							Name:  "hostname",
							Image: "image",
						},
					},
					DNSPolicy: corev1.DNSDefault,
				},
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
		},
	}
	conn.Txn(db.ContainerTable).Run(func(view db.Database) error {
		dbc := view.InsertContainer()
		dbc.Hostname = "hostname"
		dbc.Image = "image"
		dbc.IP = "ip"
		view.Commit(dbc)
		return nil
	})
	deploymentsClient.On("List", mock.Anything).Return(
		&appsv1.DeploymentList{}, nil).Once()
	deploymentsClient.On("Create", &deployment).Return(nil, nil).Once()
	updateDeployments(conn, deploymentsClient, nil)
	deploymentsClient.AssertExpectations(t)

	// When the deployment already exists, it should be updated.
	conn.Txn(db.ContainerTable).Run(func(view db.Database) error {
		dbc := view.SelectFromContainer(nil)[0]
		dbc.Env = map[string]blueprint.ContainerValue{
			"key": blueprint.NewString("value"),
		}
		view.Commit(dbc)
		return nil
	})
	changedDeployment := deployment
	changedDeployment.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
		{Name: "key", Value: "value"},
	}
	deploymentsClient.On("List", mock.Anything).Return(
		&appsv1.DeploymentList{
			Items: []appsv1.Deployment{deployment},
		}, nil).Once()
	deploymentsClient.On("Update", &changedDeployment).Return(nil, nil).Once()
	updateDeployments(conn, deploymentsClient, nil)
	deploymentsClient.AssertExpectations(t)

	// When a container is removed, its deployment should be removed.
	conn.Txn(db.ContainerTable).Run(func(view db.Database) error {
		view.Remove(view.SelectFromContainer(nil)[0])
		return nil
	})
	deploymentsClient.On("List", mock.Anything).Return(
		&appsv1.DeploymentList{
			Items: []appsv1.Deployment{changedDeployment},
		}, nil).Once()
	deploymentsClient.On("Delete", changedDeployment.Name, mock.Anything).
		Return(nil, nil).Once()
	updateDeployments(conn, deploymentsClient, nil)
	deploymentsClient.AssertExpectations(t)
}

// The pod spec should be exactly the same everytime it's built. Otherwise,
// Kubernetes would think we're creating a different pod, and destroy the
// old one.
func TestPodBuilderConsistent(t *testing.T) {
	t.Parallel()

	envSecretNameA := "envSecretValueA"
	envSecretNameB := "envSecretValueB"
	fileSecretNameA := "fileSecretValueA"
	fileSecretNameB := "fileSecretValueB"
	sharedSecretName := "sharedSecretValue"

	secretClient := &mocks.SecretClient{}
	secretClient.On("Get", envSecretNameA).Return("envSecretValueA", nil)
	secretClient.On("Get", envSecretNameB).Return("envSecretValueB", nil)
	secretClient.On("Get", fileSecretNameA).Return("fileSecretValueA", nil)
	secretClient.On("Get", fileSecretNameB).Return("fileSecretValueB", nil)
	secretClient.On("Get", sharedSecretName).Return("sharedSecretValue", nil)
	pb := newPodBuilder(secretClient, nil, nil)

	dbc := db.Container{
		Hostname: "hostname",
		Image:    "image",
		FilepathToContent: map[string]blueprint.ContainerValue{
			"a": blueprint.NewString("1"),
			"b": blueprint.NewString("2"),
			"c": blueprint.NewSecret(fileSecretNameA),
			"d": blueprint.NewSecret(fileSecretNameB),
			"e": blueprint.NewSecret(sharedSecretName),
		},
		Env: map[string]blueprint.ContainerValue{
			"a": blueprint.NewString("1"),
			"b": blueprint.NewString("2"),
			"c": blueprint.NewSecret(envSecretNameA),
			"d": blueprint.NewSecret(envSecretNameB),
			"e": blueprint.NewSecret(sharedSecretName),
		},
	}
	pod, ok := pb.makePodForContainer(dbc)
	assert.True(t, ok)
	for i := 0; i < 10; i++ {
		newPod, ok := pb.makePodForContainer(dbc)
		assert.True(t, ok)
		assert.Equal(t, pod, newPod)
	}
}

func TestPodBuilderConfigMap(t *testing.T) {
	t.Parallel()

	fileMap := map[string]string{"foo/bar": "baz"}
	pb := newPodBuilder(nil, nil, nil)
	pod, ok := pb.makePodForContainer(db.Container{
		FilepathToContent: map[string]blueprint.ContainerValue{
			"foo/bar": blueprint.NewString("baz"),
		},
	})
	assert.True(t, ok)
	assert.Len(t, pod.Volumes, 1)
	assert.Len(t, pod.Containers[0].VolumeMounts, 1)

	assert.Equal(t, pod.Volumes[0].ConfigMap.Name, configMapName(fileMap))
	assert.Equal(t, pod.Volumes[0].Name, pod.Containers[0].VolumeMounts[0].Name)
	assert.Equal(t, pod.Containers[0].VolumeMounts[0].MountPath, "foo/bar")
	assert.Equal(t, pod.Containers[0].VolumeMounts[0].SubPath, configMapKey("foo/bar"))
}

func TestPodBuilderSecret(t *testing.T) {
	t.Parallel()

	secretClient := &mocks.SecretClient{}
	pb := newPodBuilder(secretClient, nil, nil)

	mySecretName := "mySecret"
	kubeName, _ := secretRef(mySecretName)
	mySecretVal := "mySecretVal"
	containerValueMap := map[string]blueprint.ContainerValue{
		"myKey": blueprint.NewSecret(mySecretName),
	}

	// Test secret whose value isn't set yet.
	secretClient.On("Get", mock.Anything).Return("", assert.AnError).Once()
	_, ok := pb.makePodForContainer(db.Container{
		FilepathToContent: containerValueMap,
	})
	assert.False(t, ok)
	secretClient.AssertExpectations(t)

	// Once the value is set, we should be able to make the pod.
	secretClient.On("Get", mySecretName).Return(mySecretVal, nil).Once()
	pod, ok := pb.makePodForContainer(db.Container{
		FilepathToContent: containerValueMap,
	})
	assert.True(t, ok)
	secretClient.AssertExpectations(t)

	assert.Len(t, pod.Volumes, 1)
	assert.Len(t, pod.Containers[0].VolumeMounts, 1)
	assert.Equal(t, pod.Volumes[0].Name, pod.Containers[0].VolumeMounts[0].Name)
	assert.Equal(t, kubeName, pod.Volumes[0].Secret.SecretName)
	assert.Equal(t, "myKey", pod.Containers[0].VolumeMounts[0].MountPath)
	assert.Equal(t, secretKey, pod.Containers[0].VolumeMounts[0].SubPath)

	secretHashVolume, ok := getSecretEnvHash(pod, mySecretName)
	assert.True(t, ok)

	// Test referencing secrets in environment variables.
	secretClient.On("Get", mySecretName).Return(mySecretVal, nil).Once()
	pod, ok = pb.makePodForContainer(db.Container{
		Env: containerValueMap,
	})
	assert.True(t, ok)
	secretClient.AssertExpectations(t)
	assert.Contains(t, pod.Containers[0].Env, corev1.EnvVar{
		Name: "myKey",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: kubeName,
				},
				Key: secretKey,
			},
		},
	})

	// The secret hash should be the same whether the secret is referenced from
	// a volume or environment variable.
	secretHashEnv, ok := getSecretEnvHash(pod, mySecretName)
	assert.True(t, ok)
	assert.Equal(t, secretHashVolume, secretHashEnv)

	// If the secret value changes, the container's environment variables
	// should change.
	mySecretVal = "changed"
	secretClient.On("Get", mySecretName).Return(mySecretVal, nil).Once()
	pod, ok = pb.makePodForContainer(db.Container{
		FilepathToContent: containerValueMap,
	})
	assert.True(t, ok)
	secretClient.AssertExpectations(t)

	newSecretHash, ok := getSecretEnvHash(pod, mySecretName)
	assert.True(t, ok)
	assert.NotEqual(t, secretHashEnv, newSecretHash)
}

func TestPodBuilderCustomImage(t *testing.T) {
	t.Parallel()

	readyImage := "readyImage"
	readyDockerfile := "readyDockerfile"
	readyRepoDigest := "readyRepoDigest"

	buildingImage := "buildingImage"
	buildingDockerfile := "buildingDockerfile"

	images := []db.Image{
		{
			Name:       readyImage,
			Dockerfile: readyDockerfile,
			RepoDigest: readyRepoDigest,
			Status:     db.Built,
		},
		{
			Name:       buildingImage,
			Dockerfile: buildingDockerfile,
			Status:     db.Building,
		},
	}
	pb := newPodBuilder(nil, images, nil)

	// Test that a container whose image is pulled from outside the cluster is
	// unchanged.
	regularImage := "alpine"
	pod, ok := pb.makePodForContainer(db.Container{Image: regularImage})
	assert.Equal(t, regularImage, pod.Containers[0].Image)
	assert.True(t, ok)

	// Test that the container whose image is built gets its image rewritten.
	pod, ok = pb.makePodForContainer(db.Container{
		Image:      readyImage,
		Dockerfile: readyDockerfile,
	})
	assert.Equal(t, readyRepoDigest, pod.Containers[0].Image)
	assert.True(t, ok)

	// Test that the cointainer whose image is not ready yet is marked as invalid.
	_, ok = pb.makePodForContainer(db.Container{
		Image:      buildingImage,
		Dockerfile: buildingDockerfile,
	})
	assert.False(t, ok)
}

func getSecretEnvHash(pod corev1.PodSpec, secretName string) (string, bool) {
	for _, env := range pod.Containers[0].Env {
		if env.Name == "SECRET_HASH_"+secretName {
			return env.Value, true
		}
	}
	return "", false
}
