```sh
docker run -e RENOVATE_TOKEN=$GITHUB_TOKEN -e LOG_FORMAT=json -e LOG_LEVEL=debug renovate/renovate:slim org/my-repository | tee /dev/tty | docker run -i raffis/renovate-metrics:latest push --prometheus=http:/prometheus-push-gateway:9091
```