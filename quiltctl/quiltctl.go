package quiltctl

import (
	"flag"
	"fmt"
	"os"

	"github.com/NetSys/quilt/quiltctl/command"

	log "github.com/Sirupsen/logrus"
)

var commands = map[string]command.SubCommand{
	"machines":   &command.Machine{},
	"containers": &command.Container{},
	"get":        &command.Get{},
	"inspect":    &command.Inspect{},
	"run":        &command.Run{},
	"stop":       &command.Stop{},
	"ssh":        &command.SSH{},
	"exec":       &command.Exec{},
}

// Run parses and runs the quiltctl subcommand given the command line arguments.
func Run(args []string) {
	if len(args) == 0 {
		usage()
	}

	cmd, err := parseSubcommand(args[0], args[1:])
	if err != nil {
		log.WithError(err).Error("Unable to parse subcommand.")
		usage()
	}

	os.Exit(cmd.Run())
}

// HasSubcommand returns true if quiltctl has a subcommand for the given name.
func HasSubcommand(name string) bool {
	_, ok := commands[name]
	return ok
}

func parseSubcommand(name string, args []string) (command.SubCommand, error) {
	if !HasSubcommand(name) {
		return nil, fmt.Errorf("unrecognized subcommand: %s", name)
	}

	cmd := commands[name]
	err := cmd.Parse(args)
	if err != nil {
		cmd.Usage()
		return nil, err
	}

	return cmd, nil
}

func usage() {
	flag.Usage()
	os.Exit(1)
}
