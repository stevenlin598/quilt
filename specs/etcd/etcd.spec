var image = "quilt/etcd";

module.exports = function(n) {
    var etcdLabels = _(n).times(function(i) {
        var label = new Label(_.uniqueId("etcd-" + i),
                        [new Docker(image, ["run"])]);
        label.containers[0].setEnv("HOST", label.hostname());
        return label;
    });

    var peerList = [];
    for (var i = 0 ; i<etcdLabels.length ; i++) {
        peerList.push(etcdLabels[i].hostname());
    }
    var peerStr = peerList.join(",");

    for (var i = 0 ; i<etcdLabels.length ; i++) {
        etcdLabels[i].containers[0].setEnv("PEERS", peerStr);
    }

    for (var i = 0 ; i<etcdLabels.length ; i++) {
        for (var j = 0 ; i<etcdLabels.length ; i++) {
            connect(new PortRange(1000, 65535), etcdLabels[i], etcdLabels[j]);
        }
    }

    this.labels = etcdLabels;
}
