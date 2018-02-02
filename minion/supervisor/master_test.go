package supervisor

import (
	"fmt"
	"testing"

	cliPath "github.com/kelda/kelda/cli/path"
	"github.com/kelda/kelda/db"
	"github.com/kelda/kelda/util"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

func TestMaster(t *testing.T) {
	util.AppFs = afero.NewMemMapFs()
	ctx := initTest()
	ip := "1.2.3.4"
	etcdIPs := []string{""}
	ctx.conn.Txn(db.AllTables...).Run(func(view db.Database) error {
		m := view.MinionSelf()
		e := view.SelectFromEtcd(nil)[0]
		m.Role = db.Master
		m.PrivateIP = ip
		e.EtcdIPs = etcdIPs
		view.Commit(m)
		view.Commit(e)
		return nil
	})
	runMasterOnce()

	exp := map[string][]string{
		EtcdName:     etcdArgsMaster(ip, etcdIPs),
		OvsdbName:    {"ovsdb-server"},
		RegistryName: nil,
	}
	assert.Equal(t, exp, ctx.fd.running())
	assert.Empty(t, ctx.execs)

	/* Change IP, etcd IPs, and become the leader. */
	ip = "8.8.8.8"
	etcdIPs = []string{"8.8.8.8"}
	ctx.conn.Txn(db.AllTables...).Run(func(view db.Database) error {
		m := view.MinionSelf()
		e := view.SelectFromEtcd(nil)[0]
		m.Role = db.Master
		m.PrivateIP = ip
		e.EtcdIPs = etcdIPs
		e.Leader = true
		view.Commit(m)
		view.Commit(e)
		return nil
	})
	runMasterOnce()

	exp = map[string][]string{
		EtcdName:      etcdArgsMaster(ip, etcdIPs),
		OvsdbName:     {"ovsdb-server"},
		OvnnorthdName: {"ovn-northd"},
		RegistryName:  nil,
	}
	assert.Equal(t, exp, ctx.fd.running())
	assert.Empty(t, ctx.execs)

	/* Get the etcd encryption key. */
	util.WriteFile(cliPath.MinionKubeSecretPath, []byte("secret"), 0644)
	runMasterOnce()
	exp[KubeAPIServerName] = kubeAPIServerArgs(ip, etcdIPs)
	exp[KubeControllerManagerName] = kubeControllerManagerArgs()
	exp[KubeSchedulerName] = kubeSchedulerArgs()
	assert.Equal(t, exp, ctx.fd.running())

	/* Lose leadership. */
	ctx.conn.Txn(db.AllTables...).Run(func(view db.Database) error {
		e := view.SelectFromEtcd(nil)[0]
		e.Leader = false
		view.Commit(e)
		return nil
	})
	runMasterOnce()

	delete(exp, OvnnorthdName)
	assert.Equal(t, exp, ctx.fd.running())
	assert.Empty(t, ctx.execs)
}

func TestEtcdAdd(t *testing.T) {
	ctx := initTest()
	ip := "1.2.3.4"
	etcdIPs := []string{ip, "5.6.7.8"}
	ctx.conn.Txn(db.AllTables...).Run(func(view db.Database) error {
		m := view.MinionSelf()
		e := view.SelectFromEtcd(nil)[0]
		m.Role = db.Master
		m.PrivateIP = ip
		e.EtcdIPs = etcdIPs
		view.Commit(m)
		view.Commit(e)
		return nil
	})
	runMasterOnce()

	exp := map[string][]string{
		EtcdName:                  etcdArgsMaster(ip, etcdIPs),
		OvsdbName:                 {"ovsdb-server"},
		RegistryName:              nil,
		KubeAPIServerName:         kubeAPIServerArgs(ip, etcdIPs),
		KubeControllerManagerName: kubeControllerManagerArgs(),
		KubeSchedulerName:         kubeSchedulerArgs(),
	}
	assert.Equal(t, exp, ctx.fd.running())

	// Add a new master
	etcdIPs = append(etcdIPs, "9.10.11.12")
	ctx.conn.Txn(db.AllTables...).Run(func(view db.Database) error {
		m := view.MinionSelf()
		e := view.SelectFromEtcd(nil)[0]
		m.Role = db.Master
		e.EtcdIPs = etcdIPs
		view.Commit(m)
		view.Commit(e)
		return nil
	})
	runMasterOnce()

	exp[EtcdName] = etcdArgsMaster(ip, etcdIPs)
	exp[KubeAPIServerName] = kubeAPIServerArgs(ip, etcdIPs)
	assert.Equal(t, exp, ctx.fd.running())
}

func TestEtcdRemove(t *testing.T) {
	ctx := initTest()
	ip := "1.2.3.4"
	etcdIPs := []string{ip, "5.6.7.8"}
	ctx.conn.Txn(db.AllTables...).Run(func(view db.Database) error {
		m := view.MinionSelf()
		e := view.SelectFromEtcd(nil)[0]
		m.Role = db.Master
		m.PrivateIP = ip
		e.EtcdIPs = etcdIPs
		view.Commit(m)
		view.Commit(e)
		return nil
	})
	runMasterOnce()

	exp := map[string][]string{
		EtcdName:                  etcdArgsMaster(ip, etcdIPs),
		OvsdbName:                 {"ovsdb-server"},
		RegistryName:              nil,
		KubeAPIServerName:         kubeAPIServerArgs(ip, etcdIPs),
		KubeControllerManagerName: kubeControllerManagerArgs(),
		KubeSchedulerName:         kubeSchedulerArgs(),
	}
	assert.Equal(t, exp, ctx.fd.running())

	// Remove a master
	etcdIPs = etcdIPs[1:]
	ctx.conn.Txn(db.AllTables...).Run(func(view db.Database) error {
		m := view.MinionSelf()
		e := view.SelectFromEtcd(nil)[0]
		m.Role = db.Master
		e.EtcdIPs = etcdIPs
		view.Commit(m)
		view.Commit(e)
		return nil
	})
	runMasterOnce()

	exp[EtcdName] = etcdArgsMaster(ip, etcdIPs)
	exp[KubeAPIServerName] = kubeAPIServerArgs(ip, etcdIPs)
	assert.Equal(t, exp, ctx.fd.running())
}

func etcdArgsMaster(ip string, etcdIPs []string) []string {
	return []string{
		"etcd",
		fmt.Sprintf("--name=master-%s", ip),
		fmt.Sprintf("--initial-cluster=%s", initialClusterString(etcdIPs)),
		fmt.Sprintf("--advertise-client-urls=http://%s:2379", ip),
		fmt.Sprintf("--listen-peer-urls=http://%s:2380", ip),
		fmt.Sprintf("--initial-advertise-peer-urls=http://%s:2380", ip),
		"--listen-client-urls=http://0.0.0.0:2379",
		"--heartbeat-interval=500",
		"--initial-cluster-state=new",
		"--election-timeout=5000",
	}
}
