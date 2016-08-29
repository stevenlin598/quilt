(import "github.com/NetSys/quilt/specs/stdlib/labels")
(import "github.com/NetSys/quilt/specs/stdlib/strings")

var haproxySource = "quay.io/netsys/di-wp-haproxy";
var haproxyDefaultArgs = ["haproxy", "-f", "/usr/local/etc/haproxy/haproxy.cfg"];

function createHAProxyNodes(nodeCount, hosts) {
    return _(nodeCount).times(function(i) {
        var label = new Label(_.uniqueId("haproxy-" + i),
                        [new Docker(haproxySource, {args: ["run"]})]);
        label.containers[0].setEnv("HOST", label.hostname());
        return label;
    });
}

(define (createHAProxyNodes prefix nodeCount hosts)
  (map
    (lambda (i)
      (labels.Docker
        (list prefix i)
        (list haproxySource (hostStr hosts) haproxyDefaultArgs)))
    (range nodeCount)))

// Returns the labels of the new haproxy nodes
(define (create prefix nodeCount hosts)
  (let ((haproxynodes (createHAProxyNodes prefix nodeCount hosts)))
    (connect 80 haproxynodes hosts)
    haproxynodes))

// hosts: List of labels
(define (New prefix nodeCount hosts)
  (if (> nodeCount 0)
         (create prefix nodeCount hosts)))
