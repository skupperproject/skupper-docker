# skupper-docker
A command-line tool for setting up and managing Skupper docker installations

Note: If you are using Fedora 32, use the https://fedoramagazine.org/docker-and-fedora-32/ procedure to install Docker.

To create the `skupper-docker` command:

1. Clone this repo.
2. Run the following command in the repo root directory:
```
 make build-cmd
```
After this command is completed, you can create a skupper site:
```
 ./skupper-docker init
```

3. To connect to another Skupper site, you must update the generated token with the site UID. 

a) Get the UID from the Skupper site you are trying to connect to:
```
$ kubectl get -o template cm/skupper-site --template={{.metadata.uid}}
```

b) Add the UID to the token.
For example, if you followed the [Getting Started](https://skupper.io/start/index.html) and the UID is `fba75742-f594-4329-ae70-18698b88cd4a`, add the following line to `$HOME/secret.yaml` under `metadata`:
```
 skupper.io/generated-by: fba75742-f594-4329-ae70-18698b88cd4a
```

To troubleshoot issues:

1. Run `docker ps` to view the containers:

```
$ docker ps

CONTAINER ID        IMAGE                                       COMMAND                  CREATED             STATUS              PORTS                                                 NAMES
3c3b93460544        quay.io/skupper/skupper-docker-controller   "/app/controller"        50 minutes ago      Up 50 minutes                                                             skupper-service-controller
d8c9edc2f4a0        quay.io/interconnectedcloud/qdrouterd       "/home/qdrouterd/bin…"   50 minutes ago      Up 50 minutes       5671-5672/tcp, 9090/tcp, 45671/tcp, 55671-55672/tcp   skupper-router
```

2. Log into the router container:

```
$ docker exec -it skupper-router /bin/bash
```

3. Check the router stats:

```
[root@skupper-router bin]# qdstat -c
2020-10-15 14:05:05.776017 UTC
localhost.localdomain-skupper-router

Connections
  id  host              container                                               role    dir  security                         authentication            tenant  last dlv      uptime
  ==========================================================================================================================================================================================
  2   172.18.0.3:53014  mpsp0EF792H2RNTiqhfpIm09C1X6UY1-2mnucktxUPTPbSflwMClaA  normal  in   TLSv1.3(TLS_AES_128_GCM_SHA256)  CN=skupper-router(x.509)          000:00:00:04  000:00:53:02
  5   127.0.0.1:58938   b66430bc-45ed-4b8e-b48d-70e25e63f8e3                    normal  in   no-security                      no-auth                           000:00:00:00  000:00:00:00

```

This shows typical output for a successfully instantiated skupper site.
If you do not see similar output, check your Docker network configuration, for example firewall settings.


