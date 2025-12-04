# testenv-vm Architecture

## System Overview

```
+------------------------------------------------------------------+
|                         Forge Test Runner                         |
|                      (forge test <stage> run)                     |
+------------------------------------------------------------------+
                                   |
                                   | MCP: "create" / "delete"
                                   v
+------------------------------------------------------------------+
|                     testenv-vm MCP Server                         |
|                                                                   |
|  +--------------------+  +---------------+  +-----------------+   |
|  | Spec Parser        |  | DAG Builder   |  | Executor        |   |
|  | - Validate         |  | - Deps graph  |  | - Parallel exec |   |
|  | - Template render  |  | - Topo sort   |  | - Rollback      |   |
|  +--------------------+  +---------------+  +-----------------+   |
|                                   |                               |
|  +------------------------------------------------------------+   |
|  |                    Provider Manager                         |   |
|  |  - Start/stop provider MCP servers                          |   |
|  |  - Route operations to providers                            |   |
|  |  - Handle "not implemented" responses                       |   |
|  +------------------------------------------------------------+   |
+------------------------------------------------------------------+
          |                        |                        |
          | MCP                    | MCP                    | MCP
          v                        v                        v
+------------------+    +------------------+    +------------------+
| Libvirt Provider |    |  QEMU Provider   |    |  AWS Provider    |
| - vm, network    |    | - vm, network    |    | - vm (EC2)       |
| - key            |    | - key            |    | - network (VPC)  |
+------------------+    +------------------+    +------------------+
```

## Component Architecture

```
+------------------------------------------------------------------+
|                        testenv-vm Binary                          |
+------------------------------------------------------------------+
|  cmd/testenv-vm/main.go                                          |
|       |                                                           |
|       v                                                           |
|  +------------------------------------------------------------+   |
|  |  pkg/mcp/server.go - MCP "create" and "delete" tools        |   |
|  +------------------------------------------------------------+   |
|       |                                                           |
|       v                                                           |
|  +------------------------------------------------------------+   |
|  |  pkg/orchestrator/orchestrator.go - Coordination logic      |   |
|  +------------------------------------------------------------+   |
|       |                    |                    |                 |
|       v                    v                    v                 |
|  +--------------+  +---------------+  +------------------+        |
|  | pkg/spec/    |  | pkg/provider/ |  | pkg/state/       |        |
|  | parser.go    |  | manager.go    |  | store.go         |        |
|  | validator.go |  | client.go     |  |                  |        |
|  | template.go  |  | registry.go   |  |                  |        |
|  +--------------+  +---------------+  +------------------+        |
+------------------------------------------------------------------+
```

## Dependency Resolution (DAG)

```
Spec Analysis --> Template Scanning --> Graph Construction --> Execution

Example Environment:
  keys: [vm-ssh]
  networks: [bridge, libvirt-net (attachTo: bridge), dnsmasq (attachTo: bridge)]
  vms: [test-vm (network: libvirt-net, cloudInit.sshKeys: vm-ssh)]

Dependency Graph:
                    +----------+
                    |  vm-ssh  |
                    |  (key)   |
                    +----+-----+
                         |
    +--------------------+--------------------+
    |                                         |
    v                                         |
+----------+                                  |
|  bridge  |                                  |
| (network)|                                  |
+----+-----+                                  |
     |                                        |
     +------------------+                     |
     |                  |                     |
     v                  v                     |
+-----------+    +-----------+                |
|libvirt-net|    |  dnsmasq  |                |
| (network) |    | (network) |                |
+-----+-----+    +-----------+                |
      |                                       |
      +-------------------+-------------------+
                          |
                          v
                    +-----------+
                    |  test-vm  |
                    |   (vm)    |
                    +-----------+

Execution Phases:
  Phase 1: vm-ssh (key)           [parallel: 1 resource]
  Phase 2: bridge (network)       [parallel: 1 resource]
  Phase 3: libvirt-net, dnsmasq   [parallel: 2 resources]
  Phase 4: test-vm (vm)           [parallel: 1 resource]
```

## Provider Communication

