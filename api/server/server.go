package server

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/NetSys/quilt/api"
	"github.com/NetSys/quilt/api/pb"
	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/engine"
	"github.com/NetSys/quilt/stitch"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	log "github.com/Sirupsen/logrus"
)

type server struct {
	dbConn db.Conn
}

// Run accepts incoming `quiltctl` connections and responds to them.
func Run(conn db.Conn, listenAddr string) error {
	proto, addr, err := api.ParseListenAddress(listenAddr)
	if err != nil {
		return err
	}

	var sock net.Listener
	apiServer := server{conn}
	for {
		sock, err = net.Listen(proto, addr)

		if err == nil {
			break
		}
		log.WithError(err).Error("Failed to open socket.")

		time.Sleep(30 * time.Second)
	}

	// Cleanup the socket if we're interrupted.
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, os.Kill, syscall.SIGTERM, syscall.SIGHUP)
	go func(c chan os.Signal) {
		sig := <-c
		log.Printf("Caught signal %s: shutting down.\n", sig)
		sock.Close()
		os.Exit(0)
	}(sigc)

	s := grpc.NewServer()
	pb.RegisterAPIServer(s, apiServer)
	s.Serve(sock)

	return nil
}

func (s server) Query(cts context.Context, query *pb.DBQuery) (*pb.QueryReply, error) {
	var rows interface{}
	err := s.dbConn.Transact(func(view db.Database) error {
		switch db.TableType(query.Table) {
		case db.MachineTable:
			rows = view.SelectFromMachine(nil)
		case db.ContainerTable:
			rows = view.SelectFromContainer(nil)
		case db.EtcdTable:
			rows = view.SelectFromEtcd(nil)
		default:
			return fmt.Errorf("unrecognized table: %s", query.Table)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	json, err := json.Marshal(rows)
	if err != nil {
		return nil, err
	}

	return &pb.QueryReply{TableContents: string(json)}, nil
}

func (s server) Run(cts context.Context, runReq *pb.RunRequest) (*pb.RunReply, error) {
	stitch, err := stitch.New(runReq.Stitch, stitch.DefaultImportGetter)
	if err != nil {
		return &pb.RunReply{}, err
	}

	err = engine.UpdatePolicy(s.dbConn, stitch)
	if err != nil {
		return &pb.RunReply{}, err
	}

	return &pb.RunReply{}, nil
}
