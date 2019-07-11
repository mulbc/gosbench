# Open TODOs

## Worker TODOs

* Never exit when in preparation step as this could deadlock the server
* Implement S3 timeout variable
* Change S3 config to generic []aws.Config{} type
* Add second exporter that is measuring exec time of AWS functions instead of using the HTTP client

## Server TODOs

* Set Grafana annotations when tests start and when they end (at best as region)
* Log when tests start and stop
  * So that we can have a Grafana overview page parser with links to dashboards and the right timeframe
* Add timeout when waiting for workers (or whenever we could deadlock)
* Add sleep to calm disks before measuring and for Grafana annotations
  * Add sleep between tests
  * Add sleep after prep phase
* Stop after 30s when all tests are finished

## Misc

* Set up Grafana dashboard and add to repo (we should already have all data available)
  * Should include:
    * Worker bandwidth
    * Operations per second
    * Response time
    * Success rate (amount of HTTP Code 2xx responses)
    * Gauges about current time slot:
      * Total operations
      * Max bandwidth
* Add Grafana screenshots to the Readme
* Convert the above TODOs to Github tasks
