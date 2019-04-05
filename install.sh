#!/usr/bin/env bash

go build cmd/kubectl-config-cleanup.go && \
mv kubectl-config-cleanup /usr/local/bin/kubectl-config_cleanup
