package main

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/kelda/kelda/integration-tester/util"

	"github.com/stretchr/testify/assert"
)

func TestSecretValues(t *testing.T) {
	clnt, _, err := util.GetDefaultDaemonClient()
	if err != nil {
		t.Fatalf("couldn't get api client: %s", err)
	}
	defer clnt.Close()

	containers, err := clnt.QueryContainers()
	if err != nil {
		t.Fatalf("couldn't query containers: %s", err)
	}

	assert.Len(t, containers, 1)
	containerID := containers[0].BlueprintID

	// The secret values are defined in the 70-secret-setup test.
	// The non-secret values are defined in the blueprint accompanying this test.
	expEnv := map[string]string{
		"NOT_A_SECRET": "plaintext",
		"MY_SECRET":    "env secret",
	}

	expFiles := map[string]string{
		"/notASecret": "plaintext",
		"/mySecret":   "file secret",
	}

	for key, expVal := range expEnv {
		actualVal, err := getEnv(containerID, key)
		assert.NoError(t, err)

		assert.Equal(t, expVal, actualVal)
		fmt.Printf("Expected environment variable %q to have value %q. Got %q.\n",
			key, expVal, actualVal)
	}

	for path, expVal := range expFiles {
		actualVal, err := getFile(containerID, path)
		assert.NoError(t, err)

		assert.Equal(t, expVal, actualVal)
		fmt.Printf("Expected file %q to have value %q. Got %q.\n",
			path, expVal, actualVal)
	}
}

func getEnv(containerID, key string) (string, error) {
	envBytes, err := exec.Command("kelda", "ssh", containerID,
		"printenv", key).Output()
	if err != nil {
		return "", err
	}

	// Trim the newline appended by printenv.
	return strings.TrimRight(string(envBytes), "\n"), nil
}

func getFile(containerID, path string) (string, error) {
	fileBytes, err := exec.Command("kelda", "ssh", containerID, "cat", path).Output()
	if err != nil {
		return "", err
	}
	return string(fileBytes), nil
}

func TestSecretsEncrypted(t *testing.T) {
	// TODO
	// ETCDCTL_API=3 etcdctl get /registry/secrets/ --prefix --keys-only
	// for each key
	// assert etcdctl get ${secret}| hexdump -C | grep aescbc
}
