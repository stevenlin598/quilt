package command

import (
	"flag"
	"fmt"

	log "github.com/Sirupsen/logrus"

	"github.com/NetSys/quilt/api"
	"github.com/NetSys/quilt/db"
)

// Container contains the options for querying containers.
type Container struct {
	host string

	flags *flag.FlagSet
}

func (cCmd *Container) createFlagSet() {
	flags := flag.NewFlagSet("containers", flag.ExitOnError)
	flags.StringVar(&cCmd.host, "H", api.DefaultSocket, "the host to connect to")
	cCmd.flags = flags
}

// Parse parses the command line arguments for the container command.
func (cCmd *Container) Parse(args []string) error {
	cCmd.createFlagSet()
	return cCmd.flags.Parse(args)
}

// Run retrieves and prints the requested containers.
func (cCmd *Container) Run() int {
	localClient, err := getClient(cCmd.host)
	if err != nil {
		log.Error(err)
		return 1
	}

	c, err := getLeaderClient(localClient)
	localClient.Close()
	if err != nil {
		log.WithError(err).Error("Error connecting to leader.")
		return 1
	}
	defer c.Close()

	containers, err := c.QueryContainers()
	if err != nil {
		log.WithError(err).Error("Unable to query containers.")
		return 1
	}

	str := containersStr(containers)
	fmt.Print(str)

	return 0
}

func containersStr(containers []db.Container) string {
	var containersStr string
	for _, c := range containers {
		containersStr += fmt.Sprintf("%v\n", c)
	}

	return containersStr
}

// Usage prints the usage for the container command.
func (cCmd *Container) Usage() {
	cCmd.flags.Usage()
}
