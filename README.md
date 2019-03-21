## kubectl-cleanup ##

TODO: Fix godep
TODO: Add .cleanupignore functionality

### Build ###

go build cmd/kubectl-cleanup.go

### Install ###

mv kubectl-cleanup /usr/local/bin/.

> Requires `kubectl > v1.12.0`
https://kubernetes.io/docs/tasks/extend-kubectl/kubectl-plugins/#before-you-begin

> NOTE: cleanup does not support merging kubeconfig files: https://kubernetes.io/docs/concepts/configuration/organize-cluster-access-kubeconfig/#the-kubeconfig-environment-variable
