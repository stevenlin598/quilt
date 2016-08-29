var Etcd = require("github.com/NetSys/quilt/specs/etcd/etcd") // Import etcd.spec

var nWorker = 3;
new Etcd(nWorker);

// Using unique Namespaces will allow multiple Quilt instances to run on the
// same cloud provider account without conflict.
Namespace = "CHANGE_ME";

// Defines the set of addresses that are allowed to access Quilt VMs.
AdminACL = ["local"];

var baseMachine = new Machine({
    provider: "Amazon",
    diskSize: 32,
    cpu: new Range(2),
    ram: new Range(2),
    keys: githubKeys("kklin"),
});
deployWorkers(nWorker, baseMachine);
deployMasters(1, baseMachine);
