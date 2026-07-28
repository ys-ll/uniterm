# Containers

uniTerm has a built-in container management interface supporting **Kubernetes** cluster management and the **Docker / Podman / nerdctl** container engines. Manage cluster resources and container images on the local machine or on remote hosts over SSH.

## Kubernetes

![Kubernetes](/imgs/kubernetes_light.webp)

### Connection Parameters

| Parameter | Description |
|------|------|
| Source | Choose **Local file** (specify a kubeconfig path) or **Kubeconfig text** (paste the content directly) |
| Context | Select a context from the kubeconfig; click "Reload" to refresh the list |
| Namespace | The default namespace to view; check "All namespaces" to browse the whole cluster |
| Skip TLS verification | Skip API Server certificate validation (insecure, for self-signed test environments only) |
| SSH Tunnel | Optional. Access an internal cluster API through an SSH jump host |

After filling in the fields, click "Test connection" to verify the kubeconfig and network reachability.

### Resource Browsing

The left tree panel groups cluster resources by category, watches for changes, and refreshes automatically:

| Group | Resources |
|------|------|
| Workloads | Pods, Deployments, StatefulSets, DaemonSets, ReplicaSets, Jobs, CronJobs, HPAs |
| Network | Services, Ingresses, Endpoints, NetworkPolicies |
| Config | ConfigMaps, Secrets, ResourceQuotas, LimitRanges |
| Storage | PVCs, PVs, StorageClasses |
| RBAC | ServiceAccounts, Roles, RoleBindings, ClusterRoles, ClusterRoleBindings |
| Cluster | Nodes, Namespaces, Events, CRDs |

- **Top filter** — Filter in real time by name, status, namespace, and other columns
- **Health highlighting** — Not-ready Pods, under-replicated workloads, and NotReady nodes are highlighted row-wide
- **Live metrics** — Pod and Node lists show CPU / memory usage and its percentage of requests/limits (requires metrics-server installed in the cluster)

### Resource Actions

After selecting a resource, the right-click menu or inline action buttons provide the following (varies by resource type):

- **Detail** — A side drawer shows the full metadata, Spec, Status, and other fields
- **Edit** — Edit the resource YAML directly and apply
- **Create** — Create resources from a built-in YAML template
- **Scale** — Change the replica count of a Deployment / StatefulSet / ReplicaSet
- **Restart** — Rolling-restart a workload
- **View Pods** — Jump to the Pod list associated with the workload/node
- **Cordon / Drain** — Mark a node unschedulable or evict its Pods
- **Delete** — Delete after confirmation, with optional force delete (skip graceful termination)

### Pod Logs and Terminal

- **Logs** — Follow Pod logs in real time, with pause/resume, timestamps, previous container, auto-scroll, wrap, and clear
- **Terminal** — Exec directly into a Pod container for an interactive shell

## Container Engines

![Container Management](/imgs/container_light.webp)

uniTerm supports the **Docker**, **Podman**, and **nerdctl** container engines.

### Connection Parameters

| Parameter | Description |
|------|------|
| Transport | **Local** uses the container runtime installed on this machine; **Remote over SSH** reuses an existing SSH connection (including credentials and jump host) to manage containers on a remote host |
| SSH Connection | When the transport is remote over SSH, select a saved SSH connection as the channel |

> nerdctl supports switching the containerd namespace from the top bar.

### Container Management

The top tabs switch between the "Containers" and "Images" views.

The **container list** shows name, image, state, port mappings, and creation time, with the following actions:

- **Lifecycle** — Start, stop, restart, pause, unpause
- **Exec** — Enter an interactive terminal in the container
- **Logs** — View container log output
- **Rename** — Change the container name
- **Remove** — Remove the container after confirmation
- **Detail** — View overview, command, network, mounts, environment, and state
- **New Container** — Create a container by specifying image, name, port mappings, volumes, environment variables, restart policy, and command

### Image Management

The **image list** shows repository, tag, ID, and size, and supports:

- **Pull** — Pull an image from a registry by name (e.g. `nginx:latest`)
- **Remove** — Remove the image after confirmation

::: tip Related
- [Remote Terminal](/en/connections/remote-terminal) — SSH connection and jump host configuration
- [Supported Protocols](/en/protocols) — Full protocol and port list
:::
