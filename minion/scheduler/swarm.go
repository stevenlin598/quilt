package scheduler

import (
	"fmt"
	"strings"
	"sync"

	"github.com/NetSys/di/db"
	"github.com/NetSys/di/minion/docker"
)

type swarm struct {
	dk docker.Client
}

func newSwarm(dk docker.Client) scheduler {
	return swarm{dk}
}

func (s swarm) list() ([]docker.Container, error) {
	return s.dk.List(map[string][]string{"label": {"DI=Scheduler"}})
}

func (s swarm) boot(dbcs []db.Container) {
	var wg sync.WaitGroup
	wg.Add(len(dbcs))

	logChn := make(chan string, 1)
	for _, dbc := range dbcs {
		dbc := dbc
		go func() {
			err := s.dk.Run(docker.RunOptions{
				Image:  dbc.Image,
				Args:   dbc.Command,
				Labels: map[string]string{"DI": "Scheduler"},
			})
			if err != nil {
				msg := fmt.Sprintf("Failed to start container %s: %s",
					dbc.Image, err)
				select {
				case logChn <- msg:
				default:
				}
			} else {
				log.Info("Started container: %s %s", dbc.Image,
					strings.Join(dbc.Command, " "))
			}
			wg.Done()
		}()
	}

	wg.Wait()

	select {
	case msg := <-logChn:
		log.Warning(msg)
	default:
	}
}

func (s swarm) terminate(ids []string) {
	var wg sync.WaitGroup
	wg.Add(len(ids))
	defer wg.Wait()
	for _, id := range ids {
		id := id
		go func() {
			err := s.dk.RemoveID(id)
			if err != nil {
				log.Warning("Failed to stop container: %s", err)
			}
			wg.Done()
		}()
	}
}
