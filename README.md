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

* Installed dependency \
`renovate_dependency` labels: "repository", "manager", "packageFile", "depName", "currentVersion", "warning", "baseBranch"
   
* Available update of an installed dependency \
`renovate_dependency_update` labels: "repository", "manager", "packageFile", "depName", "currentVersion", "updateType", "newVersion", "vulnerabilityFix", "releaseTimestamp", "baseBranch", "pending"
   
* Timestamp of the last successful execution \
`renovate_last_successful_timestamp` labels: "repository"

### Usage

If renovate is executed via the official image (which it usually is in self-hosted environments) the structured output can be piped to `renovate-metrics` which transforms the output into
prometheus compatible metrics and pushes them to a prometheus push gateway.

Important renovate needs to be started with `LOG_LEVEL=debug` as well as `LOG_FORMAT=json` otherwise `renovate-metrics` is unable to get all information required.

Example execution (It also goes through a tee pipe to get the renovate output to stderr as well):
```sh
docker run -e RENOVATE_TOKEN=$GITHUB_TOKEN -e LOG_FORMAT=json -e LOG_LEVEL=debug renovate/renovate:slim org/my-repository | tee /dev/stderr | docker run -i ghcr.io/raffis/renovate-metrics:latest push --prometheus=http://prometheus-push-gateway:9091
```

### Authentication

If the push gateway sits behind authentication, `renovate-metrics` can authenticate using either HTTP basic auth or a
bearer token. The credentials are applied to all requests sent to the push gateway. Without any of the settings below
`renovate-metrics` talks to the push gateway unauthenticated, as it always did.

| Flag | Environment variable | Description |
|------|----------------------|-------------|
| `--prometheus-username` | `RENOVATE_METRICS_PROMETHEUS_USERNAME` | Username for basic auth |
| `--prometheus-password` | `RENOVATE_METRICS_PROMETHEUS_PASSWORD` | Password for basic auth |
| `--prometheus-bearer-token` | `RENOVATE_METRICS_PROMETHEUS_BEARER_TOKEN` | Bearer token |

An explicitly given flag takes precedence over its environment variable. Basic auth requires both a username and a
password, and it can not be combined with a bearer token. Either mistake fails the command with a configuration error
before any request is sent.

Credentials are only ever sent to the push gateway given via `--prometheus`. If the gateway answers with a redirect to
another host or downgrades the scheme, the credentials are not forwarded to that target.

Basic auth:
```sh
export RENOVATE_METRICS_PROMETHEUS_USERNAME=my-user
export RENOVATE_METRICS_PROMETHEUS_PASSWORD=my-secret
renovate-metrics push \
  --prometheus=https://pushgateway.example.com
```

Bearer token:
```sh
export RENOVATE_METRICS_PROMETHEUS_BEARER_TOKEN=my-token
renovate-metrics push \
  --prometheus=https://pushgateway.example.com
```

Prefer the environment variables over the flags and store the values as masked/protected CI variables (or as a secret
mounted into the environment). Command line arguments are visible to every other process on the host through the
process list, so passing a password or token via `--prometheus-password` or `--prometheus-bearer-token` should be
limited to local debugging.

> [!WARNING]
> Do not put credentials into the `--prometheus` URL. A URL of the form `https://user:password@pushgateway.example.com`
> leaks the password into the error output: if the gateway answers with an unexpected status code (a `401` for example,
> which is exactly what happens when the credentials are wrong) the underlying prometheus client reports the failure
> including the full URL, and `renovate-metrics` exits by printing that error. The password then ends up in the CI job
> log verbatim. Use the settings above instead, they never become part of the URL.

The credentials configured through the settings above are not printed anywhere. They are not part of the command help,
they are not part of any error message, and a crash does not dump the environment.

### Grafana dashboard

This repository comes with a predefined grafana dashboard which gives an overview around all sorts of things. 
See grafana/dashboard.json
