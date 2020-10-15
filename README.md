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