package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"strings"
	"testing"

	"github.com/kelda/kelda/integration-tester/util"

	"github.com/stretchr/testify/assert"
)

const nginxIndexPath = "/usr/share/nginx/html/index.html"

func TestPublicIPEnv(t *testing.T) {
	clnt, _, err := util.GetDefaultDaemonClient()
	assert.NoError(t, err)
	defer clnt.Close()

	containers, err := clnt.QueryContainers()
	assert.NoError(t, err)

	assert.Len(t, containers, 1)
	container := containers[0]

	pubIP, err := getEnv(container.BlueprintID, "myPubIP")
	assert.NoError(t, err)
	assert.NotEmpty(t, pubIP)

	expContent := container.FilepathToContent[nginxIndexPath].Value.(string)
	actual, err := getBody("http://" + pubIP)
	assert.NoError(t, err)

	fmt.Println("The container's public IP environment variable is", pubIP)
	fmt.Println("Expecting the homepage to contain", expContent)
	fmt.Println("Got", actual)
	assert.Equal(t, expContent, actual)
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

func getBody(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}
