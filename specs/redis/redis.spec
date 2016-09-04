var image = "quilt/redis";

function createMaster(auth) {
    var masterContainer = new Docker(image, ["run"]);
    masterContainer.setEnv("AUTH", auth);
    masterContainer.setEnv("ROLE", "master");
    return new Label(_.uniqueId("redis-ms"), [masterContainer]);
}

function createWorkers(n, auth, master) {
    var workers = _(n).times(function() {
        return new Docker(image, ["run"]);
    });
    for (var i = 0 ; i<workers.length ; i++) {
        workers[i].setEnv("ROLE", "worker");
        workers[i].setEnv("MASTER", master.hostname());
        workers[i].setEnv("AUTH", auth);
    }
    return new Label(_.uniqueId("redis-wk"), workers);
}

function Redis(nWorker, auth) {
    this.master = createMaster(auth);
    this.workers = createWorkers(nWorker, auth, this.master);
    connect(new Port(6379), this.master, this.workers);
    connect(new Port(6379), this.workers, this.master);
}

Redis.prototype.exclusive = function() {
    place(this.master, new LabelRule(true, this.workers));
}

module.exports = Redis;
