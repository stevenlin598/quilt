package etcd

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"path"
	"reflect"
	"strconv"
	"testing"

	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/minion/ip"

	"github.com/davecgh/go-spew/spew"
)

func TestUpdateEtcdContainers(t *testing.T) {
	store := newTestMock()
	store.Mkdir(minionDir, 0)
	conn := db.New()
	var containers []db.Container
	idIPMap := map[string]string{}
	conn.Transact(func(view db.Database) error {
		for i := 2; i < 5; i++ {
			c := view.InsertContainer()
			si := strconv.Itoa(i)
			c.StitchID = i
			c.IP = fmt.Sprintf("10.0.0.%s", si)
			c.Command = []string{"echo", si}
			c.Labels = []string{"red", "blue"}
			c.Env = map[string]string{"i": si}
			view.Commit(c)
			idIPMap[si] = c.IP
		}
		containers = view.SelectFromContainer(nil)
		return nil
	})

	cs := []storeContainer{}
	for i := 2; i < 5; i++ {
		si := strconv.Itoa(i)
		storeTemp := storeContainer{
			StitchID: i,
			Command:  []string{"echo", si},
			Labels:   []string{"red", "blue"},
			Env:      map[string]string{"i": si},
		}
		cs = append(cs, storeTemp)
	}

	testContainers, _ := json.Marshal(cs)
	err := store.Set(containerStore, string(testContainers), 0)
	if err != nil {
		t.Fatal("Failed to write to store.")
	}

	*store.writes = 0
	etcdData, _ := readEtcd(store)
	etcdData, _ = updateEtcdContainer(store, etcdData, containers)

	resultContainers, err := store.Get(containerStore)
	if err != nil {
		t.Fatal("Failed to read from store")
	}

	resultSlice := []storeContainer{}
	json.Unmarshal([]byte(resultContainers), &resultSlice)

	if !eq(cs, resultSlice) {
		t.Error(spew.Sprintf("Unexpected change to etcd store. Got %v,"+
			" expected %v.", resultSlice, cs))
	}

	// etcd and the db agree, there should be no writes
	if *store.writes != 0 {
		t.Error(spew.Sprintf("Unexpected writes (%d) to etcd store.",
			*store.writes))
	}

	// simulate etcd having out of date information, except the IP
	badEtcdSlice := []storeContainer{}
	for i := 2; i < 6; i++ {
		si := strconv.Itoa(i)
		badEtcd := storeContainer{
			StitchID: i,
			Command:  []string{"echo", si},
			Labels:   []string{"red", "blue"},
			Env:      map[string]string{"i": si},
		}
		badEtcdSlice = append(badEtcdSlice, badEtcd)
	}

	// add a new container with a bad ip to test ip syncing
	badEtcdSlice = append(badEtcdSlice, storeContainer{StitchID: 8})
	cs = append(cs, storeContainer{StitchID: 8})
	conn.Transact(func(view db.Database) error {
		c := view.InsertContainer()
		c.StitchID = 8
		c.IP = "10.0.0.0"
		view.Commit(c)
		containers = append([]db.Container{c}, containers...)
		return nil
	})
	idIPMap["8"] = "10.0.0.0"
	jsonIPMap, _ := json.Marshal(idIPMap)
	minionDirKey := path.Join(nodeStore, "testMinion")
	store.Set(path.Join(minionDirKey, minionIPStore), string(jsonIPMap), 0)

	badTestContainers, _ := json.Marshal(badEtcdSlice)
	err = store.Set(containerStore, string(badTestContainers), 0)
	if err != nil {
		t.Fatalf("Failed to write to store.")
	}

	*store.writes = 0
	etcdData, _ = readEtcd(store)
	etcdData, _ = updateEtcdContainer(store, etcdData, containers)

	resultContainers, err = store.Get(containerStore)
	if err != nil {
		t.Fatal("Failed to read from store.")
	}

	resultSlice = []storeContainer{}
	json.Unmarshal([]byte(resultContainers), &resultSlice)

	if !eq(cs, resultSlice) {
		t.Error(spew.Sprintf("Expected change to etcd store.\nGot      %v"+
			"\nExpected %v.", resultSlice, cs))
	}

	if !eq(cs, etcdData.containers) {
		t.Error("updateEtcdContainer did change the storeData struct")
	}

	// if etcd and the db don't agree, there should be exactly 1 write
	if *store.writes != 1 {
		t.Error(spew.Sprintf("Expected a single write to etcd. Found %d.",
			*store.writes))
	}
}

