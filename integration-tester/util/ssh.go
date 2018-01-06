package util

import (
	"errors"
	"fmt"
	"strings"

	"github.com/kelda/kelda/cli/ssh"
	"github.com/kelda/kelda/db"
	"github.com/kelda/kelda/minion/supervisor"
)

// The maximum number of SSH Sessions to open to the leader.
const maxSSHSessions = 512

// SSHUtil makes it easy to parallelize executing commands on Kelda containers.
//
// It does several things to make executing commands faster:
// 1. It reuses SSH connections.
//
// 2. It caches state. Shelling out to `kelda ssh` requires requerying all
// machines and containers in order to figure out the container's public IP
// address.
//
// 3. It rate limits the number of parallel connections and session creations
// to avoid overloading the host.
type SSHUtil struct {
	clients chan ssh.Client
}

// NewSSHUtil creates a new SSHUtil instance configured to connect to containers
// on the given machines. Any calls to `SSHUtil.SSH` for a container scheduled
// on a machine not given here will fail.
func NewSSHUtil(leaderIP string) SSHUtil {
	sshUtil := SSHUtil{make(chan ssh.Client, maxSSHSessions)}

	// We have to limit parallelization setting up SSH sessions.  Doing so too
	// quickly in parallel breaks system-logind on the remote machine:
	// https://github.com/systemd/systemd/issues/2925.  Furthermore, the concurrency
	// limit cannot exceed the sshd MaxStartups setting, or else the SSH connections
	// may be randomly rejected.
	//
	// Also note, we intentionally don't wait for this go routine to finish.  As new
	// SSH connections are created, the tests can gradually take advantage of them.
	go func() {
		for i := 0; i < maxSSHSessions; i++ {
			client, err := ssh.New(leaderIP, "")
			if err != nil {
				fmt.Printf("failed to ssh to %s: %s", leaderIP, err)
				continue
			}
			sshUtil.clients <- client
		}
	}()
	return sshUtil
}

// SSH executes `cmd` on the given container, and returns the stdout and stderr
// output of the command in a single string.
func (sshUtil SSHUtil) SSH(dbc db.Container, containerCmd ...string) (string, error) {
	if dbc.PodName == "" {
		return "", errors.New("container not yet booted")
	}

	ssh := <-sshUtil.clients
	defer func() { sshUtil.clients <- ssh }()

	containerCmdStr := strings.Join(containerCmd, " ")
	fmt.Println(dbc.BlueprintID, containerCmdStr)
	sshCmd := []string{
		"docker", "exec", supervisor.KubeAPIServerName,
		"kubectl", "exec", dbc.PodName, "--", containerCmdStr,
	}
	ret, err := ssh.CombinedOutput(strings.Join(sshCmd, " "))
	return string(ret), err
}
