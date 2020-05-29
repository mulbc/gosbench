# Running Gosbench in Kubernetes and Openshift

The following commands will default to using the `oc` command, but you can just as well use `kubectl` if you are running this on Kubernetes.

## Preparing the deployment

1. Clone this repository
1. cd into the `k8s` folder
1. Open `gosbench.yaml` in your favorite editor

In the very top of the gosbench.yaml file, you will find a ConfigMap, which represents the Gosbench config that will be used for the test. Modify this to your liking.
If you just want to test things out, at least change the s3_config parameters so that Gosbench knows how to connect to your S3 endpoint.

## Deploying Gosbench

Setting everything up is as easy as:

**NOTE**: The yaml files do not have a namespace defined, be sure to `oc project ...` into the namespace you want to use.

1. Run: `oc apply -f monitoring.yaml`
1. Run: `oc apply -f gosbench.yaml`
1. Expose the Grafana service `oc expose svc/grafana`
1. Get the Grafana address: `oc get route grafana`

**NOTE:** The Grafana credentials are `admin`:`admin`.

When you now execute `oc get all` you should see something similar to this:

```bash
$ oc get all
NAME                                   READY   STATUS              RESTARTS   AGE
pod/gosbench-server-74cbdfc774-2t6tv   0/1     ContainerCreating   0          16s
pod/monitoring-9b98fb-nbqk6            2/2     Running             0          59s
pod/worker1-55zgh                      0/1     ContainerCreating   0          17s
pod/worker2-vvfbs                      0/1     ContainerCreating   0          17s

NAME                       TYPE           CLUSTER-IP       EXTERNAL-IP                            PORT(S)          AGE
service/gosbench-server    NodePort       172.30.215.159   <none>                                 2000:30230/TCP   16s
service/gosbench-worker1   NodePort       172.30.187.174   <none>                                 8888:30314/TCP   16s
service/gosbench-worker2   NodePort       172.30.149.186   <none>                                 8888:31499/TCP   15s
service/grafana            NodePort       172.30.15.243    <none>                                 3000:30522/TCP   7m45s
service/kubernetes         ClusterIP      172.30.0.1       <none>                                 443/TCP          3d22h
service/openshift          ExternalName   <none>           kubernetes.default.svc.cluster.local   <none>           3d22h
service/prometheus         NodePort       172.30.158.169   <none>                                 9090:30200/TCP   7m45s

NAME                              READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/gosbench-server   0/1     1            0           16s
deployment.apps/monitoring        1/1     1            1           59s

NAME                                         DESIRED   CURRENT   READY   AGE
replicaset.apps/gosbench-server-74cbdfc774   1         1         0       17s
replicaset.apps/monitoring-9b98fb            1         1         1       60s

NAME                COMPLETIONS   DURATION   AGE
job.batch/worker1   0/1           18s        18s
job.batch/worker2   0/1           18s        18s

NAME                               HOST/PORT                           PATH   SERVICES   PORT   TERMINATION   WILDCARD
route.route.openshift.io/grafana   grafana-default.apps.[...]          grafana    3000                 None
```


### Fix workers that finished too early

**NOTE:** in some rare cases, the workers are up, before the server is ready, thus the workers will exit and complain that the server is unreachable.

You can check if this is the case in your envionment by executing `oc get job` - if it looks like this:

```bash
$ oc get job
NAME      COMPLETIONS   DURATION   AGE
worker1   0/1           14m        14m
worker2   1/1           59s        11m
```

Then you are most likely affected

**FIX**: To fix this, re-add the completed worker jobs:

1. Delete the worker `oc delete jobs -l app=gosbench-worker2`
1. Readd the worker `oc apply -f gosbench.yaml`

## Evaluating Gosbench runs

While the Gosbench benchmark is running, it is best to watch all gosbench logs with a tool like [stern](https://github.com/wercker/stern).
If you do not have stern, you can also only watch the server log, which provides the most valuable information: `oc logs deployment.apps/gosbench-server -f`

A finished run will print something similar to this in the server log:

```
time="2020-05-29T12:02:01Z" level=info msg="Ready to accept connections"
time="2020-05-29T12:02:02Z" level=info msg="10.130.2.41:44678 connected to us "
time="2020-05-29T12:02:02Z" level=info msg="We found worker 1 / 2 for test 0" Worker="10.130.2.41:44678"
time="2020-05-29T12:05:07Z" level=info msg="10.130.2.44:59886 connected to us "
time="2020-05-29T12:05:07Z" level=info msg="We found worker 2 / 2 for test 0" Worker="10.130.2.44:59886"
time="2020-05-29T12:05:15Z" level=info msg="All workers have finished preparations - starting performance test" test=0
time="2020-05-29T12:05:28Z" level=info msg="10.130.2.44:60004 connected to us "
time="2020-05-29T12:05:31Z" level=info msg="All workers have finished the performance test - continuing with next test" test=0
time="2020-05-29T12:05:31Z" level=info msg="GRAFANA: ?from=1590753915508&to=1590753931123" test=0
time="2020-05-29T12:05:31Z" level=info msg="All performance tests finished"
time="2020-05-29T12:05:31Z" level=info msg="10.130.2.41:45958 connected to us "
```

Important for the evaluation is always the GRAFANA line for every test, because it contains the exact timestamps during which the test was executed.

Now we head over to Grafana - first fetch the URL:

```bash
$ oc get route grafana
NAME      HOST/PORT                                               PATH   SERVICES   PORT   TERMINATION   WILDCARD
grafana   grafana-default.apps.oc.example.com          grafana    3000                 None
```

In my example I would browse to http://grafana-default.apps.oc.example.com

**NOTE:** The Grafana credentials are `admin`:`admin`.

Once you are logged in, you will find a pre-existing Dashboard called Gosbench. Go there.

Your URL will look similar to this:
http://grafana-default.apps.oc.example.com/d/R67SuKSZk/gosbench?orgId=1&from=now-30m&to=now

Now we will insert the timestamps we got from the test results to go to

http://grafana-default.apps.oc.example.com/d/R67SuKSZk/gosbench?from=1590753915508&to=1590753931123

This will load the exact time window when our test was executed and the results are interpreted correctly.
