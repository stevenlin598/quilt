package etcd

import (
	"encoding/json"
	"errors"
	"net"
	"path"
	"time"

	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/join"
	"github.com/NetSys/quilt/minion/ip"
	"github.com/NetSys/quilt/util"

	log "github.com/Sirupsen/logrus"
)

const (
	minionTimeout = 30
	subnetStore   = minionDir + "/subnets"
	selfNode      = "self"
)

var (
	// Store in variables so we can change them for unit tests
	subnetAttempts = 1000
	subnetTTL      = 5 * time.Minute
	sleep          = time.Sleep
)

func runMinionSync(conn db.Conn, store Store) {
	loopLog := util.NewEventTimer("Etcd")
	minion := getMinion(conn)
	go syncSubnet(conn, store, minion)
	for range conn.TriggerTick(minionTimeout/2, db.MinionTable).C {
		loopLog.LogStart()
		writeMinion(conn, store)
		readMinion(conn, store)
		loopLog.LogEnd()
	}
}

func getMinion(conn db.Conn) db.Minion {
	var minion db.Minion
	var err error
	for {
		minion, err = conn.MinionSelf()
		if err != nil {
			log.WithError(err).Error("Failed to get self")
		} else if minion.PrivateIP == "" {
			log.Error("Self has no PrivateIP")
		} else {
			break
		}
		time.Sleep(time.Second)
	}
	return minion
}

func readMinion(conn db.Conn, store Store) {
	tree, err := store.GetTree(nodeStore)
	if err != nil {
		log.WithError(err).Warning("Failed to get minions from Etcd.")
		return
	}

	var storeMinions []db.Minion
	for _, t := range tree.Children {
		var m db.Minion
		minion := t.Children[selfNode].Value
		if err := json.Unmarshal([]byte(minion), &m); err != nil {
			log.WithField("json", minion).Warning("Failed to parse Minion.")
			continue
		}
		storeMinions = append(storeMinions, m)
	}

	conn.Transact(func(view db.Database) error {
		dbms, sms := filterSelf(view.SelectFromMinion(nil), storeMinions)
		del, add := diffMinion(dbms, sms)

		for _, m := range del {
			view.Remove(m)
		}

		for _, m := range add {
			minion := view.InsertMinion()
			id := minion.ID
			minion = m
			minion.ID = id
			view.Commit(minion)
		}
		return nil
	})
}

func filterSelf(dbMinions, storeMinions []db.Minion) ([]db.Minion, []db.Minion) {
	var self db.Minion
	var sms, dbms []db.Minion

	for _, dbm := range dbMinions {
		if dbm.Self {
			self = dbm
		} else {
			dbms = append(dbms, dbm)
		}
	}

	for _, m := range storeMinions {
		if self.PrivateIP != m.PrivateIP {
			sms = append(sms, m)
		}
	}

	return dbms, sms
}

func diffMinion(dbMinions, storeMinions []db.Minion) (del, add []db.Minion) {
	key := func(iface interface{}) interface{} {
		m := iface.(db.Minion)
		m.ID = 0
		m.Spec = ""
		m.Self = false
		return m
	}

	_, lefts, rights := join.HashJoin(db.MinionSlice(dbMinions),
		db.MinionSlice(storeMinions), key, key)

	for _, left := range lefts {
		del = append(del, left.(db.Minion))
	}

	for _, right := range rights {
		add = append(add, right.(db.Minion))
	}

	return
}

func writeMinion(conn db.Conn, store Store) {
	minion, err := conn.MinionSelf()
	if err != nil {
		return
	}

	if minion.PrivateIP == "" {
		return
	}

	js, err := json.Marshal(minion)
	if err != nil {
		panic("Failed to convert Minion to JSON")
	}

	dir := path.Join(nodeStore, minion.PrivateIP)
	if err := createEtcdDir(dir, store, minionTimeout*time.Second); err != nil {
		log.Warning("Failed to create minion directory")
		return
	}

	key := path.Join(dir, selfNode)
	if err := store.Set(key, string(js), minionTimeout*time.Second); err != nil {
		log.Warning("Failed to update minion node in Etcd: %s", err)
	}
}

func generateSubnet(store Store, minion db.Minion) (string, error) {
	for i := 0; i < subnetAttempts; i++ {
		subnet := randomMinionSubnet()
		err := store.Create(subnetKey(subnet), minion.PrivateIP, subnetTTL)
		if err == nil {
			return subnet, nil
		}
		log.WithError(err).WithField("subnet",
			subnet).Debug("Subnet taken, trying again.")
	}

	return "", errors.New("failed to allocate subnet")
}

func updateSubnet(conn db.Conn, store Store, minion db.Minion) db.Minion {
	err := store.Refresh(subnetKey(minion.Subnet), minion.PrivateIP,
		subnetTTL)
	if err == nil {
		return minion
	}
	log.WithError(err).Infof("Failed to refresh subnet '%s', "+
		"generating a new one.", minion.Subnet)

	// Invalidate the subnet until we get a new one.
	minion = setMinionSubnet(conn, "")

	// If we failed to refresh, someone took our subnet or we never had one.
	for {
		minion.Subnet, err = generateSubnet(store, minion)
		if err == nil {
			break
		}

		log.WithError(err).Warn("Failed to allocate subnet, " +
			"trying again")
		sleep(time.Second)
	}

	return setMinionSubnet(conn, minion.Subnet)
}

func setMinionSubnet(conn db.Conn, subnet string) db.Minion {
	var err error
	var minion db.Minion
	conn.Transact(func(view db.Database) error {
		minion, err = view.MinionSelf()
		if err != nil {
			log.WithError(err).Error("Failed to get self")
			return err
		}
		minion.Subnet = subnet
		view.Commit(minion)
		return nil
	})

	return minion
}

func syncSubnet(conn db.Conn, store Store, minion db.Minion) {
	for {
		minion = updateSubnet(conn, store, minion)
		time.Sleep(subnetTTL / 4)
	}
}

func randomMinionSubnet() string {
	submask := ip.ToInt(net.IP(ip.SubMask))
	quiltmask := ip.ToInt(net.IP(ip.QuiltMask))
	subnetBits := submask ^ quiltmask

	// Reserve the 0 subnet for labels
	randomSubnet := uint32(0)
	for randomSubnet == 0 {
		randomSubnet = ip.Rand32() & subnetBits
	}

	subnet := ip.ToInt(ip.QuiltPrefix) | randomSubnet
	return ip.FromInt(subnet).String()
}

func subnetKey(subnet string) string {
	return path.Join(subnetStore, subnet)
}
