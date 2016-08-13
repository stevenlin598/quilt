package command

import (
	"errors"
	"reflect"
	"testing"

	"github.com/spf13/afero"

	"github.com/NetSys/quilt/api"
	"github.com/NetSys/quilt/api/client"
	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/util"
)

func TestMachineFlags(t *testing.T) {
	t.Parallel()

	expHost := "IP"

	machineCmd := Machine{}
	err := machineCmd.Parse([]string{"-H", expHost})

	if err != nil {
		t.Errorf("Unexpected error when parsing container args: %s", err.Error())
		return
	}

	if machineCmd.host != expHost {
		t.Errorf("Expected machine command to parse arg %s, but got %s",
			expHost, machineCmd.host)
	}
}

func TestMachineOutput(t *testing.T) {
	t.Parallel()

	res := machinesStr([]db.Machine{{
		ID:       1,
		Role:     db.Master,
		Provider: "Amazon",
		Region:   "us-west-1",
		Size:     "m4.large",
		PublicIP: "8.8.8.8",
	}})

	exp := "Machine-1{Master, Amazon us-west-1 m4.large, PublicIP=8.8.8.8}\n"
	if res != exp {
		t.Errorf("\nGot: %s\nExp: %s\n", res, exp)
	}
}

func TestContainerFlags(t *testing.T) {
	t.Parallel()

	expHost := "IP"

	containerCmd := Container{}
	err := containerCmd.Parse([]string{"-H", expHost})

	if err != nil {
		t.Errorf("Unexpected error when parsing container args: %s", err.Error())
		return
	}

	if containerCmd.host != expHost {
		t.Errorf("Expected container command to parse arg %s, but got %s",
			expHost, containerCmd.host)
	}
}

func TestContainerOutput(t *testing.T) {
	t.Parallel()

	res := containersStr([]db.Container{{ID: 1, Command: []string{"cmd", "arg"}}})
	exp := "Container-1{run  cmd arg}\n"
	if res != exp {
		t.Errorf("Expected container command to print %s, but got %s.", exp, res)
	}
}

func checkGetParsing(t *testing.T, args []string, expImport string, expErr error) {
	getCmd := Get{}
	err := getCmd.Parse(args)

	if expErr != nil {
		if err.Error() != expErr.Error() {
			t.Errorf("Expected error %s, but got %s",
				expErr.Error(), err.Error())
		}
		return
	}

	if err != nil {
		t.Errorf("Unexpected error when parsing get args: %s", err.Error())
		return
	}

	if getCmd.importPath != expImport {
		t.Errorf("Expected get command to parse arg %s, but got %s",
			expImport, getCmd.importPath)
	}
}

func TestGetFlags(t *testing.T) {
	t.Parallel()

	expImport := "spec"
	checkGetParsing(t, []string{"-import", expImport}, expImport, nil)
	checkGetParsing(t, []string{expImport}, expImport, nil)
	checkGetParsing(t, []string{}, "", errors.New("no import specified"))
}

func checkRunParsing(t *testing.T, args []string, expStitch string, expErr error) {
	runCmd := Run{}
	err := runCmd.Parse(args)

	if expErr != nil {
		if err.Error() != expErr.Error() {
			t.Errorf("Expected error %s, but got %s",
				expErr.Error(), err.Error())
		}
		return
	}

	if err != nil {
		t.Errorf("Unexpected error when parsing run args: %s", err.Error())
		return
	}

	if runCmd.stitch != expStitch {
		t.Errorf("Expected run command to parse arg %s, but got %s",
			expStitch, runCmd.stitch)
	}
}

func TestRunFlags(t *testing.T) {
	t.Parallel()

	expStitch := "spec"
	checkRunParsing(t, []string{"-stitch", expStitch}, expStitch, nil)
	checkRunParsing(t, []string{expStitch}, expStitch, nil)
	checkRunParsing(t, []string{}, "", errors.New("no spec specified"))
}

func checkStopParsing(t *testing.T, args []string, expNamespace string, expErr error) {
	stopCmd := Stop{}
	err := stopCmd.Parse(args)

	if expErr != nil {
		if err.Error() != expErr.Error() {
			t.Errorf("Expected error %s, but got %s",
				expErr.Error(), err.Error())
		}
		return
	}

	if err != nil {
		t.Errorf("Unexpected error when parsing stop args: %s", err.Error())
		return
	}

	if stopCmd.namespace != expNamespace {
		t.Errorf("Expected stop command to parse arg %s, but got %s",
			expNamespace, stopCmd.namespace)
	}
}

func TestStopFlags(t *testing.T) {
	t.Parallel()

	expNamespace := "namespace"
	checkStopParsing(t, []string{"-namespace", expNamespace}, expNamespace, nil)
	checkStopParsing(t, []string{expNamespace}, expNamespace, nil)
}

