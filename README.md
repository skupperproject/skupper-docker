# skupper-docker (DNM)
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

To troubleshoot issues:

1. Run `docker ps` to view the containers:

```
$ docker ps

CONTAINER ID        IMAGE                                       COMMAND                  CREATED             STATUS              PORTS                                                 NAMES
3c3b93460544        quay.io/skupper/skupper-docker-controller   "/app/controller"        50 minutes ago      Up 50 minutes                                                             skupper-service-controller
d8c9edc2f4a0        quay.io/skupper/qdrouterd                   "/home/qdrouterd/bin…"   50 minutes ago      Up 50 minutes       5671-5672/tcp, 9090/tcp, 45671/tcp, 55671-55672/tcp   skupper-router
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

To attach a container to the Skupper network:

1. Start the container such as the following:

```
$ docker run --name tcp-go-echo-server quay.io/skupper/tcp-go-echo
```

2. Define a service

```
$ ./skupper-docker service create tcp-go-echo 9090
```

3. Assign the target container to the service

```
$ ./skupper-docker bind tcp-go-echo container tcp-go-echo-server
```

4. Check the exposed service

```
$ ./skupper-docker list-exposed
Services exposed through Skupper:
    tcp-go-echo (tcp port 9090) with targets
      => internal.skupper.io/container name=tcp-go-echo-server

Aliases for services exposed through Skupper:
    172.18.0.4 tcp-go-echo
```

To attach a local host process on `linux` to the Skupper network:

1. Start the process such as the following (assumes golang installed):

```
$ git clone https://github.com/skupperproject/skupper-example-tcp-echo.git
$ cd skupper-example-tcp-go-echo
$ go run ./tcp-go-echo.go
```

2. Define a service

```
$ ./skupper-docker service create tcp-go-echo 9090
```

3. Assign the target process to the service

```
$ ./skupper-docker bind tcp-go-echo host-service myechoserver
```

4. Check the exposed service

```
$ ./skupper-docker list-exposed
Services exposed through Skupper:
    tcp-go-echo (tcp port 9090) with targets
      => internal.skupper.io/host-service name=myechoserver:172.17.0.1

Aliases for services exposed through Skupper:
    172.18.0.4 tcp-go-echo
```

To attach a local host process on `Mac or Windows` to the Skupper network:

1. Start the process and define a service as shown above

2. Assign the target process to the service in `host.docker.internal:host-gateway` format

```
$ ./skupper-docker bind tcp-go-echo host-service host.docker.internal:host-gateway
```

4. Check the exposed service

```
$ ./skupper-docker list-exposed
Services exposed through Skupper:
    tcp-go-echo (tcp port 9090) with targets
      => internal.skupper.io/host-service name=host.docker.internal:host-gateway

Aliases for services exposed through Skupper:
    172.18.0.4 tcp-go-echo
```

To attach a remote host process to the Skupper network:

1. Start the process on the remote host

2. Determine the IP address of the remote host (for example 192.168.22.15) and ensure it is reachable (e.g ping)

3. Create the service as described above

4. Assign the target remote host process to the service in `name:address` format

```
$ ./skupper-docker bind tcp-go-echo host-service myechoserver:192.168.22.15
```

4. Check the exposed service

```
$ ./skupper-docker list-exposed
Services exposed through Skupper:
    tcp-go-echo (tcp port 9090) with targets
      => internal.skupper.io/host-service name=myechoserver:192.168.22.15

Aliases for services exposed through Skupper:
    172.18.0.4 tcp-go-echo
```

Note that multiple targets can be concurrently assigned to a service.