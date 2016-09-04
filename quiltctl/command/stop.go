package command

import (
	"flag"
	"fmt"

	log "github.com/Sirupsen/logrus"

	"github.com/NetSys/quilt/api"
)

// Stop contains the options for stopping namespaces.
type Stop struct {
	host      string
	namespace string

	flags *flag.FlagSet
}

func (sCmd *Stop) createFlagSet() *flag.FlagSet {
	flags := flag.NewFlagSet("stop", flag.ExitOnError)

	flags.StringVar(&sCmd.host, "H", api.DefaultSocket,
		"the host to connect to")
	flags.StringVar(&sCmd.namespace, "namespace", "", "the namespace to stop")

	flags.Usage = func() {
		fmt.Println("usage: quilt stop [-H=<daemon_host>] " +
			"[-namespace=<namespace>] <namespace>]")
		fmt.Println("`stop` creates an empty Stitch for the given namespace, " +
			"and sends it to the Quilt daemon to be executed.")
		fmt.Println("The result is that resources associated with the " +
			"namespace, such as VMs, are freed.")
		sCmd.flags.PrintDefaults()
	}

	sCmd.flags = flags
	return flags
}

// Parse parses the command line arguments for the stop command.
func (sCmd *Stop) Parse(args []string) error {
	flags := sCmd.createFlagSet()

	if err := flags.Parse(args); err != nil {
		return err
	}

	if sCmd.namespace == "" {
		nonFlagArgs := flags.Args()
		if len(nonFlagArgs) > 0 {
			sCmd.namespace = nonFlagArgs[0]
		}
	}

	return nil
}

// Run stops the given namespace.
func (sCmd *Stop) Run() int {
	namespace := sCmd.namespace
	var specStr string
	if namespace != "" {
		specStr += fmt.Sprintf("setNamespace(%q);", namespace)
	}

	c, err := getClient(sCmd.host)
	if err != nil {
		log.Error(err)
		return 1
	}
	defer c.Close()

	if err = c.RunStitch(specStr); err != nil {
		log.WithError(err).Error("Unable to stop namespace.")
		return 1
	}

	fmt.Printf("Successfully began stopping namespace `%s`.\n", namespace)

	return 0
}

// Usage prints the usage for the stop command.
func (sCmd *Stop) Usage() {
	sCmd.flags.Usage()
}
