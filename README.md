### Bugzilla Operator

Operator that automate Bugzilla workflow for OpenShift engineering team.
Specifically, in monitors bugs from [Bugzilla](https://bugzilla.redhat.com) saved search query, that track bugs which are:

* Not *urgent* severity
* Not having customer case or Github linked
* Not have `LifecycleFrozen` in Developer Whiteboard
* Days since the bug was changed is greater than 30 days

For all bugs matching criteria, it will:

* **Add LifecycleStale keyword** to Developer Whiteboard field
* **Place a comment**, telling reporter and assignee that the bug has been flagged as "stale"
* **Degrade severity** and priority of the bug: (`high->medium`, `medium->low`)
* **Ask reporter** to react via `need_info?` flag

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

Example:

```yaml
---
credentials:
  username: # REQUIRED (can be obfuscated via base64:STRING)
  password: # REQUIRED
  apiKey:   # REQUIRED
lists:
  staleListName: # Bugzilla saved search name (eg. "openshift-stale-bug-list")
  sharerID: # Bugzilla sharer ID (numberic, visible in saved search link)
devWhiteboardFlag: # Keyword to flag bugs matching the stale criteria (eg. "LifecycleStale")
staleBugComment: > 
  This bug hasn't had any activity in the last 30 days. Maybe the problem got resolved, was a duplicate of something else, or became less pressing for some reason - or maybe it's still relevant but just hasn't been looked at yet.
  As such, we're marking this bug as "LifecycleStale" and decreasing the severity/priority.
  If you have further information on the current state of the bug, please update it, otherwise this bug can be closed in about 7 days. The information can be, for example, that the problem still occurs,
  that you still want the feature, that more information is needed, or that the bug is (for whatever reason) no longer relevant.
```

License
-------

Licensed under the [Apache License, Version 2.0](http://www.apache.org/licenses/).
