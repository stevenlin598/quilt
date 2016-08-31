(import "github.com/NetSys/quilt/specs/stdlib/strings")

(define image "quilt/spark")

(define (commaSepHosts lbl)
  (strings.Join (labelHosts lbl) ","))

(define (createMasters prefix n zookeeper)
  (let ((sparkDockers (makeList n (docker image "run" "master")))
        // XXX: Once the Zookeeper spec is rewritten to represent the Zookeeper
        // containers as a single label, instead of a list, this map will be
        // replaced by a call to `commaSepHosts`.
        (zookeeperHosts (strings.Join (map labelHost zookeeper) ",")))
    (if zookeeper
      (setEnv sparkDockers "ZOO" zookeeperHosts))
    (label (sprintf "%s-ms" prefix) sparkDockers)))

(define (createWorkers prefix n masters)
  (let ((masterHosts (commaSepHosts masters))
        (sparkDockers (makeList n (docker image "run" "worker"))))
    (setEnv sparkDockers "MASTERS" masterHosts)
    (label (sprintf "%s-wk" prefix) sparkDockers)))

(define (link masters workers zookeeper)
  (connect (list 1000 65535) masters workers)
  (connect (list 1000 65535) workers workers)
  (connect 7077 workers masters)
  (if zookeeper
    (connect 2181 masters zookeeper)))

// zookeeper: optional list of zookeeper nodes (empty list if unwanted)
(define (New prefix nMaster nWorker zookeeper)
  (let ((masters (createMasters prefix nMaster zookeeper))
        (workers (createWorkers prefix nWorker masters)))
    (if (and masters workers)
      (progn
        (link masters workers zookeeper)
        (hmap ("master" masters)
              ("worker" workers))))))

(define (Job sparkMap command)
  (setEnv (hmapGet sparkMap "master") "JOB" command))

(define (Exclusive sparkMap)
  (let ((exfn (lambda (x) (labelRule "exclusive" x)))
	(rules (map exfn (hmapValues sparkMap)))
	(plfn (lambda (x) (place x (hmapValues sparkMap)))))
    (map plfn rules)))

(define (Public sparkMap)
  (connect 8080 "public" (hmapGet sparkMap "master"))
  (connect 8081 "public" (hmapGet sparkMap "worker")))
