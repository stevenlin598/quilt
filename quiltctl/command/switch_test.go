package command

import (
	"errors"
	"testing"

	clientMock "github.com/quilt/quilt/api/client/mocks"
	"github.com/quilt/quilt/cluster/machine"
	"github.com/quilt/quilt/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestRetrieveSpec(t *testing.T) {
	t.Parallel()

	mockGetter := new(clientMock.Getter)
	c := &clientMock.Client{}
	mockGetter.On("Client", mock.Anything).Return(c, nil)

	c.MinionReturn = []db.Minion{
		{
			Spec: `testSpec`,
		},
	}

	sCmd := NewSwitchCommand()
	sCmd.clientGetter = mockGetter

	// testMachines must be non-empty for switch to attempt retrieving spec
	testMachines := []machine.Machine{{}}
	machineGetter = func(namespace string) ([]machine.Machine, error) {
		return testMachines, nil
	}

	spec, err := sCmd.getSpec(testMachines)
	assert.NoError(t, err)
	assert.Equal(t, spec, `testSpec`)

	exitCode := sCmd.Run()
	assert.Equal(t, exitCode, 0)
	assert.Equal(t, c.DeployArg, `testSpec`)
}

func TestNoMachines(t *testing.T) {
	t.Parallel()

	mockGetter := new(clientMock.Getter)
	c := &clientMock.Client{}
	mockGetter.On("Client", mock.Anything).Return(c, nil)

	sCmd := NewSwitchCommand()
	sCmd.clientGetter = mockGetter
	testMachines := []machine.Machine{}
	machineGetter = func(namespace string) ([]machine.Machine, error) {
		return testMachines, nil
	}

	exitCode := sCmd.Run()
	assert.Equal(t, exitCode, 1)

	spec, err := sCmd.getSpec(testMachines)
	assert.Equal(t, spec, "")
	assert.Error(t, err, "none of the machines have a valid spec")
}

func TestBadClient(t *testing.T) {
	t.Parallel()

	mockGetter := new(clientMock.Getter)
	c := &clientMock.Client{}
	mockGetter.On("Client", mock.Anything).Return(c,
		errors.New("error getting client"))

	sCmd := NewSwitchCommand()
	sCmd.clientGetter = mockGetter
	testMachines := []machine.Machine{{}}
	machineGetter = func(namespace string) ([]machine.Machine, error) {
		return testMachines, nil
	}

	exitCode := sCmd.Run()
	assert.Equal(t, exitCode, 1)
}

func TestParse(t *testing.T) {
	t.Parallel()
	checkSwitchParsing(t, []string{}, "", errors.New("no namespace specified"))
	checkSwitchParsing(t, []string{"someNamespace"}, "someNamespace", nil)
}

func checkSwitchParsing(t *testing.T, args []string, expNamespace string, expErr error) {
	sCmd := NewSwitchCommand()
	err := parseHelper(sCmd, args)

	assert.Equal(t, expErr, err)
	assert.Equal(t, expNamespace, sCmd.namespace)
}
