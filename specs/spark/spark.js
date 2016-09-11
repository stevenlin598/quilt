var image = "quilt/spark";

function commaSepHosts(labels) {
    return _(labels).map(Label.prototype.hostname).join(",");
}

function createMasters(n, zookeeper) {
    var containers = new Docker(image, ["run", "master"]).replicate(n);

    if (zookeeper) {
        var zookeeperHosts = commaSepHosts(zookeeper);
        for (var i = 0 ; i<containers.length ; i++) {
            containers[i].setEnv("ZOO", zookeeperHosts);
        }
    }

    return new Label("spark-ms", containers);
}

function createWorkers(n, masters) {
    var masterHosts = masters.children().join(",");
    var containers = new Docker(image, ["run", "worker"])
        .withEnv({"MASTERS": masterHosts})
        .replicate(n);

    return new Label("spark-wk", containers);
}

function link(masters, workers, zookeeper) {
    var allPorts = new PortRange(1000, 65535);
    connect(allPorts, workers, masters);
    connect(allPorts, masters, workers);
    connect(new Port(7077), workers, masters);
    if (zookeeper) {
        connect(new Port(2181), masters, zookeeper);
    }
}

function Spark(nMaster, nWorker, zookeeper) {
    this.masters = createMasters(nMaster, zookeeper);
    this.workers = createWorkers(nWorker, this.masters);
    link(this.masters, this.workers, zookeeper);
}

Spark.prototype.job = function(command) {
    var masters = this.masters.containers;
    for (var i = 0 ; i<masters.length ; i++) {
        masters[i].env["JOB"] = command;
    }
}

Spark.prototype.public = function() {
    connect(new Port(8080), publicInternet, this.masters);
    connect(new Port(8081), publicInternet, this.workers);
}

Spark.prototype.exclusive = function(sparkMap) {
    place(this.masters, new LabelRule("exclusive", this.workers))
}

module.exports = Spark;
