![Go](https://github.com/mfojtik/bugzilla-operator/workflows/Go/badge.svg)

### Bugzilla Operator

This operator provide [Bugzilla](https://bugzilla.redhat.com) automation and reporting for OpenShift engineering team, in particular:

* Flag bugs that are inactive for longer than 30 days with _LifecycleStale_ keyword in Developer Whiteboard.
* Automatically close bugs that were flagged after 7 days for further inactivity.
* Automatically reset the flag when activity (_needinfo?_) flag is reset.
* Report current blocker bug counts via Slack integration to team status channel and provide provide personalized list to bug assignee
* Report bugs closed in last 24h via Slack integration

#### Controllers

`stalecontroller` list bugs that has been inactive for 30 days and will be flagged with _LifecycleStale_:

* Days since bug changed: (is greater than or equal to) 30
* Severity is **not** urgent
* Link System Description does not include _Customer Portal_ (customer bugs)
* Link System Description does not include _Github_ (bugs with PR's)
* Summary does not include string _CVE_
* Status is either `NEW`, `ASSIGNED` or `POST`
    
All bugs returned from this search query are updated as following:

* `LifecycleStale` keyword is added to _Developer Whiteboard_ field
* Comment is added to the bug, asking reporter or assignee to take action
* The bug priority is degraded (high->medium, medium->low)
* The `needinfo?` flag is set for reporter
* Both reporter and assignee are notified via Slack integration

Bugs with `LifecycleStale` keyword are automatically closed after 7 days of them being flagged, unless the keyword is removed or the `needinfo?` flag is reset.

`resetcontroller` list bugs that has been flagged as _LifecycleStale_ but their _needinfo_ flag was reset
    * Devel Whiteboard has _LifecycleStale_
    * Flags does not contain the string `needinfo?`
    * Status is either `NEW`, `ASSIGNED` or `POST`
    
All bugs returned from this search query are updated as following:

* `LifecycleReset` keyword is added to _Developer Whiteboard_ field (and `LifecycleStale` is removed)
    
#### Reports

Following reports are being delivery based on the cron schedule:

* `blockers`
    * Status is either `NEW`, `ASSIGNED` or `POST`
    * Target release points to current release or `---` or any _z-stream release
    * Priority and Severity is not _low_

* `closed`
    * Status is `CLOSED`
    * Status changed after -1d


#### Installation

```
kubectl apply -f ./manifests
```

or:

```shell script
make install
```

#### Configuration

The operator is configured via YAML configuration file you have to pass via the bugzilla-operator run --config flag.
The operator automatically restart when this config is changed. The config is available via `configmap/operator-config`.

License
-------

Licensed under the [Apache License, Version 2.0](http://www.apache.org/licenses/).
