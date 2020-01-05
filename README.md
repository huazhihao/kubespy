# kubespy: pod debugging tool for kubernetes with docker runtimes

[![Build Status](https://travis-ci.org/huazhihao/kubespy.svg?branch=master)](https://travis-ci.org/huazhihao/kubespy)
![Proudly written in Bash](https://img.shields.io/badge/written%20in-bash-ff69b4.svg)

`kubespy` is a kubectl plugin implemented in bash to debug a application pod by creating and running an temporary `spy container` to join its docker namespace(eg. pid/net/ipc). You can specify the image of this temporary spy container which is supposed to include all the debug tools required, so you don't have to unnecessarily bundle those tools with the application image.

Compared to another plugin [kubectl-debug](https://github.com/verb/kubectl-debug), `kubespy` doesn't require the prerequisites of 1. `EphemeralContainers` to be enabled in the cluster 2. `shareProcessNamespace` to be enabled for the application pod. `EphemeralContainers` is still in early alpha state and is not suitable for production clusters. And modifying the spec of `shareProcessNamespace` will destroy the original application pod and the evidences inside as well.

Meanwhile `kubespy` has its prerequisite - the node that hosting the application pod needs to run on a docker runtime with admin privileges.

## Installation

```sh
$ curl -so kubectl-spy https://raw.githubusercontent.com/huazhihao/kubespy/master/kubespy
$ sudo install kubectl-spy /usr/local/bin/
```

## Usage

```sh
$ kubectl spy POD [-c CONTAINER] [--spy-image SPY_IMAGE]
```


## Examples:

[![asciicast](https://asciinema.org/a/290096.svg)](https://asciinema.org/a/290096)

```sh
# debug the first container nginx from mypod
$kubectl spy mypod

# debug container nginx from mypod
$ kubectl spy mypod -c nginx

# debug container nginx from mypod using busybox
$ kubectl spy mypod -c nginx --spy-image busybox
```

## Architecture

```
local machine: kubectl spy
                    |
                    v
master node:   kube-apiserver
                    |
                    v
worker node:   kubelet
                    |
                    v
               spy pod (eg. busybox)
                    | (chroot)
                    v
               docker runtime
                    | (run)
                    v
               spy container
                    | (join docker namespace: pid/net/ipc)
                    v
               application pod (eg. nginx)
```
