# sveltosctl
a CLI to nicely display resources/helm charts info in CAPI Cluster deployed using [ClusterProfile](https://github.com/projectsveltos/cluster-api-feature-manager)

It assumes:
1. there is a management cluster with [ClusterAPI](https://github.com/kubernetes-sigs/cluster-api);
2. [ClusterProfile](https://github.com/projectsveltos/cluster-api-feature-manager) are used to programmatically define which resources/helm charts need to be deployed in which CAPI Clusters;
3. management cluster can be accessed.

## Display deployed resources/helm releases

show features can be used to display list of resources/helm releases deployed in CAPI clusters.
Displayed information contains:
1. the CAPI Cluster in the form <namespace>/<name>
2. resource/helm chart information
3. list of ClusterProfiles currently (at the time the command is run) having resource/helm release deployed in the CAPI cluster.

```
./bin/sveltosctl show features
+-------------------------------------+---------------+-----------+----------------+---------+-------------------------------+------------------+
|               CLUSTER               | RESOURCE TYPE | NAMESPACE |      NAME      | VERSION |             TIME              | CLUSTER FEATURES |
+-------------------------------------+---------------+-----------+----------------+---------+-------------------------------+------------------+
| default/sveltos-management-workload | helm chart    | kyverno   | kyverno-latest | v2.5.0  | 2022-09-30 11:48:45 -0700 PDT | clusterfeature1  |
| default/sveltos-management-workload | :Pod          | default   | nginx          | N/A     | 2022-09-30 13:41:05 -0700 PDT | clusterfeature2  |
+-------------------------------------+---------------+-----------+----------------+---------+-------------------------------+------------------+
```

show feature command has some argurments which allow filtering by:
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

## Display outcome of ClusterProfile's in DryRun mode

A ClusterProfile can be set in DryRun mode. While in DryRun mode, nothing gets deployed/withdrawn to/from matching CAPI clusters. A report is instead generated listing what would happen if ClusterProfile sync mode would be changed from DryRun to Continuous.

Here is an example of outcome

```
./bin/sveltosctl show dryrun
+-------------------------------------+--------------------------+-----------+----------------+-----------+--------------------------------+------------------+
|               CLUSTER               |      RESOURCE TYPE       | NAMESPACE |      NAME      |  ACTION   |            MESSAGE             | CLUSTER FEATURES |
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


show dryrun command has some argurments which allow filtering by:
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

### list
  
CLI snapshot list can be used to display all available snapshots:

```
./sveltosctl snapshot list --snapshot=hourly 
+-----------------+---------------------+
| SNAPSHOT POLICY |        DATE         |
+-----------------+---------------------+
| hourly          | 2022-10-10:22:00:00 |
| hourly          | 2022-10-10:23:00:00 |
+-----------------+---------------------+
```

### diff

CLI snapshot diff can be used to display all changes between two snapshots:

```
kubectl exec -it -n projectsveltos                      sveltosctl-0   -- ./sveltosctl snapshot diff --snapshot=hourly  --from-sample=2022-10-10:22:00:00 --to-sample=2022-10-10:23:00:00 
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

### rollback

Finally, snapshot rollback can be used to bring system back in time to a given taken snapshot.
Following will bring system back to the state it had at 22:00

```
kubectl exec -it -n projectsveltos                      sveltosctl-0   -- ./sveltosctl snapshot rollback --snapshot=hourly  --sample=2022-10-10:22:00:00
```