func checkSSHParsing(t *testing.T, args []string, expMachine int,
	expSSHArgs []string, expErr error) {

	sshCmd := SSH{}
	err := sshCmd.Parse(args)

	if expErr != nil {
		if err.Error() != expErr.Error() {
			t.Errorf("Expected error %s, but got %s",
				expErr.Error(), err.Error())
		}
		return
	}

	if err != nil {
		t.Errorf("Unexpected error when parsing ssh args: %s", err.Error())
		return
	}

	if sshCmd.targetMachine != expMachine {
		t.Errorf("Expected ssh command to parse target machine %d, but got %d",
			expMachine, sshCmd.targetMachine)
	}

	if !reflect.DeepEqual(sshCmd.sshArgs, expSSHArgs) {
		t.Errorf("Expected ssh command to parse SSH args %v, but got %v",
			expSSHArgs, sshCmd.sshArgs)
	}
}

func TestSSHFlags(t *testing.T) {
	t.Parallel()

	checkSSHParsing(t, []string{"1"}, 1, []string{}, nil)
	sshArgs := []string{"-i", "~/.ssh/key"}
	checkSSHParsing(t, append([]string{"1"}, sshArgs...), 1, sshArgs, nil)
	checkSSHParsing(t, []string{}, 0, nil,
		errors.New("must specify a target machine"))
}

type mockClient struct {
	machineReturn []db.Machine
	etcdReturn    []db.Etcd
	runStitchArg  string
}

func (c *mockClient) QueryMachines() ([]db.Machine, error) {
	return c.machineReturn, nil
}

func (c *mockClient) QueryContainers() ([]db.Container, error) {
	return nil, nil
}

func (c *mockClient) QueryEtcd() ([]db.Etcd, error) {
	return c.etcdReturn, nil
}

func (c *mockClient) Close() error {
	return nil
}

func (c *mockClient) RunStitch(stitch string) error {
	c.runStitchArg = stitch
	return nil
}

func TestStopNamespace(t *testing.T) {
	c := &mockClient{}
	getClient = func(host string) (client.Client, error) {
		return c, nil
	}

	stopCmd := &Stop{namespace: "namespace"}
	stopCmd.Run()
	expStitch := `AdminACL = [];Namespace = "namespace";`
	if c.runStitchArg != expStitch {
		t.Error("stop command invoked Quilt with the wrong stitch")
	}

	stopCmd = &Stop{}
	stopCmd.Run()
	expStitch = "AdminACL = [];"
	if c.runStitchArg != expStitch {
		t.Error("stop command invoked Quilt with the wrong stitch")
	}
}

func TestRunSpec(t *testing.T) {
	c := &mockClient{}
	getClient = func(host string) (client.Client, error) {
		return c, nil
	}
	util.AppFs = afero.NewMemMapFs()

	stitchPath := "test.spec"
	testSpec := `new Docker("nginx", {});`
	util.WriteFile(stitchPath, []byte(testSpec), 0644)

	runCmd := &Run{stitch: stitchPath}
	runCmd.Run()

	expStitch := `importSources = {};` + testSpec
	if c.runStitchArg != expStitch {
		t.Error("run command invoked Quilt with the wrong stitch")
	}
}

func TestGetLeaderClient(t *testing.T) {
	passedClient := &mockClient{}
	getClient = func(host string) (client.Client, error) {
		switch host {
		// One machine doesn't know the LeaderIP
		case api.RemoteAddress("8.8.8.8"):
			return &mockClient{
				etcdReturn: []db.Etcd{
					{
						LeaderIP: "",
					},
				},
			}, nil
		// The other machine knows the LeaderIP
		case api.RemoteAddress("9.9.9.9"):
			return &mockClient{
				etcdReturn: []db.Etcd{
					{
						LeaderIP: "leader-priv",
					},
				},
			}, nil
		// The leader. getLeaderClient() should return an instance of this.
		case api.RemoteAddress("leader"):
			return passedClient, nil
		default:
			t.Errorf("Unexpected call to getClient with host %s", host)
			t.Fail()
		}
		panic("unreached")
	}

	localClient := &mockClient{
		machineReturn: []db.Machine{
			{
				PublicIP: "8.8.8.8",
			},
			{
				PublicIP: "9.9.9.9",
			},
			{
				PublicIP:  "leader",
				PrivateIP: "leader-priv",
			},
		},
	}
	res, err := getLeaderClient(localClient)
	if err != nil {
		t.Errorf("Unexpected error when getting lead minion: %s", err.Error())
		return
	}

	if res != passedClient {
		t.Errorf("Didn't retrieve the proper client for the lead minion: "+
			"expected %v, got %v", passedClient, res)
	}
}

func TestNoLeader(t *testing.T) {
	getClient = func(host string) (client.Client, error) {
		// No client knows the leader IP.
		return &mockClient{
			etcdReturn: []db.Etcd{
				{
					LeaderIP: "",
				},
			},
		}, nil
	}

	localClient := &mockClient{
		machineReturn: []db.Machine{
			{
				PublicIP: "8.8.8.8",
			},
			{
				PublicIP: "9.9.9.9",
			},
		},
	}
	_, err := getLeaderClient(localClient)
	expErr := "no leader found"
	if err == nil {
		t.Errorf("Expected an error when the leader IP is not set.")
		return
	}

	if err.Error() != expErr {
		t.Errorf("Got wrong error: expected %s, got %s", expErr, err.Error())
	}
}
