package main

import (
	"testing"

	"github.com/kelda/kelda/api/client"
	"github.com/kelda/kelda/integration-tester/util"
)

func TestNetwork(t *testing.T) {
	clnt, creds, err := util.GetDefaultDaemonClient()
	if err != nil {
		t.Fatalf("couldn't get api client: %s", err.Error())
	}
	defer clnt.Close()

	containers, err := clnt.QueryContainers()
	if err != nil {
		t.Fatalf("couldn't query containers: %s", err.Error())
	}

	machines, err := clnt.QueryMachines()
	if err != nil {
		t.Fatalf("failed to query machines: %s", err)
	}

	loadBalancers, err := clnt.QueryLoadBalancers()
	if err != nil {
		t.Fatalf("failed to query load balancers: %s", err)
	}

	connections, err := clnt.QueryConnections()
	if err != nil {
		t.Fatalf("Failed to query connections: %s", err)
	}

	leaderIP, err := client.GetLeaderIP(machines, creds)
	if err != nil {
		t.Fatalf("failed to get leader IP: %s", err)
	}

	sshUtil := util.NewSSHUtil(leaderIP)
	t.Run("DNS", func(t *testing.T) {
		t.Parallel()
		testDNS(t, sshUtil, containers, loadBalancers)
	})

	t.Run("Ping", func(t *testing.T) {
		t.Parallel()
		testPing(t, sshUtil, containers, loadBalancers, connections)
	})

	t.Run("HPing", func(t *testing.T) {
		t.Parallel()
		testHPing(t, sshUtil, containers, connections)
	})
}
