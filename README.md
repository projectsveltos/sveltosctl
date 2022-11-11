[![CI](https://github.com/projectsveltos/sveltosctl/actions/workflows/main.yaml/badge.svg)](https://github.com/projectsveltos/sveltosctl/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/projectsveltos/sveltosctl)](https://goreportcard.com/report/github.com/projectsveltos/sveltosctl)
[![Slack](https://img.shields.io/badge/join%20slack-%23projectsveltos-brighteen)](https://join.slack.com/t/projectsveltos/shared_invite/zt-1hraownbr-W8NTs6LTimxLPB8Erj8Q6Q)
[![License](https://img.shields.io/badge/license-Apache-blue.svg)](LICENSE)


# sveltosctl

<img src="https://raw.githubusercontent.com/projectsveltos/sveltos-manager/master/logos/logo.png" width="200">

**sveltosctl** is the command line client for Sveltos. **sveltosctl** nicely displays resources and helm charts info in CAPI Kubernetes Cluster deployed using [ClusterProfile](https://github.com/projectsveltos/sveltos-manager). It also provides the ability to generate configuration snapshots and rollback system to a previously taken configuration snapshot.

It assumes:
1. there is a management cluster with [ClusterAPI](https://github.com/kubernetes-sigs/cluster-api);
2. [ClusterProfile](https://github.com/projectsveltos/sveltos-manager) is used to programmatically define which resources/helm charts need to be deployed in which CAPI Clusters;
3. management cluster can be accessed 
 
> Note: sveltosctl can run as binary though it is advised to run it as pod in a management cluster to get access to all of its features.

## Quick start

### Run sveltosctl as a binary
If you decide to run it as a binary:
1. make sure management cluster can be accessed;
2. run`make build`
3. Use `./bin/sveltosctl --help` to see help message

### Run sveltosctl as a pod
If you decide to run it as a pod in the management cluster, YAML is in manifest subdirectory.

```
kubectl create -f  https://raw.githubusercontent.com/projectsveltos/sveltosctl/dev/manifest/utils.projectsveltos.io_snapshots.yaml

kubectl create -f  https://raw.githubusercontent.com/projectsveltos/sveltosctl/dev/manifest/sveltosctl.yaml
```

Please keep in mind it requires a PersistentVolume. So modify this section accordingly before posting the YAML.

```
  volumeClaimTemplates:
  - metadata:
      name: snapshot
    spec:
      accessModes: [ "ReadWriteOnce" ]
      storageClassName: "standard"
      resources:
        requests:
          storage: 1Gi
```

Once the pod is running,
```
 kubectl exec -it -n projectsveltos sveltosctl-0   -- ./sveltosctl
```

You might also want to change the timezone of sveltosctl pod by using specific timezone config and hostPath volume to set specific timezone. Currently:

```
  volumes:
  - hostPath:
      path: /usr/share/zoneinfo/America/Los_Angeles
      type: File
    name: tz-config
```

## Features List
- Display all resources and helm releases deployed in each CAPI Cluster by ClusterProfile;
- Display the effect of a ClusterProfile in DryRun mode;
- List all snapshots taken;
- Display diff between two taken snapshots;
- Rollback system to a previously taken snapshot configuration;
- Change Sveltos PODs log level without restarting PODs.

## Display deployed resources and helm releases

**show features** can be used to display list of resources/helm releases deployed in CAPI clusters.
Displayed information contains:
1. the CAPI Cluster in the form <namespace>/<name>
2. resource/helm chart information
3. list of ClusterProfiles currently (at the time the command is run) having resource/helm release deployed in the CAPI cluster.

```
./bin/sveltosctl show features
+-------------------------------------+---------------+-----------+----------------+---------+-------------------------------+------------------+
|               CLUSTER               | RESOURCE TYPE | NAMESPACE |      NAME      | VERSION |             TIME              | CLUSTER PROFILE |
+-------------------------------------+---------------+-----------+----------------+---------+-------------------------------+------------------+
| default/sveltos-management-workload | helm chart    | kyverno   | kyverno-latest | v2.5.0  | 2022-09-30 11:48:45 -0700 PDT | clusterfeature1  |
| default/sveltos-management-workload | :Pod          | default   | nginx          | N/A     | 2022-09-30 13:41:05 -0700 PDT | clusterfeature2  |
+-------------------------------------+---------------+-----------+----------------+---------+-------------------------------+------------------+
```

**show features** command has some argurments which allow filtering by:
1. clusters' namespace
2. clusters' name
3. ClusterProfile 

```
./bin/sveltosctl show features --help
Usage:
  sveltosctl show features [options] [--namespace=<name>] [--cluster=<name>] [--clusterprofile=<name>] [--verbose]

     --namespace=<name>      Show features deployed in clusters in this namespace. If not specified all namespaces are considered.
     --cluster=<name>        Show features deployed in cluster with name. If not specified all cluster names are considered.
     --clusterprofile=<name> Show features deployed because of this clusterprofile. If not specified all clusterprofile names are considered.
```

## Display usage

**show usage** displays following information:
1. which CAPI clusters are currently a match for a ClusterProfile
2. for ConfigMap/Secret referenced by at least by ClusterProfile, in which CAPI clusters their content is currently deployed.

Such information is useful to see what CAPI clusters would be affected by a change before making such a change.

```
./bin/sveltosctl show usage 
+----------------+--------------------+----------------------------+-------------------------------------+
| RESOURCE KIND  | RESOURCE NAMESPACE |       RESOURCE NAME        |              CLUSTERS               |
+----------------+--------------------+----------------------------+-------------------------------------+
| ClusterProfile |                    | mgianluc                   | default/sveltos-management-workload |
| ConfigMap      | default            | kyverno-disallow-gateway-2 | default/sveltos-management-workload |
+----------------+--------------------+----------------------------+-------------------------------------+
```

## Log severity settings
**log-level** used to display and change log severity in Sveltos PODs without restarting them.

Following for instance change log severity for the Classifier POD to debug

```
./bin/sveltosctl log-level set --component=Classifier --debug
```

Show can be used to display current log severity settings

```
./bin/sveltosctl log-level show                              
+------------+---------------+
| COMPONENT  |   VERBOSITY   |
+------------+---------------+
| Classifier | LogLevelDebug |
```

## Display outcome of ClusterProfile's in DryRun mode

A ClusterProfile can be set in DryRun mode. While in DryRun mode, nothing gets deployed/withdrawn to/from matching CAPI clusters. A report is instead generated listing what would happen if ClusterProfile sync mode would be changed from DryRun to Continuous.

Here is an example of outcome

```
./bin/sveltosctl show dryrun
+-------------------------------------+--------------------------+-----------+----------------+-----------+--------------------------------+------------------+
|               CLUSTER               |      RESOURCE TYPE       | NAMESPACE |      NAME      |  ACTION   |            MESSAGE             | CLUSTER PROFILE |
+-------------------------------------+--------------------------+-----------+----------------+-----------+--------------------------------+------------------+
| default/sveltos-management-workload | helm release             | kyverno   | kyverno-latest | Install   |                                | dryrun           |
| default/sveltos-management-workload | helm release             | nginx     | nginx-latest   | Install   |                                | dryrun           |
| default/sveltos-management-workload | :Pod                     | default   | nginx          | No Action | Object already deployed.       | dryrun           |
|                                     |                          |           |                |           | And policy referenced by       |                  |
|                                     |                          |           |                |           | ClusterProfile has not changed |                  |
|                                     |                          |           |                |           | since last deployment.         |                  |
| default/sveltos-management-workload | kyverno.io:ClusterPolicy |           | no-gateway     | Create    |                                | dryrun           |
+-------------------------------------+--------------------------+-----------+----------------+-----------+--------------------------------+------------------+
```


**show dryrun** command has some argurments which allow filtering by:
1. clusters' namespace
2. clusters' name
3. ClusterProfile 

```
./bin/sveltosctl show dryrun --help  
Usage:
  sveltosctl show dryrun [options] [--namespace=<name>] [--cluster=<name>] [--clusterprofile=<name>] [--verbose]

     --namespace=<name>      Show which features would change in clusters in this namespace. If not specified all namespaces are considered.
     --cluster=<name>        Show which features would change in cluster with name. If not specified all cluster names are considered.
     --clusterprofile=<name> Show which features would change because of this clusterprofile. If not specified all clusterprofile names are considered.
```

## Snapshot

When running sveltosctl as pod in the management cluster, it can take configuration snapshot.

A snapshot allows an administrator to perform the following tasks:
1. Live snapshots of the running configuration deployed by ClusterProfiles in each CAPI cluster;
2. Recurring snapshots;
3. Versioned storage of the configuration
4. Full viewing of any snapshot configuration including the differences between snapshots
5. Rollback to any previous configuration snapshot.

Define a Snapshot instance, following for instance will take a snaphost every hour.

```
apiVersion: utils.projectsveltos.io/v1alpha1
kind: Snapshot
metadata:
  name: hourly
spec:
  schedule: "00 * * * *"
  storage: /snapshot
```

where field _schedule_ is defined in [Cron format](https://en.wikipedia.org/wiki/Cron).

The configuration snapshots consist of text files containing:
1. ClusterProfiles;
2. All ConfigMaps/Secrets referenced by at least one ClusterProfile;
3. CAPI Cluster labels;
4. few other internal `config.projectsveltos.io` CRD instances.
   
The snapshot contains the configuration at the time of the snapshot stored. Each snapshot is stored with a version identifier. The version identifier is automatically generated by concatenating the date with the time of the snapshot.

### list
  
**snapshot list** can be used to display all available snapshots:

```
kubectl exec -it -n projectsveltos sveltosctl-0 -- ./sveltosctl snapshot list --snapshot=hourly 
+-----------------+---------------------+
| SNAPSHOT POLICY |        DATE         |
+-----------------+---------------------+
| hourly          | 2022-10-10:22:00:00 |
| hourly          | 2022-10-10:23:00:00 |
+-----------------+---------------------+
```

### diff

**snapshot diff** can be used to display all the configuration changes between two snapshots:

```
kubectl exec -it -n projectsveltos sveltosctl-0 -- ./sveltosctl snapshot diff --snapshot=hourly  --from-sample=2022-10-10:22:00:00 --to-sample=2022-10-10:23:00:00 
+-------------------------------------+--------------------------+-----------+----------------+----------+------------------------------------+
|               CLUSTER               |      RESOURCE TYPE       | NAMESPACE |      NAME      |  ACTION  |              MESSAGE               |
+-------------------------------------+--------------------------+-----------+----------------+----------+------------------------------------+
| default/sveltos-management-workload | helm release             | mysql     | mysql          | added    |                                    |
| default/sveltos-management-workload | helm release             | nginx     | nginx-latest   | added    |                                    |
| default/sveltos-management-workload | helm release             | kyverno   | kyverno-latest | modified | To version: v2.5.0 From            |
|                                     |                          |           |                |          | version v2.5.3                     |
| default/sveltos-management-workload | /Pod                     | default   | nginx          | added    |                                    |
| default/sveltos-management-workload | kyverno.io/ClusterPolicy |           | no-gateway     | modified | To see diff compare ConfigMap      |
|                                     |                          |           |                |          | default/kyverno-disallow-gateway-2 |
|                                     |                          |           |                |          | in the from folderwith ConfigMap   |
|                                     |                          |           |                |          | default/kyverno-disallow-gateway-2 |
|                                     |                          |           |                |          | in the to folder                   |
+-------------------------------------+--------------------------+-----------+----------------+----------+------------------------------------+
```

To see Sveltos CLI for snapshot in action, have a look at this [video](https://youtu.be/ALcp1_Nj9r4)

### rollback

Rollback is when a previous configuration snapshot is used to replace the current configuration deployed by ClusterProfiles. This can be done on the granularity of:. 
1. namespace: Rollbacks only ConfigMaps/Secrets and Cluster labels in this namespace. If not specified all namespaces are considered;
2. cluster: Rollback only labels for cluster with this name. If not specified all cluster's labels are updated;
3. clusterprofile: Rollback only clusterprofile with this name. If not specified all clusterprofiles are updated

When all of the configuration files for a particular version are used to replace the current configuration, this is referred to as a full rollback.

Following for instance will bring system back to the state it had at 22:00

```
kubectl exec -it -n projectsveltos sveltosctl-0 -- ./sveltosctl snapshot rollback --snapshot=hourly  --sample=2022-10-10:22:00:00
```

To see Sveltos CLI for snapshot in action, have a look at this [video](https://youtu.be/sTo6RcWP1BQ)
  
## Contributing
If you have questions, noticed any bug or want to get the latest project news, you can connect with us in the following ways:
1. Open a bug/feature enhancement on github;
2. Chat with us on the Slack in the [#projectsveltos](https://join.slack.com/t/projectsveltos/shared_invite/zt-1hraownbr-W8NTs6LTimxLPB8Erj8Q6Q) channel;

## License

Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
