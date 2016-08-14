package command

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"

	log "github.com/Sirupsen/logrus"

	"github.com/NetSys/quilt/api"
)

// SSH contains the options for SSHing into machines.
type SSH struct {
	host          string
	targetMachine int
	sshArgs       []string

	flags *flag.FlagSet
}

func (sCmd *SSH) createFlagSet() *flag.FlagSet {
	flags := flag.NewFlagSet("ssh", flag.ExitOnError)

	flags.StringVar(&sCmd.host, "H", api.DefaultSocket,
		"the host to query for machine information")

	flags.Usage = func() {
		fmt.Println("usage: quilt ssh [-H=<daemon_host>] <machine_num> " +
			"[ssh_options]")
		fmt.Println("`ssh` creates a SSH session to the specified machine. " +
			"The machine is identified the database ID produced by " +
			"`quilt queryMachines`.")
		fmt.Println("For example, to SSH to machine 5 with a specific " +
			"private key: quilt ssh 5 -i ~/.ssh/quilt")
		sCmd.flags.PrintDefaults()
	}

	sCmd.flags = flags
	return flags
}

// Parse parses the command line arguments for the ssh command.
func (sCmd *SSH) Parse(rawArgs []string) error {
	flags := sCmd.createFlagSet()

	if err := flags.Parse(rawArgs); err != nil {
		return err
	}

	parsedArgs := flags.Args()
	if len(parsedArgs) == 0 {
		return errors.New("must specify a target machine")
	}

	targetMachine, err := strconv.Atoi(parsedArgs[0])
	if err != nil {
		return fmt.Errorf("target machine must be a number: %s", parsedArgs[0])
	}

	sCmd.targetMachine = targetMachine
	sCmd.sshArgs = parsedArgs[1:]
	return nil
}

// Run SSHs into the given machine.
func (sCmd *SSH) Run() int {
	c, err := getClient(sCmd.host)
	if err != nil {
		log.Error(err)
		return 1
	}
	defer c.Close()

	machines, err := c.QueryMachines()
	if err != nil {
		log.WithError(err).Error("Unable to query machines.")
		return 1
	}

	var host string
	for _, m := range machines {
		if m.ID == sCmd.targetMachine {
			host = m.PublicIP
			break
		}
	}

	if host == "" {
		missingMachineMsg :=
			fmt.Sprintf("Unable to find machine `%d`.\n", sCmd.targetMachine)
		missingMachineMsg += "Available machines:\n"
		for _, m := range machines {
			missingMachineMsg += fmt.Sprintf("%v\n", m)
		}
		log.Error(missingMachineMsg)
		return 1
	}

	if err = ssh(host, sCmd.sshArgs).Run(); err != nil {
		log.WithError(err).Error("Error executing the SSH command")
		return 1
	}
	return 0
}

// Usage prints the usage for the ssh command.
func (sCmd *SSH) Usage() {
	sCmd.flags.Usage()
}

// Stored in a variable so we can mock it out for unit tests.
var ssh = func(host string, args []string) *exec.Cmd {
	baseArgs := []string{fmt.Sprintf("quilt@%s", host),
		"-o", "StrictHostKeyChecking=no"}

	cmd := exec.Command("ssh", append(baseArgs, args...)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}
