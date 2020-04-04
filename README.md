# kubectl config-cleanup plugin #

`kubectl config-cleanup` is a plugin for automatically cleaning up your kubeconfig.  Every cloud provider has their own utilities for adding kubernetes cluster credentials to your kubeconfig but they don't offer the ability to clean it up once the cluster is deleted. For those of us who launch and delete multiple clusters per day, it would be useful to have an automated way to clean up old kubeconfig entries. This plugin will attempt to connect to each cluster defined in a context, if the connection succeeds then the user, cluster, and context entry are maintained in the result. Otherwise, the entries are removed.

```bash
# prints the cleaned kubeconfig to stdout, similar to running: kubectl config view
kubectl config-cleanup

# cleanup and save the result
kubectl config-cleanup --raw > ./kubeconfig-clean.yaml

# cleanup and print the configs that were removed
kubectl config-cleanup --print-removed --raw > ./kubeconfig-removed.yaml

# print only the context names that were removed
kubectl config-cleanup --print-removed -o=jsonpath='{ range.contexts[*] }{ .name }{"\n"}'
```

> DO NOT attempt to overwrite the source kubeconfig, it will result in an empty config.
See Known Issues below for details

### config-cleanup.ignore ###

Add a `~/.kube/config-cleanup.ignore` to specify contexts which should be ignored during cleanup. The associated context, user, and cluster will be maintained in the output. This is useful for long running clusters where the api server is behind a firewall.

example:

```yaml
---
apiVersion: v1
kind: ConfigMap
data:
  contexts: |
    prod-cluster
    staging-cluster
    docker-for-desktop
```

## Install ##

Install with krew: `kubectl krew install config-cleanup`

or download the [latest release binary](https://github.com/b23llc/kubectl-cleanup/releases/latest) for your platform and add it to your `$PATH`


## Build from source ##

```bash
$ go build cmd/kubectl-config-cleanup.go
$ mv kubectl-config-cleanup /usr/local/bin/kubectl-config_cleanup
```

## Release ##

dryrun: `goreleaser --snapshot --skip-publish --rm-dist`

publish: `goreleaser release --rm-dist`


## Todo ##

- Add `users` and `clusters` functionality for config-cleanup.ignore

## Known issues ##

- Error log message when cleaning up GKE clusters that have already been terminated
https://github.com/kubernetes/kubernetes/issues/73791

- Attempting to overwrite the source kubeconfig will wipe the config completely.

i.e. Dont do this: `kubectl config-cleanup --kubeconfig=~/.kube/config --raw > ~/.kube/config`  
This behavior is consistent with the behavior of `kubectl config view --raw > ~/.kube/config`

A simple *example* shell script `kubectl-config_swap` in your path can easily solve for this: 

```bash
#!/usr/bin/env bash
set -euo pipefail
touch ~/.kube/config.swap
mv ~/.kube/config ~/.kube/config.swap.tmp
mv ~/.kube/config.swap ~/.kube/config
mv ~/.kube/config.swap.tmp ~/.kube/config.swap
```

The workflow would appear as:

```bash
$ kubectl config-cleanup --raw > ./kube/config.swap
$ kubectl config-swap
```

Running `config-swap` twice would revert the changes.

> Requires: [`kubectl > v1.12.0`](https://kubernetes.io/docs/tasks/extend-kubectl/kubectl-plugins/#before-you-begin)

> NOTE: `config-cleanup` does not support [merging kubeconfig files](https://kubernetes.io/docs/concepts/configuration/organize-cluster-access-kubeconfig/#the-kubeconfig-environment-variable)
