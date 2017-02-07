package command

import (
	"errors"
	"flag"
	"fmt"

	log "github.com/Sirupsen/logrus"

	"github.com/quilt/quilt/api"
	"github.com/quilt/quilt/api/client"
	"github.com/quilt/quilt/api/client/getter"
	"github.com/quilt/quilt/cluster"
	"github.com/quilt/quilt/cluster/machine"
)

// Switch contains the options for switching namespaces
type Switch struct {
	namespace    string
	common       *commonFlags
	clientGetter client.Getter
}

// NewSwitchCommand creates an instance of the Switch command.
func NewSwitchCommand() *Switch {
	return &Switch{
		common:       &commonFlags{},
		clientGetter: getter.New(),
	}
}

// For mock testing
var machineGetter = cluster.GetMachines

// InstallFlags sets up parsing for command line flags.
func (sCmd *Switch) InstallFlags(flags *flag.FlagSet) {
	sCmd.common.InstallFlags(flags)

	flags.Usage = func() {
		fmt.Println("usage: quilt switch <namespace>")
		fmt.Printf("`switch` switches to a different namespace\n")
		flags.PrintDefaults()
	}
}

// Parse parses the command line arguments for the switch command.
func (sCmd *Switch) Parse(args []string) error {
	if len(args) == 0 {
		return errors.New("no namespace specified")
	}
	sCmd.namespace = args[0]
	return nil
}

// Run switches the current cluster to the specified namespace.
func (sCmd *Switch) Run() int {
	log.Info("Switching to namespace " + sCmd.namespace)

	machines, err := machineGetter(sCmd.namespace)
	if err != nil {
		log.Error(err)
		return 1
	}
	if len(machines) < 1 {
		log.Error("no cluster running with namespace " + sCmd.namespace)
		return 1
	}

	spec, err := sCmd.getSpec(machines)
	if err != nil {
		log.Error(err)
		return 1
	}

	c, err := sCmd.clientGetter.Client(sCmd.common.host)
	if err != nil {
		log.Error(err)
		return 1
	}
	defer c.Close()

	err = c.Deploy(spec)
	if err != nil {
		log.WithError(err).Error("error while starting run.")
		return 1
	}
	log.Info("Successfully deployed spec")
	return 0
}

// getSpec attempts to retrieve the spec from the machines in the cluster
func (sCmd Switch) getSpec(machines []machine.Machine) (string, error) {
	for _, machine := range machines {
		machineClient, err := sCmd.clientGetter.Client(
			api.RemoteAddress(machine.PublicIP))
		if err != nil {
			log.Error(err)
			continue
		}

		minions, err := machineClient.QueryMinions()
		if err != nil {
			log.Error(err)
			continue
		}
		for _, minion := range minions {
			if minion.Spec != "" {
				return minion.Spec, nil
			}
		}
	}
	return "", errors.New("none of the machines have a valid spec")
}
