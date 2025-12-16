# Runtime VM Creation

**Create Additional VMs During Test Execution Without Modifying Your Spec**

"I needed to dynamically scale worker VMs during my distributed systems test based on load patterns, but testenv-vm only supported static VM definitions in forge.yaml." "Now I can call `client.CreateVM()` mid-test to spin up additional nodes that integrate seamlessly with my existing network and keys."

Runtime VM creation solves the dynamic infrastructure problem: tests that need to create VMs on-demand based on test logic rather than pre-defined specifications.

## Table of Contents

- [What problem does runtime VM creation solve?](#what-problem-does-runtime-vm-creation-solve)
- [Quick Start](#quick-start)
- [How do I access the RuntimeProvisioner?](#how-do-i-access-the-runtimeprovisioner)
- [How do I create a VM at runtime?](#how-do-i-create-a-vm-at-runtime)
- [How do I reference existing resources?](#how-do-i-reference-existing-resources)
- [How do I delete a runtime VM?](#how-do-i-delete-a-runtime-vm)
- [What happens during cleanup?](#what-happens-during-cleanup)
- [Can I create VMs concurrently?](#can-i-create-vms-concurrently)

## What problem does runtime VM creation solve?

Static VM definitions work for most tests, but some scenarios require dynamic infrastructure:

- **Distributed systems tests**: Scale worker nodes based on test conditions
- **Chaos engineering**: Create/destroy VMs to test failure recovery
- **Load testing**: Spawn additional instances to simulate scaling events
- **Multi-cluster tests**: Build clusters programmatically based on test parameters

Previously, users had to pre-define all possible VMs or implement custom provisioning. Runtime VM creation integrates dynamic VMs into the existing testenv-vm lifecycle.

## Quick Start

```go
func TestDynamicWorkers(t *testing.T) {
    // Get provisioner from orchestrator
    result, _ := orchestrator.Create(ctx, input)
    provisioner := result.Provisioner

    // Create client with provisioner attached
    client, _ := client.NewClient(provisioner, "control-plane",
        client.WithProvisioner(provisioner))

    // Create worker VM at runtime
    worker, _ := client.CreateVM(ctx, "worker-1", v1.VMSpec{
        Memory:  2048,
        VCPUs:   2,
        Network: "test-net",
        Disk:    v1.DiskSpec{Size: "10G"},
        CloudInit: &v1.CloudInitSpec{
            Users: []v1.UserSpec{{
                Name:              "ubuntu",
                SSHAuthorizedKeys: []string{"{{ .Keys.vm-ssh.PublicKey }}"},
            }},
        },
    })

    // Use the worker
    worker.WaitReady(ctx, 2*time.Minute)
    worker.Run(ctx, "echo", "hello from worker")
}
```

## How do I access the RuntimeProvisioner?

`Orchestrator.Create()` returns a `CreateResult` containing both the artifact and the provisioner:

```go
result, err := orchestrator.Create(ctx, input)
if err != nil {
    return err
}

artifact := result.Artifact      // Serializable test artifact
provisioner := result.Provisioner // RuntimeProvisioner for dynamic VMs
```

The provisioner is returned directly (not serialized in artifact JSON) because it contains runtime state (goroutines, mutex, provider connections) that cannot be serialized.

## How do I create a VM at runtime?

Two equivalent approaches:

**Via Client (recommended)**:
```go
client, _ := client.NewClient(provisioner, "existing-vm",
    client.WithProvisioner(provisioner))

newVM, err := client.CreateVM(ctx, "worker-1", v1.VMSpec{...})
```

**Via RuntimeProvisioner directly**:
```go
newVM, err := provisioner.CreateVM(ctx, "worker-1", v1.VMSpec{...})
```

Both return a new `*Client` for the created VM. The returned client also has the provisioner attached, so it can create additional VMs.

## How do I reference existing resources?

Runtime VMs can reference resources created during setup using templates:

```go
spec := v1.VMSpec{
    Network: "test-net",  // Reference network by name
    Disk: v1.DiskSpec{
        BaseImage: "{{ .Images.ubuntu.Path }}",  // Template reference
        Size:      "20G",
    },
    CloudInit: &v1.CloudInitSpec{
        Users: []v1.UserSpec{{
            Name: "ubuntu",
            SSHAuthorizedKeys: []string{
                "{{ .Keys.vm-ssh.PublicKey }}",  // Template reference
            },
        }},
    },
}
```

Templates are rendered before VM creation. Access the template context for dynamic lookups:

```go
ctx := provisioner.GetTemplateContext()
publicKey := ctx.Keys["vm-ssh"].PublicKey
networkCIDR := ctx.Networks["test-net"].CIDR
```

## How do I delete a runtime VM?

Explicit deletion is optional but supported:

```go
err := client.DeleteVM(ctx, "worker-1")
```

The VM state is updated to "destroyed" and the provider is called to remove the VM. Note: Runtime VMs are automatically deleted during environment cleanup, so explicit deletion is only needed for mid-test cleanup.

## What happens during cleanup?

Runtime VMs are tracked in the environment state and cleaned up automatically:

1. Runtime VMs are appended to the ExecutionPlan as new phases
2. During `Delete()`, resources are destroyed in reverse phase order
3. Runtime VMs (later phases) are deleted before setup VMs (earlier phases)

This ensures proper cleanup even if the test crashes or is interrupted.

## Can I create VMs concurrently?

Yes. RuntimeProvisioner uses a mutex for thread safety:

```go
var wg sync.WaitGroup
for i := 0; i < 3; i++ {
    wg.Add(1)
    go func(n int) {
        defer wg.Done()
        name := fmt.Sprintf("worker-%d", n)
        _, err := client.CreateVM(ctx, name, spec)
        // handle error
    }(i)
}
wg.Wait()
```

Concurrent CreateVM calls are serialized internally. For maximum parallelism, consider creating VMs sequentially but running operations on them concurrently.
