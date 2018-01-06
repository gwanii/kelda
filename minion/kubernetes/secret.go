package kubernetes

import (
	"crypto/sha1"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreclient "k8s.io/client-go/kubernetes/typed/core/v1"
)

// Each secret name maps to a unique Kubernetes secret. Because a Kubernetes
// secret is a map of values rather than a single value, we only use a single
// key in the map.
// secretKey is the key in which the secret value is always stored.
const secretKey = "value"

// SecretClient provides an interface for writing and reading secrets from
// Kubernetes.
type SecretClient interface {
	Exists(name string) bool
	Get(name string) (string, error)
	Set(name, val string) error
}

type secretClientImpl struct {
	client coreclient.SecretInterface
}

// NewSecretClient returns a SecretClient connected to the local Kubernetes
// API server.
func NewSecretClient() (SecretClient, error) {
	clientset, err := newClientset()
	if err != nil {
		return nil, err
	}

	kubeClient := clientset.CoreV1().Secrets(corev1.NamespaceDefault)
	return secretClientImpl{kubeClient}, nil
}

func (sc secretClientImpl) Exists(name string) bool {
	_, err := sc.Get(name)
	return err == nil
}

func (sc secretClientImpl) Get(name string) (string, error) {
	kubeName, _ := secretRef(name)
	secret, err := sc.client.Get(kubeName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("query secret: %s", err)
	}

	val, ok := secret.Data[secretKey]
	if !ok {
		return "", errors.New("malformed secret")
	}
	return string(val), nil
}

func (sc secretClientImpl) Set(name, val string) error {
	kubeName, key := secretRef(name)
	desiredSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: kubeName,
		},
		Data: map[string][]byte{
			key: []byte(val),
		},
	}

	var err error
	if sc.Exists(name) {
		_, err = sc.client.Update(&desiredSecret)
	} else {
		_, err = sc.client.Create(&desiredSecret)
	}
	return err
}

func secretRef(name string) (kubeSecretName, key string) {
	return "kelda-" + fmt.Sprintf("%x", sha1.Sum([]byte(name))), secretKey
}
