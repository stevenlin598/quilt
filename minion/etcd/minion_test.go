package etcd

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"path"
	"reflect"
	"testing"
	"time"

	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/minion/ip"

	"github.com/davecgh/go-spew/spew"
)

func TestWriteMinion(t *testing.T) {
	t.Parallel()

	ip := "1.2.3.4"
	key := "/minion/nodes/" + ip + "/" + selfNode

	conn := db.New()
	store := NewMock()

	writeMinion(conn, store)
	val, err := store.Get(key)
	if err == nil {
		t.Errorf("Expected \"\", got %s", val)
	}

	// Minion without a PrivateIP
	conn.Transact(func(view db.Database) error {
		m := view.InsertMinion()
		m.Self = true
		m.Role = db.Master
		m.Provider = "Amazon"
		m.Size = "Big"
		m.Region = "Somewhere"
		view.Commit(m)
		return nil
	})

	writeMinion(conn, store)
	val, err = store.Get(key)
	if err == nil {
		t.Errorf("Expected \"\", got %s", val)
	}

	conn.Transact(func(view db.Database) error {
		m, _ := view.MinionSelf()
		m.PrivateIP = ip
		view.Commit(m)
		return nil
	})
	writeMinion(conn, store)
	val, err = store.Get(key)
	if err != nil {
		t.Errorf("Error %s", err)
	}

	expVal := `{"Role":"Master","PrivateIP":"1.2.3.4",` +
		`"Provider":"Amazon","Size":"Big","Region":"Somewhere"}`
	if val != expVal {
		t.Errorf("Got %s\n\tExp %s", val, expVal)
	}
}

func TestReadMinion(t *testing.T) {
	t.Parallel()

	conn := db.New()
	store := NewMock()

	m := randMinion()
	js, _ := json.Marshal(m)
	store.Set(nodeStore+"/foo/"+selfNode, string(js), 0)

	readMinion(conn, store)
	minions := conn.SelectFromMinion(nil)
	if len(minions) != 1 {
		t.Error(spew.Sprintf("Wrong number of minions: %s", minions))
	}

	minions[0].ID = m.ID
	if !reflect.DeepEqual(minions[0], m) {
		t.Error(spew.Sprintf("Incorrect DB Minion: %s", minions))
	}

	store = NewMock()
	store.Mkdir(nodeStore, 0)
	readMinion(conn, store)
	minions = conn.SelectFromMinion(nil)
	if len(minions) > 0 {
		t.Error(spew.Sprintf("Expected zero minions, found: %s", minions))
	}
}

func TestReadDiff(t *testing.T) {
	t.Parallel()

	add, del := diffMinion(nil, nil)
	if len(add) > 0 {
		t.Error(spew.Sprintf("Expected no additions, found: %s", add))
	}

	if len(del) > 0 {
		t.Error(spew.Sprintf("Expected no deletions, found: %s", del))
	}

	sharedEtcd := randMinion()
	sharedDbm := sharedEtcd
	sharedDbm.Spec = "Spec"
	sharedDbm.ID = 3

	etcd := []db.Minion{randMinion()}
	dbms := []db.Minion{dbMinion()}

	del, add = diffMinion(append(dbms, sharedDbm), append(etcd, sharedEtcd))

	if !reflect.DeepEqual(del, dbms) {
		t.Error(spew.Sprintf("Diff Deletion Found:\n\t%s\nExpected:\n\t%s",
			del, dbms))
	}

	if !reflect.DeepEqual(add, etcd) {
		t.Error(spew.Sprintf("Diff Addition Found:\n\t%s\nExpected:\n\t%s",
			add, etcd))
	}
}

func TestFilter(t *testing.T) {
	newDB, newEtcd := filterSelf(nil, nil)
	if len(newDB) > 0 || len(newEtcd) > 0 {
		t.Error(spew.Sprintf("Filter change unexpected: %s, %s", newDB, newEtcd))
	}

	dbms := []db.Minion{dbMinion(), dbMinion()}
	etcd := []db.Minion{randMinion(), randMinion()}

	newDB, newEtcd = filterSelf(dbms, etcd)
	if !reflect.DeepEqual(dbms, newDB) {
		t.Error(spew.Sprintf("No Self DB Found:\n\t%s\nExpected:\n\t%s",
			newDB, dbms))
	}
	if !reflect.DeepEqual(etcd, newEtcd) {
		t.Error(spew.Sprintf("No Self Etcd Found:\n\t%s\nExpected:\n\t%s",
			newEtcd, etcd))
	}

	self := randMinion()
	selfEtcd := self
	self.Self = true

	newDB, newEtcd = filterSelf(append(dbms, self), append(etcd, selfEtcd))
	if !reflect.DeepEqual(dbms, newDB) {
		t.Error(spew.Sprintf("No Self DB Found:\n\t%s\nExpected:\n\t%s",
			newDB, dbms))
	}
	if !reflect.DeepEqual(etcd, newEtcd) {
		t.Error(spew.Sprintf("No Self Etcd Found:\n\t%s\nExpected:\n\t%s",
			newEtcd, etcd))
	}
}

