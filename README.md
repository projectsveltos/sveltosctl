[![CI](https://github.com/projectsveltos/sveltosctl/actions/workflows/main.yaml/badge.svg)](https://github.com/projectsveltos/sveltosctl/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/projectsveltos/sveltosctl)](https://goreportcard.com/report/github.com/projectsveltos/sveltosctl)
[![Slack](https://img.shields.io/badge/join%20slack-%23projectsveltos-brighteen)](https://join.slack.com/t/projectsveltos/shared_invite/zt-1hraownbr-W8NTs6LTimxLPB8Erj8Q6Q)
[![License](https://img.shields.io/badge/license-Apache-blue.svg)](LICENSE)
[![Twitter Follow](https://img.shields.io/twitter/follow/projectsveltos?style=social)](https://twitter.com/projectsveltos)

# sveltosctl

<img src="https://raw.githubusercontent.com/projectsveltos/sveltos/main/docs/assets/logo.png" width="200">

Please refere to sveltos [documentation](https://projectsveltos.github.io/sveltos/).

**sveltosctl** is the command line client for Sveltos. **sveltosctl** nicely displays resources and helm charts info in custer deployed using [ClusterProfile/Profile](https://github.com/projectsveltos/addon-controller).

It assumes:
1. [ClusterProfile/Profile](https://github.com/projectsveltos/addon-controller) is used to programmatically define which resources/helm charts need to be deployed in which CAPI Clusters;
2. management cluster can be accessed

> Note: sveltosctl can run as binary though it is advised to run it as pod in a management cluster to get access to all of its features.

## Quick start

### Run sveltosctl as a binary
If you decide to run it as a binary:
1. make sure management cluster can be accessed;
2. run`make build`
3. Use `./bin/sveltosctl --help` to see help message

- [sveltosctl](#sveltosctl)
  - [Quick start](#quick-start)
    - [Run sveltosctl as a binary](#run-sveltosctl-as-a-binary)
    - [Run sveltosctl as a pod](#run-sveltosctl-as-a-pod)
  - [Display deployed Kubernetes add-ons](#display-deployed-addons)
  - [Display resources in managed clusters](#display-information-about-resources-in-managed-cluster)
  - [Display usage](#display-usage)
  - [Multi-tenancy: display admin permissions](#multi-tenancy-display-admin-permissions)
  - [Log severity settings](#log-severity-settings)
  - [Display outcome of ClusterProfile/Profile in DryRun mode](#display-outcome-of-clusterprofile-in-dryrun-mode)
  - [Admin RBACs](#admin-rbacs)
  - [Contributing](#contributing)
  - [License](#license)

## Display deployed add-ons

**show addons** can be used to display list of Kubernetes addons (resources/helm) releases deployed in CAPI clusters.
Displayed information contains:
1. the CAPI Cluster in the form <namespace>/<name>
2. resource/helm chart information
3. list of ClusterProfiles/Profiles currently (at the time the command is run) having resource/helm release deployed in the CAPI cluster.

```
./bin/sveltosctl show addons
+-------------------------------------+---------------+-----------+----------------+---------+-------------------------------+------------------+
|               CLUSTER               | RESOURCE TYPE | NAMESPACE |      NAME      | VERSION |             TIME              | CLUSTER PROFILE |
+-------------------------------------+---------------+-----------+----------------+---------+-------------------------------+------------------+
| default/sveltos-management-workload | helm chart    | kyverno   | kyverno-latest | v2.5.0  | 2022-09-30 11:48:45 -0700 PDT | clusterfeature1  |
| default/sveltos-management-workload | :Pod          | default   | nginx          | N/A     | 2022-09-30 13:41:05 -0700 PDT | clusterfeature2  |
+-------------------------------------+---------------+-----------+----------------+---------+-------------------------------+------------------+
```

**show addons** command has some argurments which allow filtering by:
1. clusters' namespace
2. clusters' name
3. ClusterProfile/Profile

```
./bin/sveltosctl show addons --help
Usage:
  sveltosctl show addons [options] [--namespace=<name>] [--cluster=<name>] [--profile=<name>] [--verbose]

     --namespace=<name>     Show addons deployed in clusters in this namespace. If not specified all namespaces are considered.
     --cluster=<name>       Show addons deployed in cluster with name. If not specified all cluster names are considered.
     --profile=<kind/name>  Show addons deployed because of this clusterprofile/profile. If not specified all clusterprofiles/profiles are considered.
```

## Register a cluster

If there is kubeconfig with multiple contexts, the option __fleet-cluster-context__
allows to specify the context for the cluster to be managed.

So with default context pointing to the management cluster, following command will:
1. create a ServiceAccount in the managed cluster (using cluster-1 context)
2. grant this ServiceAccount cluster-admin permission
3. create a TokenRequest for such account and a Kubeconfig with bearer token from the TokenRequest
4. create a SveltosCluster in the management cluster (so using default context) and a Secret
with kubeconfig generated in the step above

```
sveltosctl register cluster --namespace=gcp --cluster=cluster-1 --fleet-cluster-context=cluster-1 --labels=k1=v1,k2=v2
```

## Display information about resources in managed cluster

**show resources** looks at all the HealthCheckReport instances and display information about those.
Defining ClusterHealthCheck/HealthCheck you can define which information to collect from which managed clusters.
Please see [documentation](https://projectsveltos.github.io/sveltos/)

For instance:

```
+-------------------------------------+--------------------------+----------------+-------------------------+----------------------------+
|               CLUSTER               |           GVK            |   NAMESPACE    |          NAME           |          MESSAGE           |
+-------------------------------------+--------------------------+----------------+-------------------------+----------------------------+
| default/sveltos-management-workload | apps/v1, Kind=Deployment | kube-system    | calico-kube-controllers | All replicas 1 are healthy |
|                                     |                          | kube-system    | coredns                 | All replicas 2 are healthy |
|                                     |                          | projectsveltos | sveltos-agent-manager   | All replicas 1 are healthy |
| gke/production                      | apps/v1, Kind=Deployment | kube-system    | calico-kube-controllers | All replicas 1 are healthy |
|                                     |                          | kube-system    | coredns                 | All replicas 2 are healthy |
|                                     |                          | projectsveltos | sveltos-agent-manager   | All replicas 1 are healthy |
+-------------------------------------+--------------------------+----------------+-------------------------+----------------------------+
```

## Display usage

**show usage** displays following information:
1. which CAPI clusters are currently a match for a ClusterProfile
2. for ConfigMap/Secret referenced by at least by ClusterProfile, in which CAPI clusters their content is currently deployed.

Such information is useful to see what CAPI clusters would be affected by a change before making such a change.

```
./bin/sveltosctl show usage
----------------+--------------------+----------------------------+-------------------------------------+
| RESOURCE KIND  | RESOURCE NAMESPACE |       RESOURCE NAME        |              CLUSTERS               |
+----------------+--------------------+----------------------------+-------------------------------------+
| ClusterProfile |                    | mgianluc                   | default/sveltos-management-workload |
| ConfigMap      | default            | kyverno-disallow-gateway-2 | default/sveltos-management-workload |
+----------------+--------------------+----------------------------+-------------------------------------+
```

## Multi-tenancy: display admin permissions

**show admin-rbac** can be used to display permissions granted to tenant admins in each managed clusters.
If we have two clusters, a ClusterAPI powered one and a SveltosCluster, both matching label selector
```env=internal``` and we post [RoleRequests](https://raw.githubusercontent.com/projectsveltos/access-manager/main/examples/shared_access.yaml), we get:

```
./bin/sveltosctl show admin-rbac
+---------------------------------------------+-------+----------------+------------+-----------+----------------+-------+
|                   CLUSTER                   | ADMIN |   NAMESPACE    | API GROUPS | RESOURCES | RESOURCE NAMES | VERBS |
+---------------------------------------------+-------+----------------+------------+-----------+----------------+-------+
| Cluster:default/sveltos-management-workload | eng   | build          | *          | *         | *              | *     |
| Cluster:default/sveltos-management-workload | eng   | ci-cd          | *          | *         | *              | *     |
| Cluster:default/sveltos-management-workload | hr    | human-resource | *          | *         | *              | *     |
| SveltosCluster:gke/prod-cluster             | eng   | build          | *          | *         | *              | *     |
| SveltosCluster:gke/prod-cluster             | eng   | ci-cd          | *          | *         | *              | *     |
| SveltosCluster:gke/prod-cluster             | hr    | human-resource | *          | *         | *              | *     |
+---------------------------------------------+-------+----------------+------------+-----------+----------------+-------+
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

## Display outcome of ClusterProfile in DryRun mode

See [video](https://youtu.be/gfWN_QJAL6k).
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
  sveltosctl show dryrun [options] [--namespace=<name>] [--cluster=<name>] [--profile=<name>] [--verbose]

     --namespace=<name> Show which Kubernetes addons would change in clusters in this namespace. If not specified all namespaces are considered.
     --cluster=<name>   Show which Kubernetes addons would change in cluster with name. If not specified all cluster names are considered.
     --profile=<name>   Show which Kubernetes addons would change because of this clusterprofile/profile. If not specified all clusterprofiles/profiles are considered.
```

## Admin RBACs

**sveltosctl show admin-rbac** can be used to display admin's RBACs per cluster:

```
./bin/sveltosctl show admin-rbac
+---------------------------------------------+-------------+-----------+------------+-----------+----------------+----------------+
|                   CLUSTER                   |  ADMIN      | NAMESPACE | API GROUPS | RESOURCES | RESOURCE NAMES |     VERBS      |
+---------------------------------------------+-------------+-----------+------------+-----------+----------------+----------------+
| Cluster:default/sveltos-management-workload | eng/devops  | default   |            | pods      | pods           | get,watch,list |
+---------------------------------------------+-------------+-----------+------------+-----------+----------------+----------------+
```

## Contributing

❤️ Your contributions are always welcome! If you want to contribute, have questions, noticed any bug or want to get the latest project news, you can connect with us in the following ways:

1. Open a bug/feature enhancement on github [![contributions welcome](https://img.shields.io/badge/contributions-welcome-brightgreen.svg?style=flat)](https://github.com/projectsveltos/addon-controller/issues)
2. Chat with us on the Slack in the #projectsveltos channel [![Slack](https://img.shields.io/badge/join%20slack-%23projectsveltos-brighteen)](https://join.slack.com/t/projectsveltos/shared_invite/zt-1hraownbr-W8NTs6LTimxLPB8Erj8Q6Q)
3. [Contact Us](mailto:support@projectsveltos.io)

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
