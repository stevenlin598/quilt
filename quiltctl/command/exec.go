package command

import (
	"errors"
	"flag"
	"fmt"
	"strconv"
	"strings"

	log "github.com/Sirupsen/logrus"

	"github.com/NetSys/quilt/api"
	"github.com/NetSys/quilt/api/client"
	"github.com/NetSys/quilt/db"
)

// Exec contains the options for running commands in containers.
type Exec struct {
	host            string
	privateKey      string
	targetContainer int
	command         string

	flags *flag.FlagSet
}

func (eCmd *Exec) createFlagSet() *flag.FlagSet {
	flags := flag.NewFlagSet("exec", flag.ExitOnError)

	flags.StringVar(&eCmd.host, "H", api.DefaultSocket,
		"the host to query for machine information")
	flags.StringVar(&eCmd.privateKey, "i", "",
		"the private key to use to connect to the host")

	flags.Usage = func() {
		fmt.Println("usage: quilt exec [-H=<daemon_host>] [-i=<private_key>] " +
			"<stitch_id> <command>")
		fmt.Println("`exec` runs a command within the specified container. " +
			"The container is identified by the stitch ID produced by " +
			"`quilt containers`.")
		fmt.Println("For example, to get a shell in container 5 with a " +
			"specific private key: quilt exec -i ~/.ssh/quilt 5 sh")
		eCmd.flags.PrintDefaults()
	}

	eCmd.flags = flags
	return flags
}

// Parse parses the command line arguments for the exec command.
func (eCmd *Exec) Parse(rawArgs []string) error {
	flags := eCmd.createFlagSet()

	if err := flags.Parse(rawArgs); err != nil {
		return err
	}

	parsedArgs := flags.Args()
	if len(parsedArgs) < 2 {
		return errors.New("must specify a target container and command")
	}

	targetContainer, err := strconv.Atoi(parsedArgs[0])
	if err != nil {
		return fmt.Errorf("target container must be a number: %s", parsedArgs[0])
	}

	eCmd.targetContainer = targetContainer
	eCmd.command = strings.Join(parsedArgs[1:], " ")
	return nil
}

// Run finds the target continer, and executes the given command in it.
func (eCmd *Exec) Run() int {
	localClient, leaderClient, err := getClients(eCmd.host)
	if err != nil {
		log.Error(err)
		return 1
	}
	defer localClient.Close()
	defer leaderClient.Close()

	containerHost, err := getContainerHost(localClient, leaderClient,
		eCmd.targetContainer)
	if err != nil {
		log.WithError(err).
			Error("Error getting the host on which the container is running.")
		return 1
	}

	containerClient, err := getClient(api.RemoteAddress(containerHost))
	if err != nil {
		log.WithError(err).Error("Error connecting to leader.")
		return 1
	}
	defer containerClient.Close()

	container, err := getContainer(containerClient, eCmd.targetContainer)
	if err != nil {
		log.WithError(err).Error("Error retrieving the container information " +
			"from the container host.")
		return 1
	}

	// -t allows the docker command to receive input over SSH.
	sshArgs := []string{"-t"}
	if eCmd.privateKey != "" {
		sshArgs = append(sshArgs, "-i", eCmd.privateKey)
	}
	sshArgs = append(sshArgs,
		fmt.Sprintf("docker exec -it %s %s", container.DockerID, eCmd.command))
	if err = ssh(containerHost, sshArgs).Run(); err != nil {
		log.WithError(err).Error("Error running the exec command.")
		return 1
	}
	return 0
}

// Usage prints the usage for the ssh command.
func (eCmd *Exec) Usage() {
	eCmd.flags.Usage()
}

// Get a client connected to the local daemon and the daemon on the lead minion.
func getClients(host string) (client.Client, client.Client, error) {
	localClient, err := getClient(host)
	if err != nil {
		return nil, nil, err
	}

	leaderClient, err := getLeaderClient(localClient)
	if err != nil {
		localClient.Close()
		return nil, nil, err
	}

	return localClient, leaderClient, err
}

// Get the public IP of the machine the container is running on.
func getContainerHost(localClient client.Client, leaderClient client.Client,
	stitchID int) (string, error) {

	container, err := getContainer(leaderClient, stitchID)
	if err != nil {
		return "", err
	}

	if container.Minion == "" {
		return "", errors.New("container hasn't been scheduled yet")
	}

	return getPublicIP(localClient, container.Minion)
}

// Get the container information for the given stitchID stored by the client.
func getContainer(c client.Client, stitchID int) (db.Container, error) {
	containers, err := c.QueryContainers()
	if err != nil {
		return db.Container{}, err
	}

	for _, c := range containers {
		if c.StitchID == stitchID {
			return c, nil
		}
	}

	return db.Container{}, fmt.Errorf("no container with stitchID %d", stitchID)
}