// Test that GenerateSubnet generates valid subnets, and passively test that it can
// generate enough unique subnets by generating 3500 out of the possible 4095
func TestGenerateSubnet(t *testing.T) {
	store := newTestMock()
	subnetAttempts = 5000 // big so we don't error unless something is wrong
	defer func() {
		subnetAttempts = 1000
	}()
	minionMaskBits, _ := ip.SubMask.Size()
	emptyBits := uint32(0xffffffff >> uint(minionMaskBits))
	for i := 0; i < 3500; i++ {
		subnetStr, err := generateSubnet(store, db.Minion{PrivateIP: "10.1.0.1"})
		if err != nil {
			t.Fatal("Ran out of attempts to generate subnet")
		}

		subnet := ip.Parse(subnetStr, ip.QuiltPrefix, ip.QuiltMask)
		if subnet.Equal(ip.LabelPrefix) {
			t.Fatal("Generated the label subnet")
		}

		if !subnet.Mask(ip.QuiltMask).Equal(ip.QuiltPrefix) {
			t.Fatal("Generated subnet is not within private subnet")
		}

		if !subnet.Mask(ip.SubMask).Equal(subnet) {
			t.Fatal("Generated subnet is not with its own CIDR subnet")
		}

		subnetInt := binary.BigEndian.Uint32(subnet.To4())
		if subnetInt&emptyBits != 0 {
			t.Fatal("Generated subnet uses too many bits")
		}
	}
}

func TestUpdateSubnet(t *testing.T) {
	store := newTestMock()
	conn := db.New()

	subnetTTL = time.Second
	defer func() {
		subnetTTL = 5 * time.Minute
	}()

	nextRand := uint32(0)
	subnetStart, _ := ip.SubMask.Size()
	ip.Rand32 = func() uint32 {
		ret := nextRand
		nextRand++
		return ret << (32 - uint(subnetStart)) // increment inside the subnet
	}

	defer func() {
		ip.Rand32 = rand.Uint32
	}()

	sleep = func(sleepTime time.Duration) {}
	defer func() {
		sleep = time.Sleep
	}()

	var m db.Minion
	conn.Transact(func(view db.Database) error {
		m = view.InsertMinion()
		m.PrivateIP = "1.2.3.4"
		m.Role = db.Worker
		m.Self = true
		view.Commit(m)
		return nil
	})

	timeUpdateSubnet := func() chan struct{} {
		out := make(chan struct{})
		go func() {
			m = updateSubnet(conn, store, m)
			out <- struct{}{}
			close(out)
		}()
		return out
	}

	done := timeUpdateSubnet()
	timer := time.After(100 * time.Millisecond)
	select {
	case <-timer:
		t.Fatal("Timed out syncing subnet")
	case <-done:
		break
	}

	firstSubnet := net.IPv4(10, 0, 16, 0).String()
	secondSubnet := net.IPv4(10, 0, 32, 0).String()
	thirdSubnet := net.IPv4(10, 0, 48, 0).String()

	minion, err := store.Get(path.Join(subnetStore, firstSubnet))
	if err != nil {
		t.Fatalf("Expected subnet %s/20 to be claimed, none found", firstSubnet)
	}

	if minion != "1.2.3.4" {
		t.Fatalf("Wrong minion owns subnet %s/20", firstSubnet)
	}

	err = store.Set(path.Join(subnetStore, firstSubnet), "1.2.3.5", time.Minute)
	if err != nil {
		t.Fatal("Could not overwrite store value")
	}

	done = timeUpdateSubnet()
	timer = time.After(100 * time.Millisecond)
	select {
	case <-timer:
		t.Fatal("Timed out syncing subnet")
	case <-done:
		break
	}

	minion, err = store.Get(path.Join(subnetStore, firstSubnet))
	if err != nil {
		t.Fatalf("Expected subnet %s/20 to be claimed, none found", firstSubnet)
	}

	if minion != "1.2.3.5" {
		t.Fatal("Minion 1.2.3.4 reclaimed a subnet it shouldn't have")
	}

	minion, err = store.Get(path.Join(subnetStore, secondSubnet))
	if err != nil {
		t.Fatalf("Expected subnet %s/20 to be claimed, none found", secondSubnet)
	}

	if minion != "1.2.3.4" {
		t.Fatalf("Wrong minion owns subnet %s/20", secondSubnet)
	}

	store.advanceTime(2 * time.Second)

	done = timeUpdateSubnet()
	timer = time.After(100 * time.Millisecond)
	select {
	case <-timer:
		t.Fatal("Timed out syncing subnet")
	case <-done:
		break
	}

	_, err = store.Get(path.Join(subnetStore, secondSubnet))
	if err == nil {
		t.Fatalf("Expected subnet %s/20 to be expired", secondSubnet)
	}

	minion, err = store.Get(path.Join(subnetStore, thirdSubnet))
	if err != nil {
		t.Fatalf("Expected subnet %s/20 to be claimed, none found", thirdSubnet)
	}

	if minion != "1.2.3.4" {
		t.Fatalf("Wrong minion owns subnet %s/20", thirdSubnet)
	}
}

func randMinion() db.Minion {
	return db.Minion{
		Role:      "Worker",
		PrivateIP: randStr(),
		Provider:  randStr(),
		Size:      randStr(),
		Region:    randStr(),
	}
}

func dbMinion() db.Minion {
	dbm := randMinion()
	dbm.Spec = randStr()
	dbm.ID = rand.Int()
	return dbm
}

func randStr() string {
	return fmt.Sprintf("%d-%d-%d-%d", rand.Int(), rand.Int(), rand.Int(), rand.Int())
}
