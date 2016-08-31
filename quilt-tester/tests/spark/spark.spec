(import "github.com/NetSys/quilt/quilt-tester/config/infrastructure")
(import "github.com/NetSys/quilt/specs/spark/spark") // Import spark.spec

// We will have three worker machines.
(define nWorker 3)

// Using unique Namespaces will allow multiple Quilt instances to run on the
// same cloud provider account without conflict.
(define Namespace "REPLACED_IN_TEST_RUN")

// Defines the set of addresses that are allowed to access Quilt VMs.
(define AdminACL (list "local"))

// Application
// spark.Exclusive enforces that no two Spark containers should be on the
// same node. spark.Public says that the containers should be allowed to talk
// on the public internet. spark.Job causes Spark to run that job when it
// boots.
(let ((sprk (spark.New "spark" 1 nWorker (list))))
     (spark.Exclusive sprk)
     (spark.Public sprk)
     (spark.Job sprk "run-example SparkPi"))

(invariant reach true "public" "spark-ms")
(invariant enough)
