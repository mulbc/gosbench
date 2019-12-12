# Open TODOs

## Worker TODOs

* Never exit when in preparation step as this could deadlock the server
* Implement S3 timeout variable
* ~~Change S3 config to generic []aws.Config{} type~~ Not parseable from Yaml
* Add second exporter that is measuring exec time of AWS functions instead of using the HTTP client

## Server TODOs

* Set Grafana annotations when tests start and when they end (at best as region)
* Add timeout when waiting for workers (or whenever we could deadlock)

## Misc

* Convert the above TODOs to Github tasks
