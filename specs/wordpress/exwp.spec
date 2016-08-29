var Spark = require("github.com/NetSys/quilt/specs/spark/spark");
var Haproxy = require("github.com/NetSys/quilt/specs/wordpress/haproxy");
var Memcached = ("github.com/NetSys/quilt/specs/wordpress/memcached");
var Mysql = ("github.com/NetSys/quilt/specs/wordpress/mysql");
var Wordpress = ("github.com/NetSys/quilt/specs/wordpress/wordpress");
var Zookeeper = ("github.com/NetSys/quilt/specs/zookeeper/zookeeper");

function link(spark, db) {
    if (spark && db) {
        connect(new Port(7077), spark.master, db.slave);
        connect(new Port(7077), spark.worker, db.slave);
    }
}

modules.exports = function(nCache, nSql, nWordpress, nHaproxy, nSparkM, nSparkW, nZoo) {
    this.memcd = new Memcached(nCache);
    this.db = new Mysql(nSql);
    this.wp = new Wordpress(nWordpress, this.db, this.memcd);
    this.hap = new Haproxy(nHaproxy, this.wp);
    this.zk = new Zookeeper(nZoo);
    this.spark = new Spark(nSparkM, nSparkW, this.zk);
    link(this.spark, this.db);
}