func TestUpdateEtcdLabel(t *testing.T) {
	store := newTestMock()
	store.Mkdir(minionDir, 0)
	conn := db.New()
	var containers []db.Container
	conn.Transact(func(view db.Database) error {
		for i := 2; i < 5; i++ {
			c := view.InsertContainer()
			si := strconv.Itoa(i)
			c.DockerID = si
			c.Labels = []string{si}
			view.Commit(c)
		}
		containers = view.SelectFromContainer(nil)
		return nil
	})

	labelStruct := map[string]string{}
	testLabel, _ := json.Marshal(labelStruct)
	err := store.Set(labelToIPStore, string(testLabel), 0)
	if err != nil {
		t.Fatal("Failed to write to store.")
	}

	*store.writes = 0
	etcdData, _ := readEtcd(store)
	etcdData, _ = updateEtcdLabel(store, etcdData, containers)

	resultLabels, err := store.Get(labelToIPStore)
	if err != nil {
		t.Fatal("Failed to read from store")
	}

	resultStruct := map[string]string{}
	json.Unmarshal([]byte(resultLabels), &resultStruct)

	if !eq(labelStruct, resultStruct) {
		t.Error(spew.Sprintf("Unexpected change to etcd store. Got %v,"+
			" expected %v.", resultStruct, labelStruct))
	}

	// etcd and the db agree, there should be no writes
	if *store.writes != 0 {
		t.Error(spew.Sprintf("Unexpected writes (%d) to etcd store.",
			*store.writes))
	}

	conn.Transact(func(view db.Database) error {
		c := view.InsertContainer()
		c.DockerID = "6"
		c.Labels = []string{"2", "3"}
		view.Commit(c)
		containers = append(containers, c)
		return nil
	})

	// Label 2 is now multhost, so if etcd knows that, it should get etcd's ip
	labelStruct["2"] = "10.0.0.11"
	testLabel, _ = json.Marshal(labelStruct)
	err = store.Set(labelToIPStore, string(testLabel), 0)
	if err != nil {
		t.Fatal("Failed to write to store.")
	}

	// the dockerIP map still has label 3's IP, but label 3 is now multihost, so it
	// should get a new IP
	labelStruct["3"] = "10.0.0.0"

	*store.writes = 0
	etcdData, _ = readEtcd(store)
	nextRand := uint32(0)
	ip.Rand32 = func() uint32 {
		ret := nextRand
		nextRand++
		return ret
	}

	defer func() {
		ip.Rand32 = rand.Uint32
	}()

	etcdData, _ = updateEtcdLabel(store, etcdData, containers)

	resultLabels, err = store.Get(labelToIPStore)
	if err != nil {
		t.Fatal("Failed to read from store.")
	}

	resultStruct = map[string]string{}
	json.Unmarshal([]byte(resultLabels), &resultStruct)

	if !eq(labelStruct, resultStruct) {
		t.Error(spew.Sprintf("Expected change to etcd store. Got %v, expected "+
			"%v.", resultStruct, labelStruct))
	}

	if !eq(labelStruct, etcdData.multiHost) {
		t.Error("updateEtcdLabel did not change storeData struct")
	}

	if *store.writes != 1 {
		t.Error(spew.Sprintf("Unexpected writes (%d) to etcd store.",
			*store.writes))
	}
}

func TestUpdateLeaderDBC(t *testing.T) {
	conn := db.New()
	conn.Transact(func(view db.Database) error {
		dbc := view.InsertContainer()
		dbc.StitchID = 1
		view.Commit(dbc)

		updateLeaderDBC(view, view.SelectFromContainer(nil), storeData{
			containers: []storeContainer{{StitchID: 1}},
		}, map[string]string{"1": "foo"})

		dbcs := view.SelectFromContainer(nil)
		if len(dbcs) != 1 || dbcs[0].StitchID != 1 || dbcs[0].IP != "foo" ||
			dbcs[0].Mac != "" {
			t.Error(spew.Sprintf("Unexpected dbc: %v", dbc))
		}

		return nil
	})
}

func TestUpdateWorkerDBC(t *testing.T) {
	conn := db.New()
	conn.Transact(func(view db.Database) error {
		testUpdateWorkerDBC(t, view)
		return nil
	})
}

