// Import spark.spec
var Spark = require("github.com/NetSys/quilt/specs/spark/spark");

// We will have three worker machines.
var nWorker = 3;

// Application
// spark.Exclusive enforces that no two Spark containers should be on the
// same node. spark.Public says that the containers should be allowed to talk
// on the public internet. spark.Job causes Spark to run that job when it
// boots.
var sprk = new Spark(1, nWorker);
sprk.exclusive()
sprk.public()
sprk.job("run-example SparkPi");

// Infrastructure

// Using unique Namespaces will allow multiple Quilt instances to run on the
// same cloud provider account without conflict.
setNamespace("kklin");

// Defines the set of addresses that are allowed to access Quilt VMs.
setAdminACL(["local"]);

var baseMachine = new Machine({
    provider: "AmazonSpot",
    region: "us-west-1",
    size: "m4.large",
    diskSize: 32,
    keys: githubKeys("kklin"),
});
deployWorkers(nWorker + 1, baseMachine);
deployMasters(1, baseMachine);

assert(new Reachable(publicInternet, sprk.masters), true);
assert(new Enough(), true);