```
Orchestrator                          Provider MCP Server
     |                                       |
     |-------- provider_capabilities ------->|
     |<------- {resources, operations} ------|
     |                                       |
     |-------- vm_create {name, spec} ------>|
     |<------- {success, resource state} ----|
     |                                       |
     |-------- vm_delete {name} ------------>|
     |<------- {success} --------------------|
```

## Resource Lifecycle

```
                    CREATE FLOW

[Spec] --> [Parse] --> [Validate] --> [Build DAG] --> [Execute Phases]
                                                            |
                   +----------------------------------------+
                   v
           +---------------+
           | For each phase|
           +-------+-------+
                   |
       +-----------+-----------+
       v                       v
  [Parallel]              [Parallel]
  Resource 1              Resource 2
       |                       |
       v                       v
  [Render Templates]     [Render Templates]
       |                       |
       v                       v
  [Call Provider]        [Call Provider]
       |                       |
       v                       v
  [Update State]         [Update State]
       |                       |
       +-----------+-----------+
                   v
           [Next Phase or Done]


                    DELETE FLOW

[Load State] --> [Reverse DAG Order] --> [Delete Resources]
                                               |
                  +----------------------------+
                  v
          +---------------+
          | For each phase|  (reverse order)
          +-------+-------+
                  |
      +-----------+-----------+
      v                       v
 [Delete VM]           [Delete VM]
      |                       |
      v                       v
 [Best Effort]         [Best Effort]
 [Log Errors]          [Log Errors]
      |                       |
      +-----------+-----------+
                  v
          [Delete Networks] --> [Delete Keys] --> [Cleanup Files]
```

## State Storage

```
.forge/testenv-vm/
    +-- state/
    |     +-- testenv-abc123.json    <-- Environment state
    +-- artifacts/
          +-- testenv-abc123/
                +-- vm-ssh.pub       <-- SSH public key
                +-- vm-ssh           <-- SSH private key
                +-- test-vm.console  <-- VM console output

State JSON Structure:
{
  "id": "testenv-abc123",
  "status": "ready|creating|failed|destroying",
  "resources": {
    "keys": { "<name>": { "provider": "...", "status": "...", "state": {...} } },
    "networks": { "<name>": { "provider": "...", "status": "...", "state": {...} } },
    "vms": { "<name>": { "provider": "...", "status": "...", "state": {...} } }
  },
  "executionPlan": { "phases": [...] }
}
```

## Template Resolution

```
Input Spec (with templates):
+------------------------------------------+
| cloudInit:                               |
|   users:                                 |
|     - sshAuthorizedKeys:                 |
|         - "{{ .Keys.vm-ssh.PublicKey }}" |
+------------------------------------------+
                    |
                    v
Template Context (built from completed resources):
+------------------------------------------+
| Keys:                                    |
|   vm-ssh:                               |
|     PublicKey: "ssh-ed25519 AAAA..."    |
|     PrivateKeyPath: "/path/to/key"      |
+------------------------------------------+
                    |
                    v
Rendered Output:
+------------------------------------------+
| cloudInit:                               |
|   users:                                 |
|     - sshAuthorizedKeys:                 |
|         - "ssh-ed25519 AAAA..."         |
+------------------------------------------+

Available Template Variables:
  {{ .Keys.<name>.PublicKey }}        {{ .Networks.<name>.IP }}
  {{ .Keys.<name>.PrivateKeyPath }}   {{ .Networks.<name>.InterfaceName }}
  {{ .VMs.<name>.IP }}                {{ .Env.<name> }}
```

## Provider Interface

```
Every provider implements these MCP tools:

+---------------------+----------------------------------+
| Tool                | Description                      |
+---------------------+----------------------------------+
| provider_capabilities| Returns supported resources     |
| vm_create           | Create VM from spec              |
| vm_get              | Get VM state by name             |
| vm_list             | List all VMs                     |
| vm_delete           | Delete VM                        |
| network_create      | Create network resource          |
| network_get         | Get network state                |
| network_list        | List networks                    |
| network_delete      | Delete network                   |
| key_create          | Generate SSH key pair            |
| key_get             | Get key state                    |
| key_list            | List keys                        |
| key_delete          | Delete key pair                  |
+---------------------+----------------------------------+

Error Response:
{ "success": false,
  "error": { "code": "NOT_IMPLEMENTED|NOT_FOUND|PERMISSION_DENIED|...",
             "message": "Human readable description",
             "retryable": true|false } }
```

