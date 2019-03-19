#!/usr/bin/env bash

go build cmd/kubectl-cleanup.go && \
mv kubectl-cleanup /usr/local/bin/.
