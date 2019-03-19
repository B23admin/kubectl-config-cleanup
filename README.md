## kubectl-cleanup ##

TODO: Fix godep
TODO: bind --kubeconfig flag

### Build ###

go build cmd/kubectl-cleanup.go

### Install ###

mv kubectl-cleanup /usr/local/bin/.

> Requires `kubectl > v1.12.0`
https://kubernetes.io/docs/tasks/extend-kubectl/kubectl-plugins/#before-you-begin

Resources Ref:
Sample Plugin: https://github.com/kubernetes/sample-cli-plugin/blob/master/pkg/cmd/ns.go
Client auth: https://github.com/kubernetes/client-go/blob/master/examples/out-of-cluster-client-configuration/main.go
Print flags: https://github.com/kubernetes/kubernetes/blob/master/pkg/kubectl/cmd/config/view.go
