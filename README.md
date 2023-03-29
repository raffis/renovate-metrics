## Renovate prometheus metrics

![Release](https://img.shields.io/github/v/release/raffis/renovate-metrics)
[![release](https://github.com/raffis/renovate-metrics/actions/workflows/release.yaml/badge.svg)](https://github.com/raffis/renovate-metrics/actions/workflows/release.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/raffis/renovate-metrics)](https://goreportcard.com/report/github.com/raffis/renovate-metrics)

Ever wanted to get metrics from renovate? 
This is now possible with this tool. It extracts the necessary data from the structured renovate logs and transforms the 
information into prometheus metrics.
This is possible if renovate runs in self-hosted environments.
(If you are able to get the structured logs from other deployments it will also work.)

## Requirements

`renovate-metrics` requires a [prometheus-pushgateway](https://github.com/prometheus/pushgateway). 

### Metrics

* `renovate_dependency` labels: "manager", "packageFile", "depName", "currentVersion"
   Installed dependency
* `renovate_dependency_update` labels: "manager", "packageFile", "depName", "currentVersion", "updateType", "newVersion", "vulnerabilityFix", "releaseTimestamp"
   Available update of an installed dependency
* `renovate_last_successful_timestamp` labels: []
   Timestamp of the last successful execution

### Usage

If renovate is executed via the official image (which it usually is in self-hosted environments) the structured output can be piped to `renovate-metrics` which transforms the output into
prometheus compatible metrics and pushes them to a prometheus push gateway.

Important renovate needs to be started with `LOG_LEVEL=debug` as well as `LOG_FORMAT=json` otherwise `renovate-metrics` is unable to get all information required.

Example execution (It also goes through a tee pipe to get the renovate output to stdout as well):
```sh
docker run -e RENOVATE_TOKEN=$GITHUB_TOKEN -e LOG_FORMAT=json -e LOG_LEVEL=debug renovate/renovate:slim org/my-repository | tee /dev/tty | docker run -i raffis/renovate-metrics:latest push --prometheus=http:/prometheus-push-gateway:9091
```

### Grafana dashboard

This repository comes with a predefined grafana dashboard which gives an overview around all sorts of things. 
See grafana/dashboard.json