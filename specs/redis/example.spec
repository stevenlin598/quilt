// Import redis.spec
var Redis = require("github.com/NetSys/quilt/specs/redis/redis");

var nWorker = 3;

// Boot redis with 2 workers and 1 master. AUTH_PASSWORD is used to secure
// the redis connection
rds = new Redis(2, "AUTH_PASSWORD");
rds.exclusive();

// Using unique Namespaces will allow multiple Quilt instances to run on the
// same cloud provider account without conflict.
Namespace = "CHANGE_ME";

// Defines the set of addresses that are allowed to access Quilt VMs.
AdminACL = ["local"];

var baseMachine = new Machine({
    provider: "Amazon",
    cpu: new Range(2),
    ram: new Range(2),
    keys: githubKeys("kklin"),
});
deployWorkers(nWorker, baseMachine);
deployMasters(1, baseMachine);