func testUpdateWorkerDBC(t *testing.T, view db.Database) {
	minion := view.InsertMinion()
	minion.Role = db.Worker
	minion.Self = true
	view.Commit(minion)

	for id := 1; id < 4; id++ {
		container := view.InsertContainer()
		container.StitchID = id
		view.Commit(container)
	}

	container := view.InsertContainer()
	container.StitchID = 3
	container.IP = "junk"
	view.Commit(container)

	cs := storeContainerSlice{
		{
			StitchID: 1,
			Command:  []string{"echo", "hi"},
			Labels:   []string{"red", "blue"},
			Env:      map[string]string{"GOPATH": "~/gocode"},
			Minion:   "1.2.3.4",
		}, {
			StitchID: 2,
			Command:  []string{"echo", "bye"},
			Labels:   []string{"blue", "green"},
			Env:      map[string]string{"GOPATH": "~"},
			Minion:   "1.2.3.4",
		}, {
			StitchID: 3,
			Command:  []string{"echo", "bye"},
			Labels:   []string{"blue", "green"},
			Env:      map[string]string{"GOPATH": "~"},
			Minion:   "1.2.3.5",
		},
	}

	nextRand := uint32(0)
	ip.Rand32 = func() uint32 {
		ret := nextRand
		nextRand++
		return ret
	}

	defer func() {
		ip.Rand32 = rand.Uint32
	}()

	store := newTestMock()

	jsonCS, _ := json.Marshal(cs)
	err := store.Set(containerStore, string(jsonCS), 0)
	if err != nil {
		t.Fatalf("Failed to write to store.")
	}

	jsonNull, _ := json.Marshal(map[string]string{})
	minionDirKey := path.Join(nodeStore, "1.2.3.4")
	err = store.Set(path.Join(minionDirKey, minionIPStore), string(jsonNull), 0)
	if err != nil {
		t.Fatalf("Failed to write to store.")
	}

	updateWorker(view, db.Minion{PrivateIP: "1.2.3.4",
		Subnet: "10.1.0.0"}, store, storeData{containers: cs})

	ipMap := map[int]string{}
	labelMap := map[int][]string{}
	commandMap := map[string][]string{}
	envMap := map[string]map[string]string{}
	for _, c := range view.SelectFromContainer(nil) {
		ipMap[c.StitchID] = c.IP
		labelMap[c.StitchID] = c.Labels
		commandMap[c.DockerID] = c.Command
		envMap[c.DockerID] = c.Env
	}

	expIPMap1 := map[int]string{
		1: "10.1.0.0",
		2: "10.1.0.1",
	}

	expIPMap2 := map[int]string{
		1: expIPMap1[2],
		2: expIPMap1[1],
	}
	if !eq(expIPMap1, ipMap) && !eq(expIPMap2, ipMap) {
		t.Error(spew.Sprintf("\nGot: %v\nExp: %v or %v\n",
			ipMap, expIPMap1, expIPMap2))
	}

	resultMap := map[string]string{}
	storeIPs, _ := store.Get(path.Join(minionDirKey, minionIPStore))
	json.Unmarshal([]byte(storeIPs), &resultMap)

	for id, ip := range resultMap {
		sid, _ := strconv.Atoi(id)
		if otherIP, ok := ipMap[sid]; !ok || ip != otherIP {
			t.Fatalf("IPs did not match: %s vs %s", ip, otherIP)
		}
	}

	expLabelMap := map[int][]string{
		1: {"red", "blue"},
		2: {"blue", "green"},
	}

	if !eq(labelMap, expLabelMap) {
		t.Error(spew.Sprintf("\nGot: %v\nExp: %v\n", labelMap, expLabelMap))
	}
}

func TestContainerJoinScore(t *testing.T) {
	t.Parallel()

	a := storeContainer{
		Minion:   "Minion",
		Image:    "Image",
		StitchID: 1,
	}
	b := a

	score := containerJoinScore(a, b)
	if score != 0 {
		t.Errorf("Unexpected score: %d", score)
	}

	b.Image = "Wrong"
	score = containerJoinScore(a, b)
	if score != -1 {
		t.Errorf("Unexpected score: %d", score)
	}
	b = a
}

func TestUpdateDBLabels(t *testing.T) {
	conn := db.New()
	conn.Transact(func(view db.Database) error {
		testUpdateDBLabels(t, view)
		return nil
	})
}

func testUpdateDBLabels(t *testing.T, view db.Database) {
	labelStruct := map[string]string{"a": "10.0.0.2"}
	ipMap := map[string]string{"1": "10.0.0.3", "2": "10.0.0.4"}
	containerSlice := []storeContainer{
		{
			StitchID: 1,
			Labels:   []string{"a", "b"},
		},
		{
			StitchID: 2,
			Labels:   []string{"a"},
		},
	}

	updateDBLabels(view, storeData{
		containers: containerSlice,
		multiHost:  labelStruct,
	}, ipMap)

	type labelIPs struct {
		labelIP      string
		containerIPs []string
	}
	lip := map[string]labelIPs{}
	for _, l := range view.SelectFromLabel(nil) {
		if _, ok := lip[l.Label]; ok {
			t.Errorf("Duplicate labels in the DB: %s", l.Label)
		}
		lip[l.Label] = labelIPs{
			labelIP:      l.IP,
			containerIPs: l.ContainerIPs,
		}
	}

	resultLabels := map[string]labelIPs{
		"a": {
			labelIP:      "10.0.0.2",
			containerIPs: []string{"10.0.0.3", "10.0.0.4"},
		},
		"b": {
			labelIP:      "10.0.0.3",
			containerIPs: []string{"10.0.0.3"},
		},
	}

	if !eq(lip, resultLabels) {
		t.Error(spew.Sprintf("Found: %v\nExpected: %v\n", lip, labelStruct))
	}
}

func eq(a, b interface{}) bool {
	return reflect.DeepEqual(a, b)
}
