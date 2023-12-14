## Preparations in Ceph
### Ensure RGW installed

```shell
# ceph orch ls
NAME                       PORTS        RUNNING  REFRESHED  AGE  PLACEMENT
alertmanager               ?:9093,9094      1/1  8m ago     16m  count:1
ceph-exporter                               3/3  8m ago     16m  *
crash                                       3/3  8m ago     16m  *
grafana                    ?:3000           1/1  8m ago     16m  count:1
mgr                                         2/2  8m ago     16m  count:2
mon                                         3/5  8m ago     16m  count:5
node-exporter              ?:9100           3/3  8m ago     16m  *
osd.all-available-devices                     6  8m ago     13m  *
prometheus                 ?:9095           1/1  8m ago     16m  count:1
rgw.test                   ?:80             2/2  8m ago     8m   ceph-demo-1;ceph-demo-2;count:2
```

You want to see a line that starts with `rgw`. In the above example it is the last line that mentions `rgw.test`.
Please note your endpoint addresses. In the above example it would be `ceph-demo-1:80` and `ceph-demo-2:80`.

#### Test S3 endpoint address

You can easily test if your S3 endpoint address is correct with curl:

```shell
# curl ceph-demo-1:80
<?xml version="1.0" encoding="UTF-8"?><ListAllMyBucketsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Owner><ID>anonymous</ID><DisplayName></DisplayName></Owner><Buckets></Buckets></ListAllMyBucketsResult>
# curl ceph-demo-2:80
<?xml version="1.0" encoding="UTF-8"?><ListAllMyBucketsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Owner><ID>anonymous</ID><DisplayName></DisplayName></Owner><Buckets></Buckets></ListAllMyBucketsResult>
```

### Create RGW user

```shell
# radosgw-admin user create --uid=gosbench --display-name="GOSBENCH"
{
    "user_id": "gosbench",
    "display_name": "GOSBENCH",
    "email": "",
    "suspended": 0,
    "max_buckets": 1000,
    "subusers": [],
    "keys": [
        {
            "user": "gosbench",
            "access_key": "H5X8459I7VYC1J9T6EF7",
            "secret_key": "zFOA496GIiC9oHoOodpyL9wyvpH2naYvGKpmPMd4"
        }
    ],
    "swift_keys": [],
    "caps": [],
    "op_mask": "read, write, delete",
    "default_placement": "",
    "default_storage_class": "",
    "placement_tags": [],
    "bucket_quota": {
        "enabled": false,
        "check_on_raw": false,
        "max_size": -1,
        "max_size_kb": 0,
        "max_objects": -1
    },
    "user_quota": {
        "enabled": false,
        "check_on_raw": false,
        "max_size": -1,
        "max_size_kb": 0,
        "max_objects": -1
    },
    "temp_url_keys": [],
    "type": "rgw",
    "mfa_ids": []
}
```

Note down the `access_key` and `secret_key`

## Deploy Gosbench with cephadm

### Write custom container service for workers and server

Create a file `gosbench-svc.yaml` with content:

```yaml
service_type: container
service_id: gosb-worker
placement:
  host_pattern: "*"
  count: 3
  # count_per_host: 1
spec:
  image: quay.io/mulbc/gosbench-worker:latest
  entrypoint: './main -d -s gosbench-server:2000'
  args:
    - "--net=host"
    - "--cpus=2"
  ports:
    - 8888
---
service_type: container
service_id: gosb-server
placement:
  host_pattern: "*"
  count: 1
spec:
  image: quay.io/mulbc/gosbench-server:latest
  entrypoint: './main -c /config/config.yml'
  args:
    - "--net=host"
  ports:
    - 2000
  volume_mounts:
    CONFIG_DIR: /config
  dirs:
    - CONFIG_DIR
  files:
    CONFIG_DIR/config.yml: |-
      s3_config:
        - access_key: "H5X8459I7VYC1J9T6EF7"
          secret_key: "zFOA496GIiC9oHoOodpyL9wyvpH2naYvGKpmPMd4"
          region: us-east-1
          endpoint: "ceph-demo-1:80"
          skipSSLverify: true

      # For generating annotations when we start/stop testcases
      # https://grafana.com/docs/http_api/annotations/#create-annotation
      grafana_config:
        endpoint: http://grafana
        username: admin
        password: grafana

      tests:
        - name: RGW-test
          read_weight: 0
          write_weight: 1
          delete_weight: 0
          list_weight: 0
          objects:
            size_min: 100
            size_max: 200
            part_size: 3
            # distribution: constant, random, sequential
            size_distribution: constant
            unit: MB
            number_min: 100
            number_max: 100
            # distribution: constant, random, sequential
            number_distribution: constant
          buckets:
            number_min: 1
            number_max: 10
            # distribution: constant, random, sequential
            number_distribution: constant
          # Name prefix for buckets and objects
          bucket_prefix: gosbench2-
          object_prefix: obj
          # End after a set amount of time
          # Runtime in time.Duration - do not forget the unit please
          # stop_with_runtime: 60s # Example with 60 seconds runtime
          stop_with_runtime:
          # End after a set amount of operations (per worker)
          stop_with_ops: 3000
          # Number of s3 performance test servers to run in parallel
          workers: 3
          # Set wheter workers share the same buckets or not
          # If set to True - bucket names will have the worker # appended
          workers_share_buckets: True
          # Number of requests processed in parallel by each worker
          parallel_clients: 30
          # Remove all generated buckets and its content after run
          clean_after: True
```

**ATTENTION** --> Make sure you set the `endpoint`, `access_key` and `secret_key` for your environment!

Then deploy it:

```shell
# ceph orch apply -i gosbench-svc.yaml
```

You can redo this if you make any changes to the yaml file

If you ever want to get rid of the gosbench containers - you can remove them with:

```shell
## REMOVE gosbench from Ceph:
# ceph orch rm container.gosb-worker
Removed service container.gosb-worker
# ceph orch rm container.gosb-server
Removed service container.gosb-server
```

You can check the status of Gosbench via these two commands:
```shell
# ceph orch ls
NAME                       PORTS        RUNNING  REFRESHED  AGE  PLACEMENT
alertmanager               ?:9093,9094      1/1  65s ago    3h   count:1
ceph-exporter                               3/3  67s ago    3h   *
crash                                       3/3  67s ago    3h   *
grafana                    ?:3000           1/1  65s ago    3h   count:1
mgr                                         2/2  65s ago    3h   count:2
mon                                         3/5  67s ago    3h   count:5
node-exporter              ?:9100           3/3  67s ago    3h   *
osd.all-available-devices                     6  67s ago    3h   *
prometheus                 ?:9095           1/1  65s ago    3h   count:1
rgw.test                   ?:80             2/2  65s ago    3h   ceph-demo-1;ceph-demo-2;count:2


# ceph orch ps --daemon_type container
NAME                               HOST         PORTS        STATUS  REFRESHED   AGE  MEM USE  MEM LIM  VERSION    IMAGE ID
container.gosb-server.ceph-demo-2  ceph-demo-2  *:2000,2000  error      5m ago  119m        -        -  <unknown>  <unknown>
container.gosb-worker.ceph-demo-1  ceph-demo-1  *:8888,8888  error      5m ago    2h        -        -  <unknown>  <unknown>
container.gosb-worker.ceph-demo-2  ceph-demo-2  *:8888,8888  error      5m ago    2h        -        -  <unknown>  <unknown>
container.gosb-worker.ceph-demo-3  ceph-demo-3  *:8888,8888  error      5m ago    2h        -        -  <unknown>  <unknown>
```
