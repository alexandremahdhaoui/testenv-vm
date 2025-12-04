# TODOs

- [ ] Implement a testenv-vm go client library (package) that simplifies how the user can e.g. run commands via ssh to the vm, check VM is successfully provisioned, copy files from/to the VM, verify files exist etc... -> we can create a nice user friendly go client
  - Must study /home/alexandremahdhaoui/go/src/github.com/alexandremahdhaoui/edge-cd/ to learn how the ssh command with context etc are done. Also rename "prependCmd" to "privilegeEscalation" or "privilegeEscalation.cmd" "privilegeEscalation" can be configured in the spec (i.e. what pattern/command requires privilege escalation - because not all does etc...)
  - The Go client MUST be unit tested
  - The Go client MUST be e2e tested
  - The Go client MUST be used for all e2e tests in the codebase
  - The Go client is AGNOSTIC of the VM backend => client provider can be injected into it.
  - The stub and libvirt provider of the Go client MUST be implemented and tested
