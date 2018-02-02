package util

import (
	"fmt"

	"github.com/kelda/kelda/api/client"
	"github.com/kelda/kelda/db"
)

// FuzzyLookup finds either a container or a machine that's called `name` by querying the
// provided `client`.  `name` could be a container's hostname, the prefix of a
// container's blueprint ID, or the prefix of a machine's cloud ID.
func FuzzyLookup(client client.Client, name string) (interface{}, error) {
	var machine db.Machine
	machines, machineError := client.QueryMachines()
	if machineError == nil {
		machine, machineError = findMachine(machines, name)
	}

	var container db.Container
	containers, containerError := client.QueryContainers()
	if containerError == nil {
		container, containerError = findContainer(containers, name)
	}

	resolvedMachine := machineError == nil
	resolvedContainer := containerError == nil
	switch {
	case !resolvedMachine && !resolvedContainer:
		return nil, fmt.Errorf("%s, %s", machineError, containerError)
	case resolvedMachine && resolvedContainer:
		return nil, fmt.Errorf("ambiguous IDs: machine %q, container %q",
			machine.CloudID, container.BlueprintID)
	}

	if resolvedMachine {
		return machine, nil
	}
	return container, nil
}

func findContainer(containers []db.Container, id string) (db.Container, error) {
	var choice *db.Container
	hostnameToContainer := map[string]db.Container{}
	for _, c := range containers {
		hostnameToContainer[c.Hostname] = c

		if len(id) > len(c.BlueprintID) || c.BlueprintID[0:len(id)] != id {
			continue
		}

		if choice != nil {
			err := fmt.Errorf("ambiguous choices %s and %s",
				choice.BlueprintID, c.BlueprintID)
			return db.Container{}, err
		}

		copy := c
		choice = &copy
	}

	if choice != nil {
		return *choice, nil
	}

	if c, ok := hostnameToContainer[id]; ok {
		return c, nil
	}

	return db.Container{}, fmt.Errorf("no container %q", id)
}

func findMachine(machines []db.Machine, id string) (db.Machine, error) {
	var choice *db.Machine
	for _, m := range machines {
		if len(id) > len(m.CloudID) || m.CloudID[:len(id)] != id {
			continue
		}
		if choice != nil {
			return db.Machine{}, fmt.Errorf("ambiguous machines %q and %q",
				choice.CloudID, m.CloudID)
		}
		copy := m
		choice = &copy
	}

	if choice == nil {
		return db.Machine{}, fmt.Errorf("no machine %q", id)
	}

	return *choice, nil
}