## Libvirt Provider Internals

```
+------------------------------------------------------------------+
|                    Libvirt Provider                               |
+------------------------------------------------------------------+
|  Connection: qemu:///system                                       |
|                                                                   |
|  VM Creation Flow:                                                |
|  [VMSpec] --> [Build Domain XML] --> [Create QCOW2 Overlay]      |
|      |                                    |                       |
|      v                                    v                       |
|  [Generate Cloud-Init ISO] -------> [Define Domain]              |
|                                          |                       |
|                                          v                       |
|                         [Start Domain] --> [Poll IP] --> [Ready] |
|                                                                   |
|  Network Kinds:                                                   |
|    bridge    --> ip link add, ip addr add                        |
|    libvirt   --> virsh net-define, net-start                     |
|    dnsmasq   --> dnsmasq process with config                     |
+------------------------------------------------------------------+
```

## QEMU Provider Internals

```
+------------------------------------------------------------------+
|                      QEMU Provider                                |
+------------------------------------------------------------------+
|  VM Creation Flow:                                                |
|  [VMSpec] --> [Build QEMU Args] --> [Create Overlay Disk]        |
|      |                                    |                       |
|      v                                    v                       |
|  [Cloud-Init ISO] --> [Start QEMU Process] --> [QMP Socket]      |
|                              |                                    |
|                              v                                    |
|                       [Port Forward SSH] --> [Readiness Check]   |
|                                                                   |
|  Network Modes:                                                   |
|    user   --> SLIRP with hostfwd for SSH                         |
|    tap    --> TAP device attached to bridge                      |
+------------------------------------------------------------------+
```

## AWS Provider Internals

```
+------------------------------------------------------------------+
|                       AWS Provider                                |
+------------------------------------------------------------------+
|  VM (EC2) Creation Flow:                                          |
|  [VMSpec] --> [Resolve AMI] --> [Create Security Group]          |
|      |                                |                          |
|      v                                v                          |
|  [Import SSH Key] -------> [RunInstances]                        |
|                                   |                              |
|                                   v                              |
|              [Wait Status Checks] --> [Get Public IP] --> [Ready]|
|                                                                   |
|  Network Kinds:                                                   |
|    vpc            --> CreateVpc + Internet Gateway               |
|    subnet-public  --> CreateSubnet + Route to IGW                |
|    subnet-private --> CreateSubnet + optional NAT Gateway        |
|    security-group --> CreateSecurityGroup + Ingress Rules        |
+------------------------------------------------------------------+
```

## Error Handling and Rollback

```
Create with Rollback:

[Start] --> [Create Key] --> [Create Network] --> [Create VM]
               |                   |                   |
               | (fail)            | (fail)            | (fail)
               v                   v                   v
           [Rollback]         [Rollback]          [Rollback]
               |                   |                   |
               v                   v                   v
           [Done]           [Delete Key]      [Delete Network]
                                  |                   |
                                  v                   v
                              [Done]            [Delete Key]
                                                      |
                                                      v
                                                  [Done]

Best-Effort Cleanup:
  - Continue deleting remaining resources even if individual deletions fail
  - Log all errors but return success if cleanup completed
  - Resources left behind are tracked for manual cleanup
```

## Repository Structure

```
testenv-vm/
+-- api/
|   +-- v1/                      # Core API types
|   +-- provider/v1/             # Provider interface types
+-- cmd/
|   +-- testenv-vm/              # Main orchestrator
|   +-- providers/
|       +-- testenv-vm-provider-libvirt/
|       +-- testenv-vm-provider-qemu/
|       +-- testenv-vm-provider-aws/
+-- pkg/
|   +-- orchestrator/            # DAG, executor, rollback
|   +-- provider/                # Manager, client, registry
|   +-- spec/                    # Parser, validator, template
|   +-- state/                   # State persistence
|   +-- mcp/                     # MCP server implementation
+-- internal/providers/          # Provider implementations
+-- test/
    +-- e2e/                     # End-to-end tests
    +-- integration/             # Per-provider integration tests
```
