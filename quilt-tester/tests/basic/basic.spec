require("github.com/NetSys/quilt/quilt-tester/config/infrastructure")

// Using unique Namespaces will allow multiple Quilt instances to run on the
// same cloud provider account without conflict.
Namespace =  "REPLACED_IN_TEST_RUN";

// Defines the set of addresses that are allowed to access Quilt VMs.
AdminACL = ["local"];

var nWorker = 1;
new Docker("google/pause", {});
var red = new Label("red", _(nWorker).times(function() {
    return new Docker("google/pause", {});
}));
var blue = new Label("blue", _(3 * nWorker).times(function() {
    return new Docker("google/pause", {});
}));
connect(new PortRange(1024, 65535), red, blue);
connect(new PortRange(1024, 65535), blue, red);
