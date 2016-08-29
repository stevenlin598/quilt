package command

import (
	"errors"
	"flag"
	"fmt"

	log "github.com/Sirupsen/logrus"
	"github.com/robertkrimen/otto"

	"github.com/NetSys/quilt/api"
	"github.com/NetSys/quilt/stitch"
)

// Run contains the options for running Stitches.
type Run struct {
	stitch string
	host   string

	flags *flag.FlagSet
}

func (rCmd *Run) createFlagSet() *flag.FlagSet {
	flags := flag.NewFlagSet("run", flag.ExitOnError)

	flags.StringVar(&rCmd.stitch, "stitch", "", "the stitch to run")
	flags.StringVar(&rCmd.host, "H", api.DefaultSocket,
		"the host to connect to")

	flags.Usage = func() {
		fmt.Println("usage: quilt run [-H=<daemon_host>] " +
			"[-stitch=<stitch>] <stitch>")
		fmt.Println("`run` compiles the provided stitch, and sends the " +
			"result to the Quilt daemon to be executed.")
		rCmd.flags.PrintDefaults()
	}

	rCmd.flags = flags
	return flags
}

// Parse parses the command line arguments for the run command.
func (rCmd *Run) Parse(args []string) error {
	flags := rCmd.createFlagSet()

	if err := flags.Parse(args); err != nil {
		return err
	}

	if rCmd.stitch == "" {
		nonFlagArgs := flags.Args()
		if len(nonFlagArgs) == 0 {
			return errors.New("no spec specified")
		}
		rCmd.stitch = nonFlagArgs[0]
	}

	return nil
}

// Run starts the run for the provided Stitch.
func (rCmd *Run) Run() int {
	c, err := getClient(rCmd.host)
	if err != nil {
		log.Error(err)
		return 1
	}
	defer c.Close()

	compiled, err := stitch.Compile(rCmd.stitch, stitch.DefaultImportGetter)
	if err != nil {
		// Print the stacktrace if it's an Otto error.
		if ottoError, ok := err.(*otto.Error); ok {
			log.Error(ottoError.String())
		} else {
			log.Error(err)
		}
		return 1
	}

	err = c.RunStitch(compiled)
	if err != nil {
		log.WithError(err).Error("Unable to start run.")
		return 1
	}

	fmt.Println("Successfully started run.")
	return 0
}

// Usage prints the usage for the run command.
func (rCmd *Run) Usage() {
	rCmd.flags.Usage()
}
